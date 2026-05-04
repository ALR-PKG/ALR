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

package repos

import (
	"context"
	"log/slog"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/config"
	database "git.alr-pkg.ru/Plemya-x/ALR/internal/db"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/rpc"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

type Config interface {
	GetPaths() *config.Paths
	Repos() []types.Repo
	Site() types.Site
}

type Repos struct {
	cfg Config
	db  *database.Database
	rpc *rpc.Client
}

func New(
	cfg Config,
	db *database.Database,
) *Repos {
	rs := &Repos{
		cfg: cfg,
		db:  db,
	}

	// Инициализируем RPC клиент если указан URL сайта
	site := cfg.Site()
	if site.URL != "" {
		rs.rpc = rpc.NewClient(site)
	}

	return rs
}

// HasRPC возвращает true если RPC клиент настроен
func (r *Repos) HasRPC() bool {
	return r.rpc != nil
}

// RPCClient возвращает RPC клиент (nil если не настроен)
func (r *Repos) RPCClient() *rpc.Client {
	return r.rpc
}

// CheckUpdates проверяет обновления через RPC (batch) или локально
func (r *Repos) CheckUpdates(
	ctx context.Context,
	installed map[string]string,
	info *distro.OSRelease,
) ([]rpc.UpdateInfo, error) {
	if r.rpc == nil {
		return nil, nil
	}

	// Формируем список установленных пакетов для RPC
	pkgs := make([]rpc.InstalledPackage, 0, len(installed))
	for fullName, version := range installed {
		name, repo := splitPkgName(fullName)
		pkgs = append(pkgs, rpc.InstalledPackage{
			Name:    name,
			Repo:    repo,
			Version: version,
		})
	}

	slog.Debug("checkForUpdates: отправка batch запроса через RPC", "count", len(pkgs))

	updates, err := r.rpc.CheckUpdates(ctx, pkgs, info)
	if err != nil {
		slog.Warn("RPC проверка обновлений не удалась, fallback на локальную БД", "error", err)
		return nil, nil
	}

	slog.Debug("checkForUpdates: RPC вернул обновления", "count", len(updates))
	return updates, nil
}

// splitPkgName разделяет полное имя пакета на имя и репозиторий
func splitPkgName(fullName string) (name, repo string) {
	// Ищем последний '+' как разделитель репозитория
	for i := len(fullName) - 1; i >= 0; i-- {
		if fullName[i] == '+' {
			return fullName[:i], fullName[i+1:]
		}
	}
	return fullName, ""
}
