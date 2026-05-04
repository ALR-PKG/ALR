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

	"git.alr-pkg.ru/Plemya-x/ALR/internal/rpc"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/depver"
)

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

	// Создаём мапу имя → пакеты
	rpcResults := make(map[string][]rpc.Package)
	for _, p := range rpcPkgs {
		key := p.Name
		rpcResults[key] = append(rpcResults[key], p)
	}

	for _, name := range pkgNames {
		if pkgs, ok := rpcResults[name]; ok && len(pkgs) > 0 {
			var result []alrsh.Package
			for _, p := range pkgs {
				result = append(result, *p.ToALRShPackage())
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

	// Сначала пробуем найти через RPC (для репозиториев, которые знает alr-site)
	rpcFound, rpcNotFound := rs.findViaRPC(ctx, pkgs)
	for k, v := range rpcFound {
		found[k] = v
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
