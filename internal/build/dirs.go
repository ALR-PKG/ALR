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
	"path/filepath"

	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type BaseDirProvider interface {
	BaseDir() string
}

type SrcDirProvider interface {
	SrcDir() string
}

type PkgDirProvider interface {
	PkgDir() string
}

type ScriptDirProvider interface {
	ScriptDir() string
}

func getDirs(
	cfg Config,
	scriptPath string,
	basePkg string,
) (types.Directories, error) {
	return getDirsForPackage(cfg, scriptPath, basePkg, "")
}

func getDirsForPackage(
	cfg Config,
	scriptPath string,
	basePkg string,
	packageName string,
) (types.Directories, error) {
	pkgsDir := cfg.GetPaths().PkgsDir

	scriptPath, err := filepath.Abs(scriptPath)
	if err != nil {
		return types.Directories{}, err
	}
	baseDir := filepath.Join(pkgsDir, basePkg)

	// Для подпакетов используем отдельную директорию pkg_<имя_подпакета>
	// Для обычных пакетов используем просто pkg
	pkgDirName := "pkg"
	if packageName != "" {
		pkgDirName = "pkg_" + packageName
	}

	return types.Directories{
		BaseDir:   getBaseDir(cfg, basePkg),
		SrcDir:    getSrcDir(cfg, basePkg),
		PkgDir:    filepath.Join(baseDir, pkgDirName),
		ScriptDir: getScriptDir(scriptPath),
	}, nil
}

func getBaseDir(cfg Config, basePkg string) string {
	return filepath.Join(cfg.GetPaths().PkgsDir, basePkg)
}

func getSrcDir(cfg Config, basePkg string) string {
	return filepath.Join(getBaseDir(cfg, basePkg), "src")
}

func GetSrcDir(cfg Config, basePkg string) string {
	return getSrcDir(cfg, basePkg)
}

func getScriptDir(scriptPath string) string {
	return filepath.Dir(scriptPath)
}
