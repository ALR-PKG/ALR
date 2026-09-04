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
	"fmt"
	"log/slog"
	"sync"

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
	cfg             Config
	db              *database.Database
	rpc             *rpc.Client
	siteRepos       []string
	siteReposLoaded bool
	siteReposErr    error
	siteReposMu     sync.RWMutex
}

func New(
	cfg Config,
	db *database.Database,
) *Repos {
	rs := &Repos{
		cfg: cfg,
		db:  db,
	}

	// Инициализируем RPC клиент если указан URL сайта и RPC не отключен
	site := cfg.Site()
	if site.URL != "" && !site.DisableRPC {
		rs.rpc = rpc.NewClient(site)
	}

	return rs
}

// HasRPC возвращает true если RPC клиент настроен
func (r *Repos) HasRPC() bool {
	return r.rpc != nil
}

// PreferRPC возвращает true если RPC настроен и предпочтителен для
// поиска, листинга и проверки обновлений. При site.preferRPC=false
// эти операции идут через локальную БД, а RPC используется только
// для получения пакетов, отсутствующих локально
func (r *Repos) PreferRPC() bool {
	return r.rpc != nil && r.cfg.Site().PreferRPC
}

// RPCClient возвращает RPC клиент (nil если не настроен)
func (r *Repos) RPCClient() *rpc.Client {
	return r.rpc
}

// loadSiteRepos загружает список репозиториев с alr-site через RPC
func (r *Repos) loadSiteRepos(ctx context.Context) {
	r.siteReposMu.Lock()
	defer r.siteReposMu.Unlock()

	if r.siteReposLoaded {
		return
	}

	if r.rpc == nil {
		r.siteReposLoaded = true
		return
	}

	repos, err := r.rpc.GetRepos(ctx)
	if err != nil {
		slog.Warn("Не удалось получить список репозиториев с сайта", "error", err)
		r.siteReposErr = err
		r.siteReposLoaded = true
		return
	}

	for _, repo := range repos {
		r.siteRepos = append(r.siteRepos, repo.Name)
	}
	r.siteReposLoaded = true
	slog.Debug("Получен список site-репозиториев", "count", len(r.siteRepos), "repos", r.siteRepos)
}

// isSiteRepo проверяет, является ли репозиторий site-репозиторием
func (r *Repos) isSiteRepo(name string) bool {
	r.siteReposMu.RLock()
	defer r.siteReposMu.RUnlock()

	for _, repo := range r.siteRepos {
		if repo == name {
			return true
		}
	}
	return false
}

// SiteReposLoaded возвращает true если список site-репозиториев был загружен
func (r *Repos) SiteReposLoaded() bool {
	r.siteReposMu.RLock()
	defer r.siteReposMu.RUnlock()
	return r.siteReposLoaded
}

// SiteReposErr возвращает ошибку загрузки site-репозиториев (если была)
func (r *Repos) SiteReposErr() error {
	r.siteReposMu.RLock()
	defer r.siteReposMu.RUnlock()
	return r.siteReposErr
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
		return nil, fmt.Errorf("RPC проверка обновлений не удалась: %w", err)
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
