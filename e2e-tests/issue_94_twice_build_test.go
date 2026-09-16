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
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.alt-gnome.ru/capytest"
)

func TestE2EIssue94TwiceBuild(t *testing.T) {
	runMatrixSuite(
		t,
		"issue-94-twice-build",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			defaultPrepare(t, r)

			var stderr bytes.Buffer

			// ALR_LOG_LEVEL=INFO нужен, т.к. в non-TTY режиме уровень логов понижается до WARN.
			// Сообщение о сборке локализовано (по факту всегда ru): slog.Info("Building dependency %s-%s").
			r.Command("sudo", "env", "ALR_LOG_LEVEL=INFO", "alr", "in", "test-94-app").
				WithCaptureStderr(&stderr).
				ExpectSuccess().
				Run(t)

			assert.Equal(t, 1, strings.Count(stderr.String(), "Сборка зависимости test-94-dep"))
		},
	)
}
