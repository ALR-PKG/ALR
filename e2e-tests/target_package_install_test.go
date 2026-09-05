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

//go:build e2e

package e2etests_test

import (
	"testing"

	"go.alt-gnome.ru/capytest"
)

// TestE2ETargetPackageInstall проверяет что целевые пакеты действительно устанавливаются
// Регрессионный тест для бага: целевые пакеты не устанавливались из-за несовпадения имён
// в targetSet (полное имя alr-bin+alr-default vs короткое имя alr-bin)
func TestE2ETargetPackageInstall(t *testing.T) {
	runMatrixSuite(
		t,
		"target-package-install",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			t.Parallel()
			defaultPrepare(t, r)

			// Устанавливаем пакет
			execShouldNoError(t, r, "sudo", "alr", "in", "foo-pkg")

			// Проверяем что пакет установлен (должен быть в списке установленных)
			execShouldNoError(t, r, "sh", "-c", "alr list -I | grep -q foo-pkg")

			// Проверяем что файлы пакета действительно установлены в систему
			// (foo-pkg из тестового репозитория устанавливает файл /opt/foo)
			execShouldNoError(t, r, "sh", "-c", "test -f /opt/foo")
		},
	)
}

// TestE2ETargetPackageUpgrade проверяет что целевые пакеты обновляются корректно
// При обновлении пакет должен быть переустановлен
func TestE2ETargetPackageUpgrade(t *testing.T) {
	runMatrixSuite(
		t,
		"target-package-upgrade",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			t.Parallel()
			defaultPrepare(t, r)

			// Устанавливаем старую версию
			execShouldNoError(t, r, "sudo", "alr", "repo", "set-ref", "alr-repo", "bd26236cd7")
			execShouldNoError(t, r, "sudo", "alr", "ref")
			execShouldNoError(t, r, "sudo", "alr", "in", "bar-pkg")

			// Проверяем что пакет установлен
			execShouldNoError(t, r, "sh", "-c", "alr list -I | grep -q bar-pkg")

			// Обновляем репозиторий до новой версии
			execShouldNoError(t, r, "sudo", "alr", "repo", "set-ref", "alr-repo", "d9a3541561")
			execShouldNoError(t, r, "sudo", "alr", "ref")

			// Проверяем что есть обновление
			execShouldNoError(t, r, "sh", "-c", "test $(alr list -U | wc -l) -eq 1 || exit 1")

			// Выполняем обновление
			execShouldNoError(t, r, "sudo", "alr", "up")

			// Проверяем что обновление применилось (список обновлений пуст)
			execShouldNoError(t, r, "sh", "-c", "test $(alr list -U | wc -l) -eq 0 || exit 1")
		},
	)
}
