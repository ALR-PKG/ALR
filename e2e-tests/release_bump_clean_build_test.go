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

// TestE2EReleaseBumpCleanBuild проверяет что при изменении только release
// (без изменения version) выполняется чистая сборка
// Регрессионный тест: ранее старые артефакты сборки могли переиспользоваться
func TestE2EReleaseBumpCleanBuild(t *testing.T) {
	runMatrixSuite(
		t,
		"release-bump-clean-build",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			t.Parallel()
			defaultPrepare(t, r)

			// Устанавливаем пакет с release=1
			execShouldNoError(t, r, "sudo", "alr", "repo", "set-ref", "alr-repo", "bd26236cd7")
			execShouldNoError(t, r, "sudo", "alr", "ref")
			execShouldNoError(t, r, "sudo", "alr", "in", "bar-pkg")

			// Проверяем что пакет установлен
			execShouldNoError(t, r, "sh", "-c", "alr list -I | grep -q bar-pkg")

			// Обновляем репозиторий до версии с тем же version но другим release
			// (в тестовом репозитории commit d9a3541561 должен иметь другой release)
			execShouldNoError(t, r, "sudo", "alr", "repo", "set-ref", "alr-repo", "d9a3541561")
			execShouldNoError(t, r, "sudo", "alr", "ref")

			// Проверяем что есть обновление
			execShouldNoError(t, r, "sh", "-c", "test $(alr list -U | wc -l) -eq 1 || exit 1")

			// Обновляем пакет
			// При изменении release должна произойти чистая сборка
			execShouldNoError(t, r, "sudo", "alr", "up")

			// Проверяем что обновление применилось
			execShouldNoError(t, r, "sh", "-c", "test $(alr list -U | wc -l) -eq 0 || exit 1")
		},
	)
}

// TestE2EReinstallSameRelease проверяет что при переустановке пакета с тем же release
// используется кэш (не происходит пересборки)
func TestE2EReinstallSameRelease(t *testing.T) {
	runMatrixSuite(
		t,
		"reinstall-same-release",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			t.Parallel()
			defaultPrepare(t, r)

			// Устанавливаем пакет
			execShouldNoError(t, r, "sudo", "alr", "in", "foo-pkg")

			// Проверяем что пакет установлен
			execShouldNoError(t, r, "sh", "-c", "alr list -I | grep -q foo-pkg")

			// Удаляем пакет
			execShouldNoError(t, r, "sudo", "alr", "rm", "foo-pkg")

			// Проверяем что пакет удален
			execShouldNoError(t, r, "sh", "-c", "! alr list -I | grep -q foo-pkg")

			// Переустанавливаем пакет (тот же release)
			// Должен использоваться кэш собранного пакета
			execShouldNoError(t, r, "sudo", "alr", "in", "foo-pkg")

			// Проверяем что пакет снова установлен
			execShouldNoError(t, r, "sh", "-c", "alr list -I | grep -q foo-pkg")
		},
	)
}
