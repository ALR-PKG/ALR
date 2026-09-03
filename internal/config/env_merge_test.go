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

	"github.com/knadh/koanf/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

// Env-переменные должны переопределять дефолты после мержа koanf,
// включая вложенные ключи site.*
func TestEnvOverridesDefaults(t *testing.T) {
	t.Setenv("ALR_SITE_URL", "https://mirror.example.com")
	t.Setenv("ALR_SITE_TIMEOUT", "30")
	t.Setenv("ALR_SITE_DISABLE_RPC", "true")

	envCfg := NewEnvConfig()
	require.NoError(t, envCfg.Load())

	merged := koanf.New(".")
	require.NoError(t, merged.Merge(defaultConfigKoanf()))
	require.NoError(t, merged.Merge(envCfg.koanf()))

	var cfg types.Config
	require.NoError(t, merged.Unmarshal("", &cfg))

	assert.Equal(t, "https://mirror.example.com", cfg.Site.URL)
	assert.Equal(t, 30, cfg.Site.Timeout)
	assert.True(t, cfg.Site.DisableRPC)
}
