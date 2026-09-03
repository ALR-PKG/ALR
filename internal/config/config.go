// This file was originally part of the project "LURE - Linux User REpository", created by Elara Musayelyan.
// It has been modified as part of "ALR - Any Linux Repository" by the ALR Authors.
//
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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/goccy/go-yaml"
	ktoml "github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/v2"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/constants"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

type ALRConfig struct {
	cfg   *types.Config
	paths *Paths

	System *SystemConfig
	User   *UserConfig
	env    *EnvConfig
}

func New() *ALRConfig {
	return &ALRConfig{
		System: NewSystemConfig(),
		User:   NewUserConfig(),
		env:    NewEnvConfig(),
	}
}

func defaultConfigKoanf() *koanf.Koanf {
	k := koanf.New(".")
	defaults := map[string]interface{}{
		"rootCmd":               "sudo",
		"useRootCmd":            true,
		"pagerStyle":            "native",
		"ignorePkgUpdates":      []string{},
		"logLevel":              "info",
		"autoPull":              true,
		"updateSystemOnUpgrade": false,
		"preferALRDeps":         false,
		"site": types.Site{
			URL:        constants.DefaultSiteURL,
			Mirrors:    []string{},
			Timeout:    10,
			DisableRPC: false,
			PreferRPC:  true,
		},
		"repo": []types.Repo{},
	}
	if err := k.Load(confmap.Provider(defaults, "."), nil); err != nil {
		panic(k)
	}
	return k
}

func (c *ALRConfig) Load() error {
	config := types.Config{}

	merged := koanf.New(".")

	if err := c.System.Load(); err != nil {
		return fmt.Errorf("failed to load system config: %w", err)
	}

	if err := c.User.Load(); err != nil {
		return fmt.Errorf("failed to load user config: %w", err)
	}

	if err := c.env.Load(); err != nil {
		return fmt.Errorf("failed to load env config: %w", err)
	}

	systemK := c.System.koanf()
	userK := c.User.koanf()
	envK := c.env.koanf()

	if err := merged.Merge(defaultConfigKoanf()); err != nil {
		return fmt.Errorf("failed to merge default config: %w", err)
	}
	if err := merged.Merge(systemK); err != nil {
		return fmt.Errorf("failed to merge system config: %w", err)
	}
	if err := merged.Merge(userK); err != nil {
		return fmt.Errorf("failed to merge user config: %w", err)
	}
	if err := merged.Merge(envK); err != nil {
		return fmt.Errorf("failed to merge env config: %w", err)
	}
	if err := merged.Unmarshal("", &config); err != nil {
		return fmt.Errorf("failed to unmarshal merged config: %w", err)
	}

	c.cfg = &config

	c.paths = &Paths{}
	c.paths.UserConfigPath = constants.SystemConfigPath
	c.paths.CacheDir = constants.SystemCachePath
	c.paths.RepoDir = filepath.Join(c.paths.CacheDir, "repo")
	c.paths.PkgsDir = filepath.Join(constants.TempDir, "pkgs") // Перемещаем в /tmp/alr/pkgs
	c.paths.DBPath = filepath.Join(c.paths.CacheDir, "alr.db")

	// Проверяем существование кэш-директории, но не пытаемся создать
	if _, err := os.Stat(c.paths.CacheDir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to check cache directory: %w", err)
		}
	}

	// Выполняем миграцию конфигурации при необходимости
	if err := c.migrateConfig(); err != nil {
		return fmt.Errorf("failed to migrate config: %w", err)
	}

	return nil
}

func (c *ALRConfig) ToYAML() (string, error) {
	data, err := yaml.Marshal(c.cfg)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *ALRConfig) migrateConfig() error {
	// Проверяем, существует ли конфигурационный файл
	if _, err := os.Stat(constants.SystemConfigPath); os.IsNotExist(err) {
		// Если файла нет, создаем полный конфигурационный файл с дефолтными значениями
		if err := c.createDefaultConfig(); err != nil {
			// Если не удается создать конфиг, это не критично - продолжаем работу
			// но выводим предупреждение
			fmt.Fprintf(os.Stderr, "Предупреждение: не удалось создать конфигурационный файл %s: %v\n", constants.SystemConfigPath, err)
			return nil
		}
	} else {
		needSave := false
		// Если файл существует, проверяем, есть ли в нем новые опции
		if !c.System.k.Exists("updateSystemOnUpgrade") {
			c.System.SetUpdateSystemOnUpgrade(false)
			needSave = true
		}
		if !c.System.k.Exists("preferALRDeps") {
			c.System.SetPreferALRDeps(false)
			needSave = true
		}
		if needSave {
			if err := c.System.Save(); err != nil {
				return nil
			}
		}
	}

	return nil
}

func (c *ALRConfig) createDefaultConfig() error {
	// Проверяем, запущен ли процесс от root
	if os.Getuid() != 0 {
		// Если не root, пытаемся запустить создание конфига с повышением привилегий
		return c.createDefaultConfigWithPrivileges()
	}

	// Если уже root, создаем конфиг напрямую
	return c.doCreateDefaultConfig()
}

func (c *ALRConfig) createDefaultConfigWithPrivileges() error {
	// Если useRootCmd отключен, просто пытаемся создать без повышения привилегий
	if !c.cfg.UseRootCmd {
		return c.doCreateDefaultConfig()
	}

	// Определяем команду для повышения привилегий
	rootCmd := c.cfg.RootCmd
	if rootCmd == "" {
		rootCmd = "sudo" // fallback
	}

	// Создаем временный файл с дефолтной конфигурацией
	tmpFile, err := os.CreateTemp("", "alr-config-*.toml")
	if err != nil {
		return fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	// Генерируем дефолтную конфигурацию во временный файл
	defaults := defaultConfigKoanf()
	tempSystemConfig := &SystemConfig{k: defaults}

	bytes, err := tempSystemConfig.k.Marshal(ktoml.Parser())
	if err != nil {
		return fmt.Errorf("не удалось сериализовать конфигурацию: %w", err)
	}

	if _, err := tmpFile.Write(bytes); err != nil {
		return fmt.Errorf("не удалось записать во временный файл: %w", err)
	}
	tmpFile.Close()

	// Используем команду повышения привилегий для создания директории и копирования файла

	// Создаем директорию с правами
	configDir := filepath.Dir(constants.SystemConfigPath)
	mkdirCmd := exec.Command(rootCmd, "mkdir", "-p", configDir)
	if err := mkdirCmd.Run(); err != nil {
		return fmt.Errorf("не удалось создать директорию %s: %w", configDir, err)
	}

	// Копируем файл в нужное место
	cpCmd := exec.Command(rootCmd, "cp", tmpFile.Name(), constants.SystemConfigPath)
	if err := cpCmd.Run(); err != nil {
		return fmt.Errorf("не удалось скопировать конфигурацию в %s: %w", constants.SystemConfigPath, err)
	}

	// Устанавливаем правильные права доступа
	chmodCmd := exec.Command(rootCmd, "chmod", "644", constants.SystemConfigPath)
	if err := chmodCmd.Run(); err != nil {
		// Не критично, продолжаем
		fmt.Fprintf(os.Stderr, "Предупреждение: не удалось установить права доступа для %s: %v\n", constants.SystemConfigPath, err)
	}

	return nil
}

func (c *ALRConfig) doCreateDefaultConfig() error {
	// Проверяем, существует ли директория для конфига
	configDir := filepath.Dir(constants.SystemConfigPath)
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		// Пытаемся создать директорию
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return fmt.Errorf("не удалось создать директорию %s: %w", configDir, err)
		}
	}

	// Загружаем дефолтную конфигурацию
	defaults := defaultConfigKoanf()

	// Копируем все дефолтные значения в системную конфигурацию
	c.System.k = defaults

	// Сохраняем конфигурацию в файл
	if err := c.System.Save(); err != nil {
		return fmt.Errorf("не удалось сохранить конфигурацию в %s: %w", constants.SystemConfigPath, err)
	}

	return nil
}

func (c *ALRConfig) RootCmd() string             { return c.cfg.RootCmd }
func (c *ALRConfig) PagerStyle() string          { return c.cfg.PagerStyle }
func (c *ALRConfig) AutoPull() bool              { return c.cfg.AutoPull }
func (c *ALRConfig) Repos() []types.Repo         { return c.cfg.Repos }
func (c *ALRConfig) SetRepos(repos []types.Repo) { c.System.SetRepos(repos) }
func (c *ALRConfig) IgnorePkgUpdates() []string  { return c.cfg.IgnorePkgUpdates }
func (c *ALRConfig) LogLevel() string            { return c.cfg.LogLevel }
func (c *ALRConfig) UseRootCmd() bool            { return c.cfg.UseRootCmd }
func (c *ALRConfig) UpdateSystemOnUpgrade() bool { return c.cfg.UpdateSystemOnUpgrade }
func (c *ALRConfig) PreferALRDeps() bool         { return c.cfg.PreferALRDeps }
func (c *ALRConfig) Site() types.Site            { return c.cfg.Site }
func (c *ALRConfig) DisableRPC() bool            { return c.cfg.Site.DisableRPC }
func (c *ALRConfig) PreferRPC() bool             { return c.cfg.Site.PreferRPC }
func (c *ALRConfig) GetPaths() *Paths            { return c.paths }
