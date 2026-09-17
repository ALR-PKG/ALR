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
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/goreleaser/nfpm/v2"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

type Cache struct {
	cfg Config
}

func (c *Cache) CheckForBuiltPackage(
	ctx context.Context,
	input *BuildInput,
	vars *alrsh.Package,
) (string, bool, error) {
	filename, err := pkgFileName(input, vars)
	if err != nil {
		return "", false, err
	}

	path, found := findTrustedCachedPkg(c.cfg, filename)
	return path, found, nil
}

// findTrustedCachedPkg ищет готовый пакет в плоском кэше PkgsDir и
// возвращает его, только если владелец файла доверенный.
func findTrustedCachedPkg(cfg Config, filename string) (string, bool) {
	pkgPath := filepath.Join(cfg.GetPaths().PkgsDir, filename)

	stat, err := os.Stat(pkgPath)
	if err != nil {
		return "", false
	}

	sys, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		slog.Debug("Не удалось получить владельца файла кэша, пересборка", "path", pkgPath)
		return "", false
	}

	if !isTrustedOwner(sys.Uid) {
		slog.Debug("Пакет в кэше принадлежит недоверенному владельцу, пересборка", "path", pkgPath, "uid", sys.Uid)
		return "", false
	}

	return pkgPath, true
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
