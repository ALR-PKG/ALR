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

package build

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/goreleaser/nfpm/v2"
	"github.com/leonelquinteros/gotext"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

type Cache struct {
	cfg Config
}

// CheckForBuiltPackage ищет готовый пакет в кэше и засчитывает попадание,
// только если рядом лежит доверенная мета с актуальным семантическим
// ключом. Любая проблема с метой — промах (безопасная сторона: пересборка).
func (c *Cache) CheckForBuiltPackage(
	ctx context.Context,
	input *BuildInput,
	vars *alrsh.Package,
	funcsKey string,
) (string, bool, error) {
	filename, err := pkgFileName(input, vars)
	if err != nil {
		return "", false, err
	}

	path, found := findTrustedCachedPkg(c.cfg, filename)
	if !found {
		return "", false, nil
	}

	if !checkCacheMeta(path, SemanticCacheKey(vars, funcsKey)) {
		return "", false, nil
	}

	return path, true, nil
}

// checkCacheMeta сверяет мету кэша <pkgPath>.meta с ожидаемым ключом.
// Мета проверяется так же, как сам пакет: регулярный файл с доверенным
// владельцем. Отсутствие, битый JSON, неизвестная версия формата или
// несовпадение ключа — промах.
func checkCacheMeta(pkgPath, expectedKey string) bool {
	metaPath := pkgPath + ".meta"

	if !isTrustedRegularFile(metaPath) {
		return false
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		slog.Debug("Не удалось прочитать мету кэша, пересборка", "path", metaPath, "error", err)
		return false
	}

	var meta cacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		slog.Debug("Мета кэша повреждена, пересборка", "path", metaPath, "error", err)
		return false
	}

	if meta.Version != cacheMetaVersion {
		slog.Debug("Неизвестная версия меты кэша, пересборка", "path", metaPath, "version", meta.Version)
		return false
	}

	if meta.Key != expectedKey {
		slog.Debug("Семантический ключ кэша не совпал, пересборка", "path", pkgPath)
		return false
	}

	return true
}

// findTrustedCachedPkg ищет готовый пакет в плоском кэше PkgsDir и
// возвращает его, только если это регулярный файл с доверенным владельцем.
func findTrustedCachedPkg(cfg Config, filename string) (string, bool) {
	pkgPath := filepath.Join(cfg.GetPaths().PkgsDir, filename)
	if !isTrustedRegularFile(pkgPath) {
		return "", false
	}
	return pkgPath, true
}

// isTrustedRegularFile проверяет, что path — регулярный файл (не симлинк)
// с доверенным владельцем.
func isTrustedRegularFile(path string) bool {
	stat, err := os.Lstat(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Debug("Не удалось прочитать файл кэша, пересборка", "path", path, "error", err)
		}
		return false
	}

	// Симлинк или не-регулярный файл в общем кэше не доверен:
	// симлинк на root-owned файл иначе прошёл бы проверку владельца
	if !stat.Mode().IsRegular() {
		slog.Debug("Файл кэша не является регулярным файлом, пересборка", "path", path)
		return false
	}

	sys, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		slog.Debug("Не удалось получить владельца файла кэша, пересборка", "path", path)
		return false
	}

	if !isTrustedOwner(sys.Uid) {
		slog.Debug("Файл кэша принадлежит недоверенному владельцу, пересборка", "path", path, "uid", sys.Uid)
		return false
	}

	return true
}

// verifyTrustedPaths перепроверяет владельца путей непосредственно перед
// установкой, закрывая TOCTOU-окно между попаданием в кэш и InstallLocal.
func verifyTrustedPaths(paths []string) error {
	for _, path := range paths {
		stat, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", gotext.Get("Unable to stat cached package"), path, err)
		}

		sys, ok := stat.Sys().(*syscall.Stat_t)
		if !ok || !stat.Mode().IsRegular() || !isTrustedOwner(sys.Uid) {
			slog.Debug("Отказ в установке пакета из кэша", "path", path)
			return fmt.Errorf("%s: %s", gotext.Get("Cached package owner changed, refusing to install"), path)
		}
	}
	return nil
}

// isTrustedOwner возвращает true, если uid — сам пользователь, root (фолбэк
// демона) или nobody (тоже фолбэк демона в общем кэше).
func isTrustedOwner(uid uint32) bool {
	return uid == uint32(os.Geteuid()) || uid == 0 || uid == 65534
}

func pkgFileName(
	input interface {
		OsInfoProvider
		PkgFormatProvider
		RepositoryProvider
	},
	vars *alrsh.Package,
) (string, error) {
	pkgInfo := getBasePkgInfo(vars, input)

	packager, err := nfpm.Get(input.PkgFormat())
	if err != nil {
		return "", err
	}

	return packager.ConventionalFileName(pkgInfo), nil
}
