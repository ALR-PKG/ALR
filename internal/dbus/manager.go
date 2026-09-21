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
	"fmt"
	"log/slog"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/search"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

// DBusManager реализует ru.alr_pkg.ALR.Manager интерфейс
type DBusManager struct {
	service    *Service
	properties *prop.Properties
}

// NewDBusManager создает новый Manager
func NewDBusManager(service *Service) *DBusManager {
	propsSpec := map[string]map[string]*prop.Prop{
		ManagerInterfaceName: {
			"Version": {
				Value:    config.Version,
				Writable: false,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
			"ActiveJobsCount": {
				Value:    uint32(0),
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}

	return &DBusManager{
		service:    service,
		properties: prop.New(service.GetConn(), DBusObjectPath, propsSpec),
	}
}

// SearchPackages ищет пакеты по запросу
func (m *DBusManager) SearchPackages(query string, filters map[string]dbus.Variant) ([]PackageInfo, *dbus.Error) {
	slog.Debug("SearchPackages called", "query", query, "filters", filters)

	deps := m.service.GetDeps()
	if deps == nil || deps.DB == nil {
		return nil, dbus.NewError(ErrorNotInitialized, []interface{}{"service not initialized"})
	}

	// Создаем поисковик
	s := search.New(deps.DB)

	// Строим опции поиска
	optsBuilder := search.NewSearchOptions()

	// Если есть фильтры, применяем их
	if name, ok := filters["name"]; ok {
		if nameStr, ok := name.Value().(string); ok {
			optsBuilder = optsBuilder.WithName(nameStr)
		}
	}
	if desc, ok := filters["description"]; ok {
		if descStr, ok := desc.Value().(string); ok {
			optsBuilder = optsBuilder.WithDescription(descStr)
		}
	}
	if repo, ok := filters["repository"]; ok {
		if repoStr, ok := repo.Value().(string); ok {
			optsBuilder = optsBuilder.WithRepository(repoStr)
		}
	}
	if provides, ok := filters["provides"]; ok {
		if providesStr, ok := provides.Value().(string); ok {
			optsBuilder = optsBuilder.WithProvides(providesStr)
		}
	}

	// Если query не пустой и нет специфических фильтров, ищем по имени
	if query != "" && filters["name"].Signature().String() == "" {
		optsBuilder = optsBuilder.WithName(query)
	}

	// Выполняем поиск
	ctx := m.service.Context()
	packages, err := s.Search(ctx, optsBuilder.Build())
	if err != nil {
		slog.Error("Search failed", "err", err)
		return nil, dbus.NewError(ErrorSearchFailed, []interface{}{err.Error()})
	}

	// Конвертируем в PackageInfo
	results := make([]PackageInfo, 0, len(packages))
	for _, pkg := range packages {
		info := m.convertToPackageInfo(&pkg)
		results = append(results, info)
	}

	return results, nil
}

// GetPackage возвращает object path для пакета
func (m *DBusManager) GetPackage(name, repository string) (dbus.ObjectPath, *dbus.Error) {
	// Проверяем, есть ли уже такой пакет
	path := GetPackageObjectPath(repository, name)
	if pkg, ok := m.service.GetPackage(path); ok {
		_ = pkg // используем для проверки существования
		return path, nil
	}

	// Ищем пакет в БД
	deps := m.service.GetDeps()
	if deps == nil || deps.DB == nil {
		return "", dbus.NewError(ErrorNotInitialized, []interface{}{"service not initialized"})
	}

	dbPkg, err := deps.DB.GetPkg("name = ? AND repository = ?", name, repository)
	if err != nil || dbPkg == nil {
		return "", dbus.NewError(ErrorPackageNotFound, []interface{}{fmt.Sprintf("package %s/%s not found", repository, name)})
	}

	// Создаем и регистрируем D-Bus объект пакета
	dbusPkg := NewDBusPackage(m.service, dbPkg)
	path = m.service.RegisterPackage(dbusPkg)

	return path, nil
}

// ListRepositories возвращает список репозиториев
func (m *DBusManager) ListRepositories() ([]RepositoryInfo, *dbus.Error) {
	cfg := m.service.GetConfig()
	if cfg == nil {
		return nil, dbus.NewError(ErrorNotInitialized, []interface{}{"config not loaded"})
	}

	repos := cfg.Repos()
	results := make([]RepositoryInfo, 0, len(repos))
	for _, repo := range repos {
		results = append(results, RepositoryInfo{
			Name:    repo.Name,
			URL:     repo.URL,
			Ref:     repo.Ref,
			Mirrors: repo.Mirrors,
		})
	}

	return results, nil
}

// RefreshRepositories обновляет все репозитории
func (m *DBusManager) RefreshRepositories(sender dbus.Sender) (dbus.ObjectPath, *dbus.Error) {
	slog.Info("RefreshRepositories called", "sender", sender)

	// Проверяем авторизацию через PolicyKit по uid вызвавшего
	if err := m.service.checkAuthorization(sender, ActionRefresh); err != nil {
		return "", dbus.NewError(ErrorNotAuthorized, []interface{}{err.Error()})
	}

	deps := m.service.GetDeps()
	if deps == nil || deps.Repos == nil {
		return "", dbus.NewError(ErrorNotInitialized, []interface{}{"service not initialized"})
	}

	// Создаем задачу обновления
	jobID := m.service.NextJobID()
	job := NewDBusJob(m.service, jobID, JobTypeRefresh, "", "")

	// Запускаем асинхронно
	go func() {
		job.SetStatus(JobStatusRunning)
		job.SetProgress(0.0, "Starting repository refresh...")

		ctx := m.service.Context()
		repos := m.service.GetConfig().Repos()

		if err := deps.Repos.Pull(ctx, repos); err != nil {
			job.SetFailed(err.Error())
			m.service.Notify("ALR Error", fmt.Sprintf("Failed to refresh repositories: %v", err), UrgencyCritical)
			return
		}

		job.SetProgress(1.0, "Repositories updated successfully")
		job.SetCompleted()

		m.service.Notify("ALR", "Repositories updated successfully", UrgencyNormal)
		m.EmitSignal(DBusObjectPath, ManagerInterfaceName, ManagerSignalRepositoryUpdated, "", "")
	}()

	path := m.service.RegisterJob(job)
	return path, nil
}

// GetVersion возвращает версию ALR
func (m *DBusManager) GetVersion() (string, *dbus.Error) {
	return config.Version, nil
}

// ListJobs возвращает список активных задач
func (m *DBusManager) ListJobs() ([]dbus.ObjectPath, *dbus.Error) {
	// Это будет реализовано через итерацию по jobs map в service
	return []dbus.ObjectPath{}, nil
}

// GetActiveJob возвращает текущую активную задачу
func (m *DBusManager) GetActiveJob() (dbus.ObjectPath, bool, *dbus.Error) {
	// TODO: реализовать отслеживание активной задачи
	return "", false, nil
}

// UpgradeAll обновляет все пакеты
func (m *DBusManager) UpgradeAll(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	// TODO: реализовать обновление всех пакетов
	return "", dbus.NewError(ErrorNotImplemented, []interface{}{"UpgradeAll not yet implemented"})
}

// convertToPackageInfo конвертирует alrsh.Package в PackageInfo
func (m *DBusManager) convertToPackageInfo(pkg *alrsh.Package) PackageInfo {
	// Проверяем установлен ли пакет
	deps := m.service.GetDeps()
	installed := false
	if deps != nil && deps.Manager != nil {
		fullName := fmt.Sprintf("alr-%s+%s", pkg.Name, pkg.Repository)
		if isInstalled, err := deps.Manager.IsInstalled(fullName); err == nil {
			installed = isInstalled
		}
	}

	return PackageInfo{
		Name:        pkg.Name,
		Repository:  pkg.Repository,
		Version:     pkg.Version,
		Release:     uint32(pkg.Release),
		Epoch:       uint32(pkg.Epoch),
		Description: pkg.Description.Resolved(),
		Summary:     pkg.Summary.Resolved(),
		Homepage:    pkg.Homepage.Resolved(),
		Licenses:    pkg.Licenses,
		Installed:   installed,
		Size:        0, // TODO: получать размер из БД или вычислять
	}
}

// EmitSignal отправляет сигнал от имени менеджера
func (m *DBusManager) EmitSignal(path dbus.ObjectPath, iface, name string, values ...interface{}) {
	m.service.EmitSignal(path, iface, name, values...)
}

// IntrospectionData возвращает данные для introspection
func (m *DBusManager) IntrospectionData() introspect.Interface {
	return introspect.Interface{
		Name: ManagerInterfaceName,
		Methods: []introspect.Method{
			{
				Name: "SearchPackages",
				Args: []introspect.Arg{
					{Name: "query", Type: "s", Direction: "in"},
					{Name: "filters", Type: "a{sv}", Direction: "in"},
					{Name: "packages", Type: "a(osss)", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "GetPackage",
				Args: []introspect.Arg{
					{Name: "name", Type: "s", Direction: "in"},
					{Name: "repository", Type: "s", Direction: "in"},
					{Name: "package", Type: "o", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "ListRepositories",
				Args: []introspect.Arg{
					{Name: "repos", Type: "a(sssas)", Direction: "out"},
				},
			},
			{
				Name: "RefreshRepositories",
				Args: []introspect.Arg{
					{Name: "job", Type: "o", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "GetVersion",
				Args: []introspect.Arg{
					{Name: "version", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "ListJobs",
				Args: []introspect.Arg{
					{Name: "jobs", Type: "ao", Direction: "out"},
				},
			},
			{
				Name: "GetActiveJob",
				Args: []introspect.Arg{
					{Name: "job", Type: "o", Direction: "out"},
					{Name: "has_active", Type: "b", Direction: "out"},
				},
			},
			{
				Name: "UpgradeAll",
				Args: []introspect.Arg{
					{Name: "options", Type: "a{sv}", Direction: "in"},
					{Name: "job", Type: "o", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
		},
		Signals: []introspect.Signal{
			{
				Name: "RepositoryAdded",
				Args: []introspect.Arg{
					{Name: "name", Type: "s"},
					{Name: "url", Type: "s"},
				},
			},
			{
				Name: "RepositoryRemoved",
				Args: []introspect.Arg{
					{Name: "name", Type: "s"},
				},
			},
			{
				Name: "PackageInstalled",
				Args: []introspect.Arg{
					{Name: "name", Type: "s"},
					{Name: "repository", Type: "s"},
					{Name: "version", Type: "s"},
				},
			},
			{
				Name: "PackageRemoved",
				Args: []introspect.Arg{
					{Name: "name", Type: "s"},
					{Name: "repository", Type: "s"},
				},
			},
		},
		Properties: []introspect.Property{
			{
				Name:   "Version",
				Type:   "s",
				Access: "read",
			},
			{
				Name:   "ActiveJobsCount",
				Type:   "u",
				Access: "read",
			},
		},
	}
}
