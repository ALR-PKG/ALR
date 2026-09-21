// ALR - Any Linux Repository
// Copyright (C) 2025 The ALR Authors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package dbus

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

// Service управляет D-Bus соединением и экспортирует объекты ALR
type Service struct {
	conn   *dbus.Conn
	deps   *appbuilder.AppDeps
	config *config.ALRConfig

	// Менеджеры объектов
	manager  *DBusManager
	packages map[dbus.ObjectPath]*DBusPackage
	jobs     map[dbus.ObjectPath]*DBusJob

	// Счетчик ID для задач
	jobIDCounter uint32

	// Context для отмены
	ctx    context.Context
	cancel context.CancelFunc

	// Mutex для потокобезопасности
	mu sync.RWMutex

	// Notification клиент
	notifier *Notifier

	// Авторизатор мутирующих вызовов (по умолчанию PolicyKit);
	// подменяется в тестах
	authorizer Authorizer
}

// NewService создает новый D-Bus сервис
func NewService() *Service {
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		packages: make(map[dbus.ObjectPath]*DBusPackage),
		jobs:     make(map[dbus.ObjectPath]*DBusJob),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Init инициализирует ALR зависимости
func (s *Service) Init() error {
	ctx := context.Background()

	deps, err := appbuilder.New(ctx).
		WithConfig().
		WithDB().
		WithReposNoPull().
		WithDistroInfo().
		WithManager().
		Build()
	if err != nil {
		return fmt.Errorf("failed to initialize ALR dependencies: %w", err)
	}

	s.deps = deps
	s.config = deps.Cfg

	// Инициализация менеджера
	s.manager = NewDBusManager(s)

	// Инициализация notifier
	s.notifier = NewNotifier(s.conn)

	return nil
}

// Run запускает D-Bus сервис
func (s *Service) Run(sessionBus bool) error {
	var err error

	// Подключение к шине
	if sessionBus {
		s.conn, err = dbus.ConnectSessionBus()
	} else {
		s.conn, err = dbus.ConnectSystemBus()
	}
	if err != nil {
		return fmt.Errorf("failed to connect to D-Bus: %w", err)
	}
	defer s.conn.Close()

	slog.Info("Connected to D-Bus", "session", sessionBus)

	// Авторизатор по умолчанию инициализируется один раз до начала
	// обработки вызовов (в тестах authorizer подменяется заранее)
	if s.authorizer == nil {
		s.authorizer = NewPolicyKitAuthorizer(s.conn)
	}

	// Инициализация ALR
	if err := s.Init(); err != nil {
		return err
	}
	defer s.deps.Defer()

	// Запрос well-known name
	reply, err := s.conn.RequestName(DBusWellKnownName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("failed to request name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("name already taken")
	}

	slog.Info("Acquired D-Bus name", "name", DBusWellKnownName)

	// Экспорт объектов
	if err := s.exportObjects(); err != nil {
		return fmt.Errorf("failed to export objects: %w", err)
	}

	slog.Info("D-Bus service running")

	// Ожидание сигнала завершения
	<-s.ctx.Done()

	return nil
}

// exportObjects экспортирует все D-Bus объекты
func (s *Service) exportObjects() error {
	// Экспорт Manager
	if err := s.conn.Export(s.manager, DBusObjectPath, ManagerInterfaceName); err != nil {
		return fmt.Errorf("failed to export manager: %w", err)
	}

	// Экспорт introspection для Manager
	if err := s.conn.Export(
		introspect.NewIntrospectable(&introspect.Node{
			Interfaces: []introspect.Interface{s.manager.IntrospectionData()},
		}),
		DBusObjectPath,
		"org.freedesktop.DBus.Introspectable",
	); err != nil {
		return fmt.Errorf("failed to export introspection: %w", err)
	}

	// Экспорт Properties interface для Manager
	if err := s.conn.Export(
		&s.manager.properties,
		DBusObjectPath,
		PropertiesInterface,
	); err != nil {
		return fmt.Errorf("failed to export properties: %w", err)
	}

	return nil
}

// Stop останавливает сервис
func (s *Service) Stop() {
	s.cancel()
}

// getAuthorizer возвращает авторизатор.
// Без явной инициализации (Run не вызывался) — fail-closed:
func (s *Service) getAuthorizer() Authorizer {
	if s.authorizer != nil {
		return s.authorizer
	}
	// Без явной инициализации (Run не вызывался) — fail-closed:
	// PolicyKitAuthorizer с nil-соединением всегда отказывает
	return NewPolicyKitAuthorizer(s.conn)
}

// checkAuthorization резолвит uid вызвавшего и проверяет авторизацию
// действия через polkit. Возвращает ошибку при отказе.
func (s *Service) checkAuthorization(sender dbus.Sender, actionID string) error {
	_, err := s.authorizeCaller(sender, actionID)
	return err
}

// authorizeCaller — то же, что checkAuthorization, но возвращает uid
// вызвавшего для дальнейшего использования (сборка от его uid).
func (s *Service) authorizeCaller(sender dbus.Sender, actionID string) (uint32, error) {
	uid, err := resolveCallerUID(s.conn, sender)
	if err != nil {
		slog.Warn("Failed to resolve caller uid", "sender", sender, "action", actionID, "err", err)
		return 0, fmt.Errorf("failed to resolve caller: %w", err)
	}

	authorized, err := s.getAuthorizer().CheckAuthorization(actionID, uid, nil, true)
	if err != nil {
		slog.Error("Authorization check failed", "action", actionID, "uid", uid, "err", err)
		return 0, fmt.Errorf("authorization check failed: %w", err)
	}
	if !authorized {
		slog.Warn("Authorization denied", "action", actionID, "uid", uid)
		return 0, fmt.Errorf("not authorized for action %s", actionID)
	}

	return uid, nil
}

// GetDeps возвращает ALR зависимости
func (s *Service) GetDeps() *appbuilder.AppDeps {
	return s.deps
}

// GetConn возвращает D-Bus соединение
func (s *Service) GetConn() *dbus.Conn {
	return s.conn
}

// GetConfig возвращает конфигурацию
func (s *Service) GetConfig() *config.ALRConfig {
	return s.config
}

// NextJobID возвращает следующий ID задачи
func (s *Service) NextJobID() uint32 {
	return atomic.AddUint32(&s.jobIDCounter, 1)
}

// RegisterJob регистрирует новую задачу
func (s *Service) RegisterJob(job *DBusJob) dbus.ObjectPath {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := GetJobObjectPath(job.info.ID)
	s.jobs[path] = job

	// Экспорт задачи
	go s.exportJob(job, path)

	return path
}

// exportJob экспортирует задачу на шину
func (s *Service) exportJob(job *DBusJob, path dbus.ObjectPath) {
	if s.conn == nil {
		return
	}
	if err := s.conn.Export(job, path, JobInterfaceName); err != nil {
		slog.Error("Failed to export job", "path", path, "err", err)
		return
	}

	if err := s.conn.Export(
		introspect.NewIntrospectable(&introspect.Node{
			Interfaces: []introspect.Interface{job.IntrospectionData()},
		}),
		path,
		"org.freedesktop.DBus.Introspectable",
	); err != nil {
		slog.Error("Failed to export job introspection", "path", path, "err", err)
	}
}

// UnregisterJob удаляет задачу из реестра
func (s *Service) UnregisterJob(path dbus.ObjectPath) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.jobs, path)

	// Удалить экспорт
	if s.conn != nil {
		s.conn.Export(nil, path, JobInterfaceName)
	}
}

// GetJob возвращает задачу по пути
func (s *Service) GetJob(path dbus.ObjectPath) (*DBusJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, ok := s.jobs[path]
	return job, ok
}

// RegisterPackage регистрирует пакет
func (s *Service) RegisterPackage(pkg *DBusPackage) dbus.ObjectPath {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := GetPackageObjectPath(pkg.repository, pkg.name)
	s.packages[path] = pkg

	// Экспорт пакета
	go s.exportPackage(pkg, path)

	return path
}

// exportPackage экспортирует пакет на шину
func (s *Service) exportPackage(pkg *DBusPackage, path dbus.ObjectPath) {
	if s.conn == nil {
		return
	}
	if err := s.conn.Export(pkg, path, PackageInterfaceName); err != nil {
		slog.Error("Failed to export package", "path", path, "err", err)
		return
	}

	if err := s.conn.Export(
		introspect.NewIntrospectable(&introspect.Node{
			Interfaces: []introspect.Interface{pkg.IntrospectionData()},
		}),
		path,
		"org.freedesktop.DBus.Introspectable",
	); err != nil {
		slog.Error("Failed to export package introspection", "path", path, "err", err)
	}
}

// GetPackage возвращает пакет по пути
func (s *Service) GetPackage(path dbus.ObjectPath) (*DBusPackage, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pkg, ok := s.packages[path]
	return pkg, ok
}

// EmitSignal отправляет сигнал
func (s *Service) EmitSignal(path dbus.ObjectPath, iface, name string, values ...interface{}) {
	if s.conn == nil {
		return
	}

	if err := s.conn.Emit(path, iface+"."+name, values...); err != nil {
		slog.Debug("Failed to emit signal", "signal", name, "err", err)
	}
}

// Notify отправляет уведомление
func (s *Service) Notify(title, body string, urgency int32) {
	if s.notifier != nil {
		s.notifier.Notify(title, body, urgency)
	}
}

// Context возвращает контекст сервиса
func (s *Service) Context() context.Context {
	return s.ctx
}

// CreateIntrospectableNode создает Node для introspection
func CreateIntrospectableNode(ifaces ...introspect.Interface) *introspect.Node {
	return &introspect.Node{
		Name:       DBusObjectPath,
		Interfaces: ifaces,
	}
}
