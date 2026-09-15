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

func TestE2EAlrAddRepo(t *testing.T) {
	runMatrixSuite(
		t,
		"add-repo-remove-repo",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			execShouldNoError(t, r, "sudo", "alr", "addrepo", "--name", "alr-repo", "--url", "https://git.alr-pkg.ru/ALR/alr-repo.git")
			execShouldNoError(t, r, "bash", "-c", "cat /etc/alr/alr.toml")
			execShouldNoError(t, r, "sudo", "alr", "removerepo", "--name", "alr-repo")

			r.Command("bash", "-c", "cat /etc/alr/alr.toml").
				ExpectStdoutContains("repo = []").
				Run(t)
		},
	)
}
