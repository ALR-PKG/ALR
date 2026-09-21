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

package dbus

import (
	"git.alr-pkg.ru/ALR/ALR/internal/build"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

// jobConfig оборачивает общий конфиг демона, подменяя только Paths
// (копией — общий конфиг не мутируется, другие job не затрагиваются).
type jobConfig struct {
	build.Config
	paths *config.Paths
}

func (c *jobConfig) GetPaths() *config.Paths { return c.paths }

// callerJobConfig возвращает конфиг задачи для сборки от uid вызвавшего:
// BuildDir/PkgsDir указывают на XDG-кэш вызвавшего, остальные пути и
// настройки делегируются общему конфигу демона.
func callerJobConfig(cfg build.Config, uid uint32) build.Config {
	paths := *cfg.GetPaths()
	paths.BuildDir, paths.PkgsDir = callerCachePaths(uid)
	return &jobConfig{Config: cfg, paths: &paths}
}
