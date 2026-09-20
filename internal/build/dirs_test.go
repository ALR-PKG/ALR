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

package build

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

func TestGetDirsUsesBuildDir(t *testing.T) {
	buildDir := filepath.Join(t.TempDir(), "build")
	pkgsDir := filepath.Join(t.TempDir(), "pkgs")

	cfg := &fakeResolverConfig{
		paths: &config.Paths{BuildDir: buildDir, PkgsDir: pkgsDir},
	}

	dirs, err := getDirs(cfg, "somepkg/alr.sh", "somepkg")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(buildDir, "somepkg"), dirs.BaseDir)
	assert.Equal(t, filepath.Join(buildDir, "somepkg", "src"), dirs.SrcDir)
	assert.Equal(t, filepath.Join(buildDir, "somepkg", "pkg"), dirs.PkgDir)
}

func TestGetDirsForPackageSubpackageUsesBuildDir(t *testing.T) {
	buildDir := filepath.Join(t.TempDir(), "build")
	pkgsDir := filepath.Join(t.TempDir(), "pkgs")

	cfg := &fakeResolverConfig{
		paths: &config.Paths{BuildDir: buildDir, PkgsDir: pkgsDir},
	}

	dirs, err := getDirsForPackage(cfg, "somepkg/alr.sh", "somepkg", "subpackage")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(buildDir, "somepkg"), dirs.BaseDir)
	assert.Equal(t, filepath.Join(buildDir, "somepkg", "pkg_subpackage"), dirs.PkgDir)
}

func TestGetBaseDirAndSrcDirUseBuildDir(t *testing.T) {
	buildDir := filepath.Join(t.TempDir(), "build")
	pkgsDir := filepath.Join(t.TempDir(), "pkgs")

	cfg := &fakeResolverConfig{
		paths: &config.Paths{BuildDir: buildDir, PkgsDir: pkgsDir},
	}

	assert.Equal(t, filepath.Join(buildDir, "somepkg"), getBaseDir(cfg, "somepkg"))
	assert.Equal(t, filepath.Join(buildDir, "somepkg", "src"), getSrcDir(cfg, "somepkg"))
	assert.Equal(t, filepath.Join(buildDir, "somepkg", "src"), GetSrcDir(cfg, "somepkg"))
}
