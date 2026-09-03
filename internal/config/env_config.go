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

package config

import (
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/v2"
)

type EnvConfig struct {
	k *koanf.Koanf
}

func NewEnvConfig() *EnvConfig {
	return &EnvConfig{
		k: koanf.New("."),
	}
}

func (c *EnvConfig) koanf() *koanf.Koanf {
	return c.k
}

func (c *EnvConfig) Load() error {
	// Явный маппинг env-переменных на ключи конфига:
	// строковые трансформации не дают вложенных ключей вида site.url
	allowedKeys := map[string]string{
		"ALR_LOG_LEVEL":        "logLevel",
		"ALR_PAGER_STYLE":      "pagerStyle",
		"ALR_AUTO_PULL":        "autoPull",
		"ALR_PREFER_ALR_DEPS":  "preferALRDeps",
		"ALR_SITE_URL":         "site.url",
		"ALR_SITE_TIMEOUT":     "site.timeout",
		"ALR_SITE_DISABLE_RPC": "site.disableRPC",
		"ALR_SITE_PREFER_RPC":  "site.preferRPC",
	}
	err := c.k.Load(env.Provider("ALR_", ".", func(s string) string {
		return allowedKeys[s]
	}), nil)

	return err
}
