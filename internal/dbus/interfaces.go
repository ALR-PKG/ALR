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
	"github.com/godbus/dbus/v5"
)

// ManagerInterface определяет методы для ru.alr_pkg.ALR.Manager
type ManagerInterface interface {
	// SearchPackages ищет пакеты по запросу
	// query - строка поиска
	// filters - map с фильтрами (name, description, repository, provides)
	// Возвращает массив пакетов и ошибку
	SearchPackages(query string, filters map[string]dbus.Variant) ([]PackageInfo, *dbus.Error)

	// GetPackage возвращает object path для пакета
	GetPackage(name, repository string) (dbus.ObjectPath, *dbus.Error)

	// ListRepositories возвращает список репозиториев
	ListRepositories() ([]RepositoryInfo, *dbus.Error)

	// RefreshRepositories обновляет все репозитории
	// Возвращает object path задачи
	RefreshRepositories() (dbus.ObjectPath, *dbus.Error)

	// GetVersion возвращает версию ALR
	GetVersion() (string, *dbus.Error)

	// ListJobs возвращает список активных задач
	ListJobs() ([]dbus.ObjectPath, *dbus.Error)

	// GetActiveJob возвращает текущую активную задачу
	GetActiveJob() (dbus.ObjectPath, bool, *dbus.Error)

	// UpgradeAll обновляет все пакеты
	// Возвращает object path задачи
	UpgradeAll(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error)
}

// PackageInterface определяет методы для ru.alr_pkg.ALR.Package
type PackageInterface interface {
	// GetDetails возвращает детальную информацию о пакете
	GetDetails() (map[string]dbus.Variant, *dbus.Error)

	// Install устанавливает пакет
	// options - опции установки (clean, interactive)
	// Возвращает object path задачи
	Install(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error)

	// Remove удаляет пакет
	// options - опции удаления
	// Возвращает object path задачи
	Remove(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error)

	// Build собирает пакет
	// options - опции сборки
	// Возвращает object path задачи
	Build(options map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error)

	// CheckForUpdates проверяет наличие обновлений
	// Возвращает (доступно_ли_обновление, новая_версия, ошибка)
	CheckForUpdates() (bool, string, *dbus.Error)

	// GetDependencies возвращает зависимости пакета
	GetDependencies() ([]string, *dbus.Error)
}

// JobInterface определяет методы для ru.alr_pkg.ALR.Job
type JobInterface interface {
	// Cancel отменяет задачу
	Cancel() *dbus.Error

	// Wait ожидает завершения задачи с таймаутом
	// timeout - таймаут в секундах (0 = бесконечно)
	// Возвращает (успех, сообщение_об_ошибке)
	Wait(timeout int32) (bool, string, *dbus.Error)

	// GetInfo возвращает информацию о задаче
	GetInfo() (JobInfo, *dbus.Error)
}

// Signals для Manager
const (
	// ManagerSignalRepositoryAdded сигнал добавления репозитория
	ManagerSignalRepositoryAdded = "RepositoryAdded"
	// ManagerSignalRepositoryRemoved сигнал удаления репозитория
	ManagerSignalRepositoryRemoved = "RepositoryRemoved"
	// ManagerSignalRepositoryUpdated сигнал обновления репозитория
	ManagerSignalRepositoryUpdated = "RepositoryUpdated"
	// ManagerSignalPackageInstalled сигнал установки пакета
	ManagerSignalPackageInstalled = "PackageInstalled"
	// ManagerSignalPackageRemoved сигнал удаления пакета
	ManagerSignalPackageRemoved = "PackageRemoved"
)

// Signals для Job
const (
	// JobSignalProgressChanged сигнал изменения прогресса
	JobSignalProgressChanged = "ProgressChanged"
	// JobSignalStatusChanged сигнал изменения статуса
	JobSignalStatusChanged = "StatusChanged"
	// JobSignalCompleted сигнал завершения задачи
	JobSignalCompleted = "Completed"
)

// PropertyChanged сигнал изменения свойства
type PropertyChanged struct {
	Interface             string
	ChangedProperties     map[string]dbus.Variant
	InvalidatedProperties []string
}
