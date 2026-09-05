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
func (rs *Repos) findInCache(ctx context.Context, pkgNames []string) (map[string][]alrsh.Package, []string) {
	found := make(map[string][]alrsh.Package)
	var notFound []string
	cutoff := time.Now().Add(-rpcCacheTTL).Unix()

	for _, name := range pkgNames {
		if name == "" {
			continue
		}
		dep := depver.Parse(name)
		where, args := nameToQuery(dep.Name)
		where += " AND cached_at > ?"
		args = append(args, cutoff)

		result, err := rs.db.GetPkgs(ctx, where, args...)
		if err == nil && dep.HasVersionConstraint() && len(result) > 0 {
			result = filterByVersion(result, dep)
		}
		if err == nil && len(result) > 0 {
			found[name] = result
			slog.Debug("FindPkgs: пакет найден в локальном кэше", "name", name)
		} else {
			notFound = append(notFound, name)
		}
	}
	return found, notFound
}

// splitRepoQualifier разбирает имя пакета с квалификатором репозитория:
// "repo/pkg" → ("pkg", "repo"), "pkg+repo" → ("pkg", "repo"), "pkg" → ("pkg", "")
func splitRepoQualifier(name string) (string, string) {
	if i := strings.Index(name, "/"); i > 0 {
		return name[i+1:], name[:i]
	}
	if i := strings.Index(name, "+"); i > 0 {
		return name[:i], name[i+1:]
	}
	return name, ""
}

// nameToQuery строит WHERE-условие поиска по имени пакета
// с учётом форм repo/pkg и pkg+repo
func nameToQuery(searchName string) (string, []any) {
	pkgName, repo := splitRepoQualifier(searchName)
	if repo != "" {
		return "name = ? AND repository = ?", []any{pkgName, repo}
	}
	return "name = ?", []any{searchName}
}

// findViaRPC ищет пакеты через ALR Site RPC API
// Возвращает найденные пакеты, список не найденных, и флаг успеха RPC-запроса
func (rs *Repos) findViaRPC(ctx context.Context, pkgNames []string) (map[string][]alrsh.Package, []string, bool) {
	if rs.rpc == nil {
		return nil, pkgNames, false
	}

	found := make(map[string][]alrsh.Package)
	var notFound []string

	// Преобразуем repo/pkg → pkg+repo для RPC API,
	// версионные ограничения (pkg>=1.0) срезаем — фильтруем после ответа
	rpcNames := make([]string, 0, len(pkgNames))
	for _, name := range pkgNames {
		searchName := depver.Parse(name).Name
		pkgName, repo := splitRepoQualifier(searchName)
		rpcName := searchName
		if strings.Contains(searchName, "/") {
			rpcName = pkgName + "+" + repo
		}
		rpcNames = append(rpcNames, rpcName)
	}

	// Отправляем batch-запрос на RPC
	slog.Debug("FindPkgs: запрос через RPC", "url", rs.rpc, "count", len(rpcNames))
	rpcPkgs, err := rs.rpc.GetPackages(ctx, rpcNames)
	if err != nil {
		slog.Debug("FindPkgs: RPC запрос не удался, используем локальную БД", "error", err)
		return nil, pkgNames, false
	}
	slog.Debug("FindPkgs: RPC вернул пакеты", "count", len(rpcPkgs))

	// Создаём мапу имя → пакеты (ключ — оригинальное имя запроса)
	rpcResults := make(map[string][]rpc.Package)
	for _, p := range rpcPkgs {
		// Сопоставляем с оригинальным именем запроса по имени пакета или
		// basepkg_name; если в запросе есть квалификатор репозитория
		// (repo/pkg или pkg+repo), проверяем и его
		matched := false
		for _, origName := range pkgNames {
			origSearch := depver.Parse(origName).Name
			pkgName, repo := splitRepoQualifier(origSearch)
			if repo != "" && p.Repo != repo {
				continue
			}
			if p.Name == pkgName || p.BasePkgName == pkgName {
				rpcResults[origName] = append(rpcResults[origName], p)
				matched = true
				break
			}
		}
		if !matched {
			rpcResults[p.Name] = append(rpcResults[p.Name], p)
		}
	}

	for _, name := range pkgNames {
		if pkgs, ok := rpcResults[name]; ok && len(pkgs) > 0 {
			dep := depver.Parse(name)
			var result []alrsh.Package
			for _, p := range pkgs {
				pkg := *p.ToALRShPackage()
				pkg.CachedAt = time.Now().Unix()
				// Сохраняем пакет в локальную БД для кэширования и tab completion
				if err := rs.db.InsertPackage(ctx, pkg); err != nil {
					slog.Debug("FindPkgs: не удалось сохранить пакет в кэш", "name", pkg.Name, "error", err)
				}
				// В кэш пишем всё, а в результат отдаём только подходящие по версии
				if dep.HasVersionConstraint() && !dep.Satisfies(pkg.Version) {
					continue
				}
				result = append(result, pkg)
			}
			if len(result) > 0 {
				found[name] = result
				slog.Debug("FindPkgs: пакет найден через RPC", "name", name)
			} else {
				notFound = append(notFound, name)
			}
		} else {
			notFound = append(notFound, name)
		}
	}

	return found, notFound, true
}

func (rs *Repos) FindPkgs(ctx context.Context, pkgs []string) (map[string][]alrsh.Package, []string, error) {
	slog.Debug("FindPkgs: started", "packages", pkgs)
	found := make(map[string][]alrsh.Package)
	var notFound []string

	// Загружаем список site-репозиториев (лениво, с кэшированием)
	rs.loadSiteRepos(ctx)

	// Сначала проверяем локальный кэш (пакеты, ранее полученные через RPC)
	var cacheNotFound []string
	if rs.rpc != nil {
		cacheFound, remaining := rs.findInCache(ctx, pkgs)
		for k, v := range cacheFound {
			found[k] = v
		}
		cacheNotFound = remaining
	} else {
		cacheNotFound = pkgs
	}

	// Затем пробуем найти через RPC (для репозиториев, которые знает alr-site)
	var rpcNotFound []string
	var rpcSuccess bool
	if len(cacheNotFound) > 0 && rs.rpc != nil {
		rpcFound, remaining, success := rs.findViaRPC(ctx, cacheNotFound)
		rpcSuccess = success
		for k, v := range rpcFound {
			found[k] = v
		}
		rpcNotFound = remaining
	} else {
		rpcNotFound = cacheNotFound
	}

	// Определяем, можем ли мы фильтровать site-репозитории
	// Фильтруем только если RPC запрос был успешен И список site-репозиториев загружен
	siteReposAvailable := rs.SiteReposErr() == nil

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

		// Если RPC работал и мы знаем список site-репозиториев,
		// исключаем пакеты из site-репозиториев (для них git — только fallback при недоступности RPC)
		if rpcSuccess && siteReposAvailable && len(result) > 0 {
			var filtered []alrsh.Package
			for _, pkg := range result {
				if !rs.isSiteRepo(pkg.Repository) {
					filtered = append(filtered, pkg)
				} else {
					slog.Debug("FindPkgs: исключаем пакет из site-репозитория (RPC активен)", "name", pkg.Name, "repo", pkg.Repository)
				}
			}
			result = filtered
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
