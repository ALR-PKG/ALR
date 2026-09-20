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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

func TestRemoveDuplicatesSources(t *testing.T) {
	type testCase struct {
		Name         string
		Sources      []string
		Checksums    []string
		NewSources   []string
		NewChecksums []string
	}

	for _, tc := range []testCase{{
		Name:         "prefer non-skip values",
		Sources:      []string{"a", "b", "c", "a"},
		Checksums:    []string{"skip", "skip", "skip", "1"},
		NewSources:   []string{"a", "b", "c"},
		NewChecksums: []string{"1", "skip", "skip"},
	}} {
		t.Run(tc.Name, func(t *testing.T) {
			s, c := removeDuplicatesSources(tc.Sources, tc.Checksums)
			assert.Equal(t, s, tc.NewSources)
			assert.Equal(t, c, tc.NewChecksums)
		})
	}
}

func TestCopyToCache(t *testing.T) {
	const testKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

	t.Run("copies file to pkgs dir and removes source", func(t *testing.T) {
		pkgsDir := filepath.Join(t.TempDir(), "pkgs")
		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("package-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		dstPath, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst"), dstPath)

		data, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("package-data"), data)

		fi, err := os.Stat(dstPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm())

		_, err = os.Stat(srcPath)
		assert.True(t, os.IsNotExist(err), "источник должен быть удалён из дерева сборки")

		tmpFiles, err := filepath.Glob(filepath.Join(pkgsDir, "*.tmp-*"))
		require.NoError(t, err)
		assert.Empty(t, tmpFiles, "в кэше не должно оставаться временных файлов")
	})

	t.Run("writes meta file with cache key next to package", func(t *testing.T) {
		pkgsDir := t.TempDir()
		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("package-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		dstPath, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
		require.NoError(t, err)

		metaPath := dstPath + ".meta"
		data, err := os.ReadFile(metaPath)
		require.NoError(t, err, "рядом с пакетом должна лежать мета")

		var meta cacheMeta
		require.NoError(t, json.Unmarshal(data, &meta))
		assert.Equal(t, testKey, meta.Key)
		assert.Equal(t, cacheMetaVersion, meta.Version, "мета должна нести версию формата")

		fi, err := os.Stat(metaPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm())

		tmpFiles, err := filepath.Glob(filepath.Join(pkgsDir, "*.tmp-*"))
		require.NoError(t, err)
		assert.Empty(t, tmpFiles, "мета публикуется атомарно, временных файлов быть не должно")
	})

	t.Run("replaces existing file in cache", func(t *testing.T) {
		pkgsDir := t.TempDir()
		dstPath := filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(dstPath, []byte("old-data"), 0o644))
		require.NoError(t, os.WriteFile(dstPath+".meta", []byte(`{"key":"old-key"}`), 0o644))

		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("new-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		gotPath, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
		require.NoError(t, err)
		assert.Equal(t, dstPath, gotPath)

		data, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("new-data"), data)

		metaData, err := os.ReadFile(dstPath + ".meta")
		require.NoError(t, err)
		var meta cacheMeta
		require.NoError(t, json.Unmarshal(metaData, &meta))
		assert.Equal(t, testKey, meta.Key, "мета должна обновиться вместе с пакетом")
	})

	t.Run("missing source returns error and cache stays empty", func(t *testing.T) {
		pkgsDir := t.TempDir()
		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		_, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
		require.Error(t, err)

		entries, err := os.ReadDir(pkgsDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "в кэше не должно быть ни пакета, ни временных файлов")
	})

	t.Run("copy error leaves no partial file in cache", func(t *testing.T) {
		pkgsDir := t.TempDir()
		// Каталог как источник: Open успешен, но чтение падает — io.Copy вернёт ошибку
		srcPath := filepath.Join(t.TempDir(), "src")
		require.NoError(t, os.MkdirAll(srcPath, 0o755))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		_, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
		require.Error(t, err)

		entries, err := os.ReadDir(pkgsDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "в кэше не должно остаться частичного или временного файла")
	})

	t.Run("meta write error removes published package", func(t *testing.T) {
		pkgsDir := t.TempDir()
		// Каталог с именем будущей меты: rename временного файла на него упадёт,
		// то есть мета не сможет быть опубликована после публикации пакета
		metaPath := filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst.meta")
		require.NoError(t, os.MkdirAll(metaPath, 0o755))

		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("package-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		_, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
		require.Error(t, err)

		_, statErr := os.Stat(filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst"))
		assert.True(t, os.IsNotExist(statErr), "пакет без меты не должен оставаться в кэше")

		tmpFiles, err := filepath.Glob(filepath.Join(pkgsDir, "*.tmp-*"))
		require.NoError(t, err)
		assert.Empty(t, tmpFiles, "временных файлов быть не должно")
	})

	t.Run("waits for cache lock held by another process", func(t *testing.T) {
		pkgsDir := t.TempDir()
		lockPath := filepath.Join(pkgsDir, ".alr-cache.lock")

		// flock через отдельный open file description — имитация блокировки
		// кэша другим процессом (демоном или параллельной сборкой)
		probe, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o664)
		require.NoError(t, err)
		require.NoError(t, unix.Flock(int(probe.Fd()), unix.LOCK_EX|unix.LOCK_NB))

		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("package-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		done := make(chan error, 1)
		go func() {
			_, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst", testKey)
			done <- err
		}()

		// Пока probe держит flock, публикация обязана ждать
		select {
		case err := <-done:
			t.Fatalf("ожидалось блокирующее ожидание лока кэша, copyToCache завершилась: %v", err)
		case <-time.After(200 * time.Millisecond):
		}

		_, statErr := os.Stat(filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst"))
		assert.True(t, os.IsNotExist(statErr), "до снятия лока пакет не должен быть опубликован")

		require.NoError(t, unix.Flock(int(probe.Fd()), unix.LOCK_UN))
		require.NoError(t, probe.Close())

		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("copyToCache не получила лок кэша после его освобождения")
		}

		_, err = os.Stat(filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst"))
		require.NoError(t, err, "после снятия лока пакет должен быть опубликован")
		_, err = os.Stat(filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst.meta"))
		require.NoError(t, err, "после снятия лока мета должна быть опубликована")
	})
}
