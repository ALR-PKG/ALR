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

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvConfigKeys(t *testing.T) {
	t.Setenv("ALR_SITE_URL", "https://mirror.example.com")
	t.Setenv("ALR_SITE_TIMEOUT", "30")
	t.Setenv("ALR_SITE_DISABLE_RPC", "true")
	t.Setenv("ALR_SITE_PREFER_RPC", "false")
	t.Setenv("ALR_LOG_LEVEL", "debug")
	t.Setenv("ALR_PREFER_ALR_DEPS", "true")
	t.Setenv("ALR_UNKNOWN_VAR", "ignored")

	c := NewEnvConfig()
	require.NoError(t, c.Load())

	k := c.koanf()

	// Вложенные ключи site.* должны появляться как site.url, а не плоский siteUrl
	assert.Equal(t, "https://mirror.example.com", k.String("site.url"))
	assert.Equal(t, "30", k.String("site.timeout"))
	assert.Equal(t, "true", k.String("site.disableRPC"))
	assert.Equal(t, "false", k.String("site.preferRPC"))
	assert.Equal(t, "debug", k.String("logLevel"))
	assert.Equal(t, "true", k.String("preferALRDeps"))

	assert.False(t, k.Exists("siteUrl"), "плоский ключ siteUrl не должен создаваться")
	assert.False(t, k.Exists("unknown_var"), "неразрешённые переменные должны игнорироваться")
}
