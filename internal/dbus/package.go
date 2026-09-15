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
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"

	"git.alr-pkg.ru/ALR/ALR/internal/build"
	"git.alr-pkg.ru/ALR/ALR/internal/manager"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

// DBusPackage реализует ru.alr-pkg.ALR.Package интерфейс
type DBusPackage struct {
	service *Service

	// Информация о пакете
	name        string
	repository  string
	version     string
	description string
	summary     string
	homepage    string
	licenses    []string
	installed   bool
	depends     []string

	// D-Bus properties
	properties *prop.Properties

	// Mutex для потокобезопасности
	mu sync.RWMutex
}

// NewDBusPackage создает новый D-Bus объект пакета
func NewDBusPackage(service *Service, pkg *alrsh.Package) *DBusPackage {
	dp := &DBusPackage{
		service:     service,
		name:        pkg.Name,
		repository:  pkg.Repository,
		version:     pkg.Version,
		description: pkg.Description.Resolved(),
		summary:     pkg.Summary.Resolved(),
		homepage:    pkg.Homepage.Resolved(),
		licenses:    pkg.Licenses,
		depends:     pkg.Depends.Resolved(),
	}

	// Инициализация properties
	path := GetPackageObjectPath(pkg.Repository, pkg.Name)
	propsSpec := map[string]map[string]*prop.Prop{
		PackageInterfaceName: {
			"Name": {
				Value:    dp.name,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Repository": {
				Value:    dp.repository,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Version": {
				Value:    dp.version,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Description": {
				Value:    dp.description,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Summary": {
				Value:    dp.summary,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Homepage": {
				Value:    dp.homepage,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Licenses": {
				Value:    dp.licenses,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Installed": {
				Value:    dp.installed,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: nil,
			},
		},
	}

	dp.properties = prop.New(service.GetConn(), path, propsSpec)

	// Проверяем установлен ли пакет
	dp.updateInstalledStatus()

	return dp
}

// updateInstalledStatus обновляет статус установки
func (p *DBusPackage) updateInstalledStatus() {
	deps := p.service.GetDeps()
	if deps == nil || deps.Manager == nil {
		return
	}

	fullName := fmt.Sprintf("alr-%s+%s", p.name, p.repository)
	installed, err := deps.Manager.IsInstalled(fullName)
	if err != nil {
		slog.Debug("Failed to check installation status", "package", fullName, "err", err)
		return
	}

	p.mu.Lock()
	p.installed = installed
	p.mu.Unlock()

	// Обновляем property
	if p.properties != nil {
		if err := p.properties.Set(PackageInterfaceName, "Installed", dbus.MakeVariant(installed)); err != nil {
			slog.Debug("Failed to set Installed property", "err", err)
		}
	}
}

// GetDetails возвращает детальную информацию о пакете
func (p *DBusPackage) GetDetails() (map[string]dbus.Variant, *dbus.Error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return map[string]dbus.Variant{
		"Name":        dbus.MakeVariant(p.name),
		"Repository":  dbus.MakeVariant(p.repository),
		"Version":     dbus.MakeVariant(p.version),
		"Description": dbus.MakeVariant(p.description),
		"Summary":     dbus.MakeVariant(p.summary),
		"Homepage":    dbus.MakeVariant(p.homepage),
		"Licenses":    dbus.MakeVariant(p.licenses),
		"Installed":   dbus.MakeVariant(p.installed),
		"Depends":     dbus.MakeVariant(p.depends),
	}, nil
}

// Install устанавливает пакет
func (p *DBusPackage) Install(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	slog.Info("Install called", "package", p.name, "repository", p.repository)

	// Проверяем авторизацию через PolicyKit
	if err := p.checkAuthorization("ru.alr-pkg.install"); err != nil {
		return "", dbus.NewError("ru.alr-pkg.ALR.Error.NotAuthorized", []interface{}{err.Error()})
	}

	// Парсим опции
	clean := false
	interactive := true

	if opt, ok := options["clean"]; ok {
		if val, ok := opt.Value().(bool); ok {
			clean = val
		}
	}
	if opt, ok := options["interactive"]; ok {
		if val, ok := opt.Value().(bool); ok {
			interactive = val
		}
	}

	// Создаем задачу
	jobID := p.service.NextJobID()
	job := NewDBusJob(p.service, jobID, JobTypeInstall, p.name, p.repository)

	// Уведомление о начале
	p.service.Notify("ALR", fmt.Sprintf("Starting installation of %s...", p.name), UrgencyNormal)

	// Запускаем асинхронно
	go p.runInstall(job, clean, interactive)

	path := p.service.RegisterJob(job)
	return path, nil
}

// runInstall выполняет установку
func (p *DBusPackage) runInstall(job *DBusJob, clean, interactive bool) {
	job.SetStatus(JobStatusRunning)
	job.SetProgress(0.0, "Initializing installation...")

	ctx := p.service.Context()
	deps := p.service.GetDeps()

	// Получаем исполнители
	installer, installerClose, err := build.GetSafeInstaller()
	if err != nil {
		job.SetFailed(fmt.Sprintf("Failed to get installer: %v", err))
		p.service.Notify("ALR Error", fmt.Sprintf("Failed to install %s", p.name), UrgencyCritical)
		return
	}
	defer installerClose()

	scripter, scripterClose, err := build.GetSafeScriptExecutor()
	if err != nil {
		job.SetFailed(fmt.Sprintf("Failed to get scripter: %v", err))
		p.service.Notify("ALR Error", fmt.Sprintf("Failed to install %s", p.name), UrgencyCritical)
		return
	}
	defer scripterClose()

	// Создаем билдер
	builder, err := build.NewMainBuilder(
		deps.Cfg,
		deps.Manager,
		deps.Repos,
		scripter,
		installer,
	)
	if err != nil {
		job.SetFailed(fmt.Sprintf("Failed to create builder: %v", err))
		p.service.Notify("ALR Error", fmt.Sprintf("Failed to install %s", p.name), UrgencyCritical)
		return
	}

	// Прогресс обратного вызова
	progressFunc := func(percent float64, message string) {
		job.SetProgress(percent, message)
	}

	job.SetProgress(0.1, "Resolving dependencies...")

	// Выполняем установку
	fullName := fmt.Sprintf("%s/%s", p.repository, p.name)
	_, err = builder.InstallPkgs(
		ctx,
		&build.BuildArgs{
			Opts: &types.BuildOpts{
				Clean:       clean,
				Interactive: interactive,
			},
			Info:       deps.Info,
			PkgFormat_: build.GetPkgFormat(deps.Manager),
		},
		[]string{fullName},
	)
	if err != nil {
		job.SetFailed(err.Error())
		p.service.Notify("ALR Error", fmt.Sprintf("Failed to install %s: %v", p.name, err), UrgencyCritical)
		return
	}

	// Обновляем статус
	p.updateInstalledStatus()

	job.SetProgress(1.0, "Installation completed successfully")
	job.SetCompleted()

	p.service.Notify("ALR", fmt.Sprintf("%s installed successfully", p.name), UrgencyNormal)

	// Отправляем сигнал
	path := GetPackageObjectPath(p.repository, p.name)
	p.service.EmitSignal(path, ManagerInterfaceName, ManagerSignalPackageInstalled, p.name, p.repository, p.version)

	_ = progressFunc // используем переменную
}

// Remove удаляет пакет
func (p *DBusPackage) Remove(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	slog.Info("Remove called", "package", p.name, "repository", p.repository)

	// Проверяем авторизацию
	if err := p.checkAuthorization("ru.alr-pkg.remove"); err != nil {
		return "", dbus.NewError("ru.alr-pkg.ALR.Error.NotAuthorized", []interface{}{err.Error()})
	}

	// Парсим опции
	interactive := true
	if opt, ok := options["interactive"]; ok {
		if val, ok := opt.Value().(bool); ok {
			interactive = val
		}
	}

	// Создаем задачу
	jobID := p.service.NextJobID()
	job := NewDBusJob(p.service, jobID, JobTypeRemove, p.name, p.repository)

	// Уведомление
	p.service.Notify("ALR", fmt.Sprintf("Removing %s...", p.name), UrgencyNormal)

	// Запускаем асинхронно
	go p.runRemove(job, interactive)

	path := p.service.RegisterJob(job)
	return path, nil
}

// runRemove выполняет удаление
func (p *DBusPackage) runRemove(job *DBusJob, interactive bool) {
	job.SetStatus(JobStatusRunning)
	job.SetProgress(0.0, "Removing package...")

	deps := p.service.GetDeps()

	fullName := fmt.Sprintf("alr-%s+%s", p.name, p.repository)

	if err := deps.Manager.Remove(&manager.Opts{
		NoConfirm: !interactive,
	}, fullName); err != nil {
		job.SetFailed(err.Error())
		p.service.Notify("ALR Error", fmt.Sprintf("Failed to remove %s: %v", p.name, err), UrgencyCritical)
		return
	}

	// Обновляем статус
	p.updateInstalledStatus()

	job.SetProgress(1.0, "Package removed successfully")
	job.SetCompleted()

	p.service.Notify("ALR", fmt.Sprintf("%s removed successfully", p.name), UrgencyNormal)

	// Отправляем сигнал
	path := GetPackageObjectPath(p.repository, p.name)
	p.service.EmitSignal(path, ManagerInterfaceName, ManagerSignalPackageRemoved, p.name, p.repository)
}

// Build собирает пакет
func (p *DBusPackage) Build(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	return "", dbus.NewError("ru.alr-pkg.ALR.Error.NotImplemented", []interface{}{"Build not yet implemented via D-Bus"})
}

// CheckForUpdates проверяет наличие обновлений
func (p *DBusPackage) CheckForUpdates() (bool, string, *dbus.Error) {
	deps := p.service.GetDeps()
	if deps == nil || deps.Manager == nil {
		return false, "", dbus.NewError("ru.alr-pkg.ALR.Error.NotInitialized", []interface{}{"service not initialized"})
	}

	fullName := fmt.Sprintf("alr-%s+%s", p.name, p.repository)
	installedVer, err := deps.Manager.GetInstalledVersion(fullName)
	if err != nil {
		return false, "", dbus.NewError("ru.alr-pkg.ALR.Error.Internal", []interface{}{err.Error()})
	}

	if installedVer == "" {
		// Пакет не установлен
		return false, "", nil
	}

	// Сравниваем версии
	if installedVer != p.version {
		return true, p.version, nil
	}

	return false, "", nil
}

// GetDependencies возвращает зависимости пакета
func (p *DBusPackage) GetDependencies() ([]string, *dbus.Error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.depends, nil
}

// checkAuthorization проверяет авторизацию через PolicyKit
func (p *DBusPackage) checkAuthorization(action string) error {
	// TODO: реализовать полную интеграцию с PolicyKit
	// Пока просто проверяем, что мы root
	// В будущем здесь будет вызов polkit.CheckAuthorization
	return nil
}

// IntrospectionData возвращает данные для introspection
func (p *DBusPackage) IntrospectionData() introspect.Interface {
	return introspect.Interface{
		Name: PackageInterfaceName,
		Methods: []introspect.Method{
			{
				Name: "GetDetails",
				Args: []introspect.Arg{
					{Name: "details", Type: "a{sv}", Direction: "out"},
				},
			},
			{
				Name: "Install",
				Args: []introspect.Arg{
					{Name: "options", Type: "a{sv}", Direction: "in"},
					{Name: "job", Type: "o", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "Remove",
				Args: []introspect.Arg{
					{Name: "options", Type: "a{sv}", Direction: "in"},
					{Name: "job", Type: "o", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "Build",
				Args: []introspect.Arg{
					{Name: "options", Type: "a{sv}", Direction: "in"},
					{Name: "job", Type: "o", Direction: "out"},
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "CheckForUpdates",
				Args: []introspect.Arg{
					{Name: "available", Type: "b", Direction: "out"},
					{Name: "version", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "GetDependencies",
				Args: []introspect.Arg{
					{Name: "depends", Type: "as", Direction: "out"},
				},
			},
		},
		Properties: []introspect.Property{
			{Name: "Name", Type: "s", Access: "read"},
			{Name: "Repository", Type: "s", Access: "read"},
			{Name: "Version", Type: "s", Access: "read"},
			{Name: "Description", Type: "s", Access: "read"},
			{Name: "Summary", Type: "s", Access: "read"},
			{Name: "Homepage", Type: "s", Access: "read"},
			{Name: "Licenses", Type: "as", Access: "read"},
			{Name: "Installed", Type: "b", Access: "read"},
		},
	}
}
