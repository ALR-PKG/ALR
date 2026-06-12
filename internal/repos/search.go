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

package repos

import (
	"context"
	"log/slog"
	"time"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/rpc"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
)

const rpcListCacheTTL = 24 * time.Hour

// ListAllPackages возвращает все доступные пакеты.
// Если настроен RPC, используется smart caching: при свежем и полном кэше
// возвращаются локальные данные, иначе список дополняется пакетами с сайта.
func (r *Repos) ListAllPackages(ctx context.Context) ([]alrsh.Package, error) {
	localPkgs, err := r.db.GetPkgs(ctx, "true")
	if err != nil {
		return nil, err
	}

	if r.rpc == nil {
		return localPkgs, nil
	}

	siteCount, err := r.rpc.CountPackages(ctx, "name", "")
	if err == nil {
		localCount, err := r.db.CountPkgs(ctx, "cached_at IS NOT NULL")
		if err == nil && int64(siteCount) == localCount {
			minCachedAt, err := r.db.GetMinCachedAt(ctx, "")
			if err == nil && minCachedAt > time.Now().Add(-rpcListCacheTTL).Unix() {
				return localPkgs, nil
			}
		}
	} else {
		slog.Debug("ListAllPackages: ошибка получения count с сайта", "error", err)
	}

	rpcPkgs, err := r.rpc.ListAllPackages(ctx, "")
	if err != nil {
		slog.Warn("ListAllPackages: не удалось загрузить пакеты с сайта, используем локальную БД", "error", err)
		return localPkgs, nil
	}

	return r.mergeAndCachePackages(ctx, localPkgs, rpcPkgs), nil
}

// SearchPackages ищет пакеты по имени/описанию локально и через RPC.
// Аргумент name используется для поиска по подстроке в имени пакета.
func (r *Repos) SearchPackages(ctx context.Context, name string) ([]alrsh.Package, error) {
	where := "name LIKE ? OR json_array_contains(provides, ?)"
	args := []any{"%" + name + "%", name}

	localPkgs, err := r.db.GetPkgs(ctx, where, args...)
	if err != nil {
		return nil, err
	}

	if r.rpc == nil {
		return localPkgs, nil
	}

	rpcPkgs, err := r.rpc.SearchAllPackages(ctx, "name", name)
	if err != nil {
		slog.Debug("SearchPackages: RPC поиск не удался, используем только локальную БД", "error", err)
		return localPkgs, nil
	}

	return r.mergeAndCachePackages(ctx, localPkgs, rpcPkgs), nil
}

func (r *Repos) mergeAndCachePackages(ctx context.Context, localPkgs []alrsh.Package, rpcPkgs []rpc.Package) []alrsh.Package {
	seen := make(map[string]struct{}, len(localPkgs))
	for _, pkg := range localPkgs {
		seen[pkg.Name+"+"+pkg.Repository] = struct{}{}
	}

	now := time.Now().Unix()
	for _, p := range rpcPkgs {
		pkg := *p.ToALRShPackage()
		pkg.CachedAt = now

		key := pkg.Name + "+" + pkg.Repository
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		localPkgs = append(localPkgs, pkg)

		if dbErr := r.db.InsertPackage(ctx, pkg); dbErr != nil {
			slog.Debug("mergeAndCachePackages: не удалось сохранить пакет в кэш", "name", pkg.Name, "error", dbErr)
		}
	}

	return localPkgs
}
