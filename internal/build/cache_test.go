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
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
)

const testFuncsKey = "test-funcs-key"

func newTestCacheInput() (*BuildInput, *alrsh.Package) {
	input := &BuildInput{
		info:       &distro.OSRelease{},
		pkgFormat:  "apk",
		repository: "test-repo",
	}
	vars := &alrsh.Package{
		Name:    "somepkg",
		Version: "1.0",
		Release: 1,
	}
	return input, vars
}

// placeCachedPkg кладёт пакет в PkgsDir и пишет ему мету с ключом,
// соответствующим vars и funcsKey (как это делает copyToCache).
func placeCachedPkg(t *testing.T, pkgsDir, filename string, vars *alrsh.Package, funcsKey string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename), []byte("pkg"), 0o644))
	require.NoError(t, writeCacheMeta(pkgsDir, filename, SemanticCacheKey(vars, funcsKey)))
}

func TestCheckForBuiltPackage(t *testing.T) {
	t.Run("finds package in PkgsDir", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		placeCachedPkg(t, pkgsDir, filename, vars, testFuncsKey)

		path, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.True(t, found)
		assert.Equal(t, filepath.Join(pkgsDir, filename), path)
	})

	t.Run("missing package is not found", func(t *testing.T) {
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: t.TempDir()},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("package without meta is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename), []byte("pkg"), 0o644))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("broken meta is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename), []byte("pkg"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename+".meta"), []byte("{not json"), 0o644))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("meta with unknown version is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename), []byte("pkg"), 0o644))
		meta := `{"v":999,"key":"` + SemanticCacheKey(vars, testFuncsKey) + `"}`
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename+".meta"), []byte(meta), 0o644))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("meta key mismatch is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		// Мета записана для другого vars (другая версия пакета)
		otherVars := &alrsh.Package{Name: "somepkg", Version: "2.0", Release: 1}
		placeCachedPkg(t, pkgsDir, filename, otherVars, testFuncsKey)

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("funcsKey mismatch is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		placeCachedPkg(t, pkgsDir, filename, vars, "old-funcs-key")

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("non-regular meta is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename), []byte("pkg"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(pkgsDir, filename+".meta"), 0o755))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("meta symlink is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(pkgsDir, filename), []byte("pkg"), 0o644))
		// Симлинк на валидную мету не доверен: цель могла бы быть подменена
		realMeta := filepath.Join(t.TempDir(), "real.meta")
		require.NoError(t, os.WriteFile(realMeta,
			[]byte(`{"v":1,"key":"`+SemanticCacheKey(vars, testFuncsKey)+`"}`), 0o644))
		require.NoError(t, os.Symlink(realMeta, filepath.Join(pkgsDir, filename+".meta")))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("symlink in PkgsDir is not a cache hit", func(t *testing.T) {
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		target := filepath.Join(t.TempDir(), "target.pkg")
		require.NoError(t, os.WriteFile(target, []byte("pkg"), 0o644))
		require.NoError(t, os.Symlink(target, filepath.Join(pkgsDir, filename)))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})

	t.Run("package in build tree is not a cache hit", func(t *testing.T) {
		// Старая схема: пакет лежал в дереве сборки (<BaseDir>/<filename>).
		// После переезда кэша в PkgsDir такой файл не должен засчитываться.
		buildDir := t.TempDir()
		pkgsDir := t.TempDir()
		cfg := &fakeResolverConfig{
			paths: &config.Paths{BuildDir: buildDir, PkgsDir: pkgsDir},
		}
		cache := &Cache{cfg}
		input, vars := newTestCacheInput()

		filename, err := pkgFileName(input, vars)
		require.NoError(t, err)
		oldPath := filepath.Join(buildDir, vars.Name, filename)
		require.NoError(t, os.MkdirAll(filepath.Dir(oldPath), 0o755))
		require.NoError(t, os.WriteFile(oldPath, []byte("pkg"), 0o644))

		_, found, err := cache.CheckForBuiltPackage(context.Background(), input, vars, testFuncsKey)
		require.NoError(t, err)
		assert.False(t, found)
	})
}

func TestVerifyTrustedPaths(t *testing.T) {
	t.Run("own regular file is accepted", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "pkg-1.0.apk")
		require.NoError(t, os.WriteFile(path, []byte("pkg"), 0o644))

		assert.NoError(t, verifyTrustedPaths([]string{path}))
	})

	t.Run("missing file returns error", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "missing.pkg")

		assert.Error(t, verifyTrustedPaths([]string{path}))
	})

	t.Run("symlink is rejected", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target.pkg")
		require.NoError(t, os.WriteFile(target, []byte("pkg"), 0o644))
		link := filepath.Join(t.TempDir(), "link.pkg")
		require.NoError(t, os.Symlink(target, link))

		assert.Error(t, verifyTrustedPaths([]string{link}))
	})
}

func TestIsTrustedOwner(t *testing.T) {
	t.Run("trusted", func(t *testing.T) {
		assert.True(t, isTrustedOwner(uint32(os.Geteuid())))
		assert.True(t, isTrustedOwner(0))
		assert.True(t, isTrustedOwner(65534))
	})

	t.Run("untrusted", func(t *testing.T) {
		assert.False(t, isTrustedOwner(424242))
	})
}
