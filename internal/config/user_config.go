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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	ktoml "github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/constants"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

type UserConfig struct {
	k   *koanf.Koanf
	cfg *types.Config
}

func NewUserConfig() *UserConfig {
	return &UserConfig{
		k:   koanf.New("."),
		cfg: &types.Config{},
	}
}

func (c *UserConfig) koanf() *koanf.Koanf {
	return c.k
}

func expandUserConfigPath() string {
	path := constants.UserConfigPath
	if strings.HasPrefix(path, "~/") {
		usr, err := user.Current()
		if err == nil {
			path = filepath.Join(usr.HomeDir, path[2:])
		}
	}
	return path
}

func (c *UserConfig) Load() error {
	path := expandUserConfigPath()
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err := c.k.Load(file.Provider(path), ktoml.Parser()); err != nil {
		return err
	}

	return c.k.Unmarshal("", c.cfg)
}

func (c *UserConfig) Save() error {
	path := expandUserConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	bytes, err := c.k.Marshal(ktoml.Parser())
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	if _, err := file.Write(bytes); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	if err := file.Sync(); err != nil {
		return fmt.Errorf("failed to sync config: %w", err)
	}

	return nil
}

func (c *UserConfig) SetRootCmd(v string) {
	err := c.k.Set("rootCmd", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetUseRootCmd(v bool) {
	err := c.k.Set("useRootCmd", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetPagerStyle(v string) {
	err := c.k.Set("pagerStyle", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetIgnorePkgUpdates(v []string) {
	err := c.k.Set("ignorePkgUpdates", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetAutoPull(v bool) {
	err := c.k.Set("autoPull", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetLogLevel(v string) {
	err := c.k.Set("logLevel", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetRepos(v []types.Repo) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var m []interface{}
	err = json.Unmarshal(b, &m)
	if err != nil {
		panic(err)
	}
	err = c.k.Set("repo", m)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetUpdateSystemOnUpgrade(v bool) {
	err := c.k.Set("updateSystemOnUpgrade", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetPreferALRDeps(v bool) {
	err := c.k.Set("preferALRDeps", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetSiteURL(v string) {
	err := c.k.Set("site.url", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetSiteTimeout(v int) {
	err := c.k.Set("site.timeout", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetSiteDisableRPC(v bool) {
	err := c.k.Set("site.disableRPC", v)
	if err != nil {
		panic(err)
	}
}

func (c *UserConfig) SetSitePreferRPC(v bool) {
	err := c.k.Set("site.preferRPC", v)
	if err != nil {
		panic(err)
	}
}
