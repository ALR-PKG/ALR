// ALR - Any Linux Repository
// Copyright (C) 2026 The ALR Authors
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

package cliutils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

func TestPkgPromptOptionIncludesRelease(t *testing.T) {
	option := pkgPromptOption(alrsh.Package{
		Repository: "alr-repo",
		Name:       "artcraft-launcher",
		Version:    "0.6.1",
		Release:    2,
	})

	assert.Equal(t, "alr-repo/artcraft-launcher 0.6.1-2", option)
}
