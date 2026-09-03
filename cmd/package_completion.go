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

package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/db"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/repos"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/rpc"
)

// completePackages возвращает список пакетов в формате name+repo для автодополнения.
// Поддерживает:
//   - прямой ввод имени (prefix без разделителей)
//   - постфикс репозитория через "+"  (name+repo)
//   - устаревший формат repo/pkg (для обратной совместимости)
//
// Для пакетов с сайта используется smart caching:
//   - сравнивает количество пакетов на сайте и в локальной БД
//   - если совпадает и кэш не старше 24 часов — использует локальную БД
//   - иначе — загружает весь список с сайта и обновляет кэш
func completePackages(
	ctx context.Context,
	database *db.Database,
	rp *repos.Repos,
	prefix string,
) ([]string, error) {
	seen := make(map[string]struct{})
	var out []string

	// Поддержка устаревшего формата repo/pkg
	searchPrefix := prefix
	repoFilter := ""
	if strings.Contains(prefix, "/") {
		parts := strings.SplitN(prefix, "/", 2)
		repoFilter = parts[0]
		searchPrefix = parts[1]
	}

	var namePrefix string
	var repoSuffix string
	if strings.Contains(searchPrefix, "+") {
		parts := strings.SplitN(searchPrefix, "+", 2)
		namePrefix = parts[0]
		repoSuffix = parts[1]
	} else {
		namePrefix = searchPrefix
	}

	// Поиск в локальной БД (всегда, включая локальные репы и закэшированные пакеты)
	var where string
	var args []any
	switch {
	case repoFilter != "" && namePrefix != "":
		where = "name LIKE ? ESCAPE ? AND repository = ?"
		args = []any{cliutils.EscapeLikePattern(namePrefix) + "%", "\\", repoFilter}
	case repoFilter != "":
		where = "repository = ?"
		args = []any{repoFilter}
	case repoSuffix != "" && namePrefix != "":
		where = "name LIKE ? ESCAPE ? AND repository LIKE ? ESCAPE ?"
		args = []any{cliutils.EscapeLikePattern(namePrefix) + "%", "\\", cliutils.EscapeLikePattern(repoSuffix) + "%", "\\"}
	case repoSuffix != "":
		where = "repository LIKE ? ESCAPE ?"
		args = []any{cliutils.EscapeLikePattern(repoSuffix) + "%", "\\"}
	case namePrefix != "":
		where = "name LIKE ? ESCAPE ?"
		args = []any{cliutils.EscapeLikePattern(namePrefix) + "%", "\\"}
	default:
		where = "true"
	}

	result, err := database.GetPkgs(ctx, where, args...)
	if err != nil {
		return nil, fmt.Errorf("completion: ошибка получения пакетов: %w", err)
	}

	// Пакеты из git-репозиториев (cached_at = 0) не переопределяем сайтом;
	// сайт-кэш (cached_at > 0) обновляем, чтобы cached_at не протух навсегда
	gitKeys := make(map[string]struct{}, len(result))
	for _, pkg := range result {
		key := pkg.Name + "+" + pkg.Repository
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if pkg.CachedAt == 0 {
			gitKeys[key] = struct{}{}
		}
		out = append(out, key)
	}

	// Smart caching для RPC пакетов
	if rp != nil && rp.HasRPC() {
		useCache := false
		siteCount, err := rp.RPCClient().CountPackages(ctx, "name", namePrefix)
		if err != nil {
			slog.Debug("completion: ошибка получения count с сайта", "error", err)
		} else {
			// Считаем только пакеты, закэшированные с сайта (cached_at > 0);
			// у пакетов из git-репозиториев cached_at = 0
			var localCount int64
			if namePrefix != "" {
				localCount, err = database.CountPkgs(ctx,
					"cached_at > 0 AND name LIKE ? ESCAPE ?",
					cliutils.EscapeLikePattern(namePrefix)+"%", "\\")
			} else {
				localCount, err = database.CountPkgs(ctx, "cached_at > 0")
			}
			if err != nil {
				slog.Debug("completion: ошибка получения count из БД", "error", err)
			} else {
				if int64(siteCount) == localCount {
					// Проверяем свежесть кэша (не старше 24 часов)
					minCachedAt, err := database.GetMinCachedAt(ctx, namePrefix)
					if err == nil && minCachedAt > time.Now().Add(-24*time.Hour).Unix() {
						useCache = true
					}
				}
			}
		}

		if !useCache {
			slog.Debug("completion: обновление кэша пакетов с сайта", "prefix", prefix)
			var allPkgs []rpc.Package
			if namePrefix != "" {
				allPkgs, err = rp.RPCClient().SearchAllPackages(ctx, "name", namePrefix)
			} else {
				allPkgs, err = rp.RPCClient().ListAllPackages(ctx, "")
			}
			if err != nil {
				slog.Debug("completion: ошибка загрузки пакетов с сайта", "error", err)
			} else {
				for _, p := range allPkgs {
					pkg := p.ToALRShPackage()
					pkg.CachedAt = time.Now().Unix()

					if repoFilter != "" && pkg.Repository != repoFilter {
						continue
					}
					if repoSuffix != "" && !strings.HasPrefix(pkg.Repository, repoSuffix) {
						continue
					}

					key := pkg.Name + "+" + pkg.Repository
					if _, ok := gitKeys[key]; ok {
						continue
					}
					if _, ok := seen[key]; !ok {
						seen[key] = struct{}{}
						out = append(out, key)
					}

					if dbErr := database.InsertPackage(ctx, *pkg); dbErr != nil {
						slog.Debug("completion: не удалось сохранить пакет в кэш", "name", pkg.Name, "error", dbErr)
					}
				}
			}
		}
	}

	return out, nil
}
