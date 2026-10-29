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

	"github.com/knadh/koanf/providers/confmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateMissingKeysSetsPreferRPC(t *testing.T) {
	cfg := New()
	require.NoError(t, cfg.System.k.Load(confmap.Provider(map[string]interface{}{
		"site": map[string]interface{}{"url": "https://alr-pkg.ru"},
	}, "."), nil))

	needSave := cfg.migrateMissingKeys()

	assert.True(t, needSave)
	assert.True(t, cfg.System.k.Bool("site.preferRPC"))
}

func TestMigrateMissingKeysKeepsExplicitPreferRPCFalse(t *testing.T) {
	cfg := New()
	require.NoError(t, cfg.System.k.Load(confmap.Provider(map[string]interface{}{
		"site": map[string]interface{}{"preferRPC": false},
	}, "."), nil))

	cfg.migrateMissingKeys()

	assert.False(t, cfg.System.k.Bool("site.preferRPC"))
}
