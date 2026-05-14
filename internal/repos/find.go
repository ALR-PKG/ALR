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

package repos

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/rpc"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/depver"
)

const rpcCacheTTL = 1 * time.Hour

// findInCache ищет пакеты в локальной SQLite БД (кэш от предыдущих RPC-запросов).
// Возвращает найденные пакеты и список имён, которых нет в кэше или кэш устарел.
func (rs *Repos) findInCache(pkgNames []string) (map[string][]alrsh.Package, []string) {
	found := make(map[string][]alrsh.Package)
	var notFound []string
	cutoff := time.Now().Add(-rpcCacheTTL).Unix()

	for _, name := range pkgNames {
		if name == "" {
			continue
		}
		result, err := rs.db.GetPkgs(context.Background(), "name = ? AND cached_at > ?", name, cutoff)
		if err == nil && len(result) > 0 {
			found[name] = result
			slog.Debug("FindPkgs: пакет найден в локальном кэше", "name", name)
		} else {
			notFound = append(notFound, name)
		}
	}
	return found, notFound
}

// findViaRPC ищет пакеты через ALR Site RPC API
func (rs *Repos) findViaRPC(ctx context.Context, pkgNames []string) (map[string][]alrsh.Package, []string) {
	if rs.rpc == nil {
		return nil, pkgNames
	}

	found := make(map[string][]alrsh.Package)
	var notFound []string

	// Отправляем batch-запрос на RPC
	slog.Debug("FindPkgs: запрос через RPC", "url", rs.rpc, "count", len(pkgNames))
	rpcPkgs, err := rs.rpc.GetPackages(ctx, pkgNames)
	if err != nil {
		slog.Debug("FindPkgs: RPC запрос не удался, используем локальную БД", "error", err)
		return nil, pkgNames
	}
	slog.Debug("FindPkgs: RPC вернул пакеты", "count", len(rpcPkgs))

	// Создаём мапу имя → пакеты (ключ — полное имя с репозиторием)
	rpcResults := make(map[string][]rpc.Package)
	for _, p := range rpcPkgs {
		key := p.Name
		if p.Repo != "" {
			key = p.Name + "+" + p.Repo
		}
		rpcResults[key] = append(rpcResults[key], p)
	}

	for _, name := range pkgNames {
		if pkgs, ok := rpcResults[name]; ok && len(pkgs) > 0 {
			var result []alrsh.Package
			for _, p := range pkgs {
				pkg := *p.ToALRShPackage()
				pkg.CachedAt = time.Now().Unix()
				result = append(result, pkg)
				// Сохраняем пакет в локальную БД для кэширования и tab completion
				if err := rs.db.InsertPackage(ctx, pkg); err != nil {
					slog.Debug("FindPkgs: не удалось сохранить пакет в кэш", "name", pkg.Name, "error", err)
				}
			}
			found[name] = result
			slog.Debug("FindPkgs: пакет найден через RPC", "name", name)
		} else {
			notFound = append(notFound, name)
		}
	}

	return found, notFound
}

func (rs *Repos) FindPkgs(ctx context.Context, pkgs []string) (map[string][]alrsh.Package, []string, error) {
	slog.Debug("FindPkgs: started", "packages", pkgs)
	found := make(map[string][]alrsh.Package)
	var notFound []string

	// Сначала проверяем локальный кэш (пакеты, ранее полученные через RPC)
	var cacheNotFound []string
	if rs.rpc != nil {
		cacheFound, remaining := rs.findInCache(pkgs)
		for k, v := range cacheFound {
			found[k] = v
		}
		cacheNotFound = remaining
	} else {
		cacheNotFound = pkgs
	}

	// Затем пробуем найти через RPC (для репозиториев, которые знает alr-site)
	var rpcNotFound []string
	if len(cacheNotFound) > 0 && rs.rpc != nil {
		rpcFound, remaining := rs.findViaRPC(ctx, cacheNotFound)
		for k, v := range rpcFound {
			found[k] = v
		}
		rpcNotFound = remaining
	} else {
		rpcNotFound = cacheNotFound
	}

	// Для оставшихся ищем в локальной БД (git-репозитории или fallback)
	for _, pkgName := range rpcNotFound {
		if pkgName == "" {
			continue
		}

		// Parse version constraint from package name
		dep := depver.Parse(pkgName)
		searchName := dep.Name

		var result []alrsh.Package
		var err error

		switch {
		case strings.Contains(searchName, "/"):
			// repo/pkg
			parts := strings.SplitN(searchName, "/", 2)
			repo := parts[0]
			name := parts[1]
			result, err = rs.db.GetPkgs(ctx, "name = ? AND repository = ?", name, repo)

		case strings.Contains(searchName, "+"):
			// pkg+repo
			parts := strings.SplitN(searchName, "+", 2)
			name := parts[0]
			repo := parts[1]
			result, err = rs.db.GetPkgs(ctx, "name = ? AND repository = ?", name, repo)

		default:
			// Сначала ищем по точному имени пакета
			slog.Debug("FindPkgs: searching by name", "name", searchName)
			result, err = rs.db.GetPkgs(ctx, "name = ?", searchName)
			if err != nil {
				return nil, nil, fmt.Errorf("FindPkgs: get by name: %w", err)
			}
			slog.Debug("FindPkgs: search by name result", "name", searchName, "count", len(result))

			// Затем по provides
			if len(result) == 0 {
				slog.Debug("FindPkgs: searching by provides", "name", searchName)
				result, err = rs.db.GetPkgs(ctx, "json_array_contains(provides, ?)", searchName)
				if err != nil {
					return nil, nil, fmt.Errorf("FindPkgs: get by provides: %w", err)
				}
				slog.Debug("FindPkgs: search by provides result", "name", searchName, "count", len(result))
			}

			// В последнюю очередь по basepkg_name (для мультипакетов)
			if len(result) == 0 {
				slog.Debug("FindPkgs: searching by basepkg_name", "name", searchName)
				result, err = rs.db.GetPkgs(ctx, "basepkg_name = ?", searchName)
				if err != nil {
					return nil, nil, fmt.Errorf("FindPkgs: get by basepkg_name: %w", err)
				}
			}
		}

		if err != nil {
			return nil, nil, fmt.Errorf("FindPkgs: lookup for %q failed: %w", pkgName, err)
		}

		// Filter by version if constraint is specified
		if dep.HasVersionConstraint() && len(result) > 0 {
			result = filterByVersion(result, dep)
		}

		if len(result) == 0 {
			notFound = append(notFound, pkgName)
		} else {
			found[pkgName] = result
		}
	}

	return found, notFound, nil
}

// filterByVersion filters packages by version constraint.
func filterByVersion(pkgs []alrsh.Package, dep depver.Dependency) []alrsh.Package {
	var filtered []alrsh.Package
	for _, pkg := range pkgs {
		if dep.Satisfies(pkg.Version) {
			filtered = append(filtered, pkg)
		}
	}
	return filtered
}
