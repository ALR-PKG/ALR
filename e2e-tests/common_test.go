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
	"fmt"
	"os"
	"testing"

	"go.alt-gnome.ru/capytest"
	"go.alt-gnome.ru/capytest/providers/podman"
)

// e2eImage возвращает полное имя тестового образа. Префикс реестра
// можно переопределить через ALR_E2E_IMAGE_PREFIX.
func e2eImage(name string) string {
	prefix := os.Getenv("ALR_E2E_IMAGE_PREFIX")
	if prefix == "" {
		prefix = "git.alr-pkg.ru/plemya-x/alr-e2e-test-image"
	}
	return fmt.Sprintf("%s-%s:latest", prefix, name)
}

var ALL_SYSTEMS []string = []string{
	"ubuntu-24.04",
	"alt-sisyphus",
	"fedora-41",
	"archlinux",
	"alpine",
	"opensuse-leap",
}

var AUTOREQ_AUTOPROV_SYSTEMS []string = []string{
	"alt-sisyphus",
	"fedora-41",
	"opensuse-leap",
}

var RPM_SYSTEMS []string = []string{
	"alt-sisyphus",
	"fedora-41",
	"opensuse-leap",
}

var COMMON_SYSTEMS []string = []string{
	"ubuntu-24.04",
}

func execShouldNoError(t *testing.T, r capytest.Runner, cmd string, args ...string) {
	t.Helper()
	r.Command(cmd, args...).ExpectSuccess().Run(t)
}

func execShouldError(t *testing.T, r capytest.Runner, cmd string, args ...string) {
	t.Helper()
	r.Command(cmd, args...).ExpectFailure().Run(t)
}

const REPO_NAME_FOR_E2E_TESTS = "alr-repo"
const REPO_URL_FOR_E2E_TESTS = "https://git.alr-pkg.ru/Plemya-x/repo-for-tests.git"

func defaultPrepare(t *testing.T, r capytest.Runner) {
	execShouldNoError(t, r,
		"sudo",
		"alr",
		"repo",
		"add",
		REPO_NAME_FOR_E2E_TESTS,
		REPO_URL_FOR_E2E_TESTS,
	)

	execShouldNoError(t, r,
		"sudo",
		"alr",
		"ref",
	)
}

func runMatrixSuite(t *testing.T, name string, images []string, test func(t *testing.T, r capytest.Runner)) {
	t.Helper()
	for _, image := range images {
		ts := capytest.NewTestSuite(t, podman.Provider(
			podman.WithImage(e2eImage(image)),
			podman.WithVolumes("./alr:/tmp/alr-bin"),
			// podman прокидывает proxy-переменные хоста в контейнер;
			// прокси хоста (127.0.0.1) из контейнера недоступен — отключаем
			podman.WithEnvVars(
				"http_proxy=", "https_proxy=", "all_proxy=",
				"HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=",
			),
			podman.WithPrivileged(true),
		))
		ts.BeforeEach(func(t *testing.T, r capytest.Runner) {
			execShouldNoError(t, r, "/bin/alr-test-setup", "alr-install")
			execShouldNoError(t, r, "/bin/alr-test-setup", "passwordless-sudo-setup")
			// RPC-режим по умолчанию включён и ходит на боевой сайт —
			// отключаем его, чтобы e2e-тесты не зависели от сети.
			// Выставляем и для root (sudo-вызовы), и для тестового пользователя.
			execShouldNoError(t, r, "sudo", "alr", "config", "set", "site.disableRPC", "true")
			execShouldNoError(t, r, "alr", "config", "set", "site.disableRPC", "true")
		})
		ts.Run(fmt.Sprintf("%s/%s", name, image), test)
	}
}
