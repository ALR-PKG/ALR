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
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

// Тесты подменяют переменные-зависимости (geteuid, userCacheDir и т.д.)
// и не параллельны: глобальное состояние восстанавливается через t.Cleanup.

func stubVar[T any](t *testing.T, ptr *T, v T) {
	t.Helper()
	orig := *ptr
	*ptr = v
	t.Cleanup(func() { *ptr = orig })
}

// stubPrivilegedUser делает вид, что текущий пользователь — член общей группы
func stubPrivilegedUser(t *testing.T) {
	t.Helper()
	stubVar(t, &geteuid, func() int { return 1000 })
	stubVar(t, &currentUserGroupGIDs, func() (map[uint32]bool, error) {
		return map[uint32]bool{1000: true}, nil
	})
	stubVar(t, &userInGroups, func(map[uint32]bool, ...string) bool { return true })
}

// stubUnprivilegedUser делает вид, что текущий пользователь не входит
// ни в одну кандидатную группу
func stubUnprivilegedUser(t *testing.T, ucd string) {
	t.Helper()
	stubVar(t, &geteuid, func() int { return 1000 })
	stubVar(t, &currentUserGroupGIDs, func() (map[uint32]bool, error) {
		return map[uint32]bool{1000: true}, nil
	})
	stubVar(t, &userInGroups, func(map[uint32]bool, ...string) bool { return false })
	stubVar(t, &userCacheDir, func() (string, error) { return ucd, nil })
}

// Контракт проверки свободного места: CheckFreeSpace для выбранного корня
// кэша скачиваний выполняется только если есть что скачивать. Без скачиваний
// dlcache вообще не трогает ФС (каталог dl создаётся на первое скачивание),
// а корень кэша может ещё не существовать — требовать место не у чего.

func TestDownloadSources_NoSpaceCheckWithoutSources(t *testing.T) {
	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: filepath.Join(t.TempDir(), "missing")},
	}
	dl := NewSourceDownloader(cfg)

	err := dl.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{})
	require.NoError(t, err)
}

func TestDownloadSources_ChecksSpaceBeforeDownloading(t *testing.T) {
	// root → общий CacheDir без подготовки: несуществующий каталог,
	// Statfs внутри CheckFreeSpace падает раньше, чем начнётся скачивание
	// мусорного URL — ошибка именно про место.
	stubVar(t, &geteuid, func() int { return 0 })

	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: filepath.Join(t.TempDir(), "missing")},
	}
	d := NewSourceDownloader(cfg)

	err := d.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{
		Sources:   []string{"not-a-url"},
		Checksums: []string{"SKIP"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось проверить свободное место")
}

func TestDownloadSources_EnoughSpacePassesCheck(t *testing.T) {
	stubVar(t, &geteuid, func() int { return 0 })

	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: t.TempDir()},
	}
	d := NewSourceDownloader(cfg)

	// Места достаточно — проверка проходит, ошибка дальше уже от попытки
	// скачать мусорный URL, а не от CheckFreeSpace.
	err := d.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{
		Sources:   []string{"not-a-url"},
		Checksums: []string{"SKIP"},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "свободное место")
}

func TestResolveDlCacheRoot_RootUsesShared(t *testing.T) {
	stubVar(t, &geteuid, func() int { return 0 })
	ensureCalled := false
	stubVar(t, &ensureSharedDir, func(string, os.FileMode) error {
		ensureCalled = true
		return nil
	})

	shared := filepath.Join(t.TempDir(), "cache")
	cfg := &fakeResolverConfig{paths: &config.Paths{CacheDir: shared}}

	root, err := resolveDlCacheRoot(cfg)
	require.NoError(t, err)
	assert.Equal(t, shared, root)
	assert.False(t, ensureCalled, "для root подготовка каталога не нужна")
}

func TestResolveDlCacheRoot_MemberUsesShared(t *testing.T) {
	stubPrivilegedUser(t)
	var ensuredPath string
	var ensuredMode os.FileMode
	stubVar(t, &ensureSharedDir, func(path string, mode os.FileMode) error {
		ensuredPath, ensuredMode = path, mode
		return nil
	})

	shared := filepath.Join(t.TempDir(), "cache")
	cfg := &fakeResolverConfig{paths: &config.Paths{CacheDir: shared}}

	root, err := resolveDlCacheRoot(cfg)
	require.NoError(t, err)
	assert.Equal(t, shared, root)
	assert.Equal(t, filepath.Join(shared, "dl"), ensuredPath)
	assert.Equal(t, os.FileMode(0o2775), ensuredMode)
}

func TestResolveDlCacheRoot_MemberFallsBackOnEnsureError(t *testing.T) {
	stubPrivilegedUser(t)
	ucd := t.TempDir()
	stubVar(t, &userCacheDir, func() (string, error) { return ucd, nil })
	stubVar(t, &ensureSharedDir, func(string, os.FileMode) error {
		return errors.New("sudo недоступен")
	})

	shared := filepath.Join(t.TempDir(), "cache")
	cfg := &fakeResolverConfig{paths: &config.Paths{CacheDir: shared}}

	root, err := resolveDlCacheRoot(cfg)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(ucd, "alr"), root)
	assert.DirExists(t, filepath.Join(ucd, "alr", "dl"))
}

func TestResolveDlCacheRoot_MemberFallbackKeepsSharedError(t *testing.T) {
	stubPrivilegedUser(t)
	stubVar(t, &userCacheDir, func() (string, error) {
		return "", errors.New("нет HOME")
	})
	ensureErr := errors.New("sudo недоступен")
	stubVar(t, &ensureSharedDir, func(string, os.FileMode) error { return ensureErr })

	cfg := &fakeResolverConfig{paths: &config.Paths{CacheDir: t.TempDir()}}

	_, err := resolveDlCacheRoot(cfg)
	require.ErrorIs(t, err, ensureErr,
		"при неудаче per-user фолбэка возвращается исходная ошибка общего пути")
}

func TestResolveDlCacheRoot_NonMemberUsesPerUser(t *testing.T) {
	ucd := t.TempDir()
	stubUnprivilegedUser(t, ucd)
	stubVar(t, &ensureSharedDir, func(string, os.FileMode) error {
		t.Fatal("ensureSharedDir не должен вызываться: у пользователя нет прав, sudo не дёргаем")
		return nil
	})

	shared := filepath.Join(t.TempDir(), "cache")
	cfg := &fakeResolverConfig{paths: &config.Paths{CacheDir: shared}}

	root, err := resolveDlCacheRoot(cfg)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(ucd, "alr"), root)
	assert.DirExists(t, filepath.Join(ucd, "alr", "dl"))
	// Общий каталог не создавался
	_, statErr := os.Stat(shared)
	assert.True(t, os.IsNotExist(statErr))
}

func TestResolveDlCacheRoot_NonMemberUserCacheDirError(t *testing.T) {
	stubVar(t, &geteuid, func() int { return 1000 })
	stubVar(t, &currentUserGroupGIDs, func() (map[uint32]bool, error) {
		return map[uint32]bool{1000: true}, nil
	})
	stubVar(t, &userInGroups, func(map[uint32]bool, ...string) bool { return false })
	ucdErr := errors.New("нет HOME")
	stubVar(t, &userCacheDir, func() (string, error) { return "", ucdErr })

	cfg := &fakeResolverConfig{paths: &config.Paths{CacheDir: t.TempDir()}}

	_, err := resolveDlCacheRoot(cfg)
	require.ErrorIs(t, err, ucdErr)
}

func TestDownloadSources_NonMemberUsesPerUserCache(t *testing.T) {
	ucd := t.TempDir()
	stubUnprivilegedUser(t, ucd)
	stubVar(t, &ensureSharedDir, func(string, os.FileMode) error {
		t.Fatal("ensureSharedDir не должен вызываться для пользователя без групп")
		return nil
	})

	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: filepath.Join(t.TempDir(), "missing")},
	}
	d := NewSourceDownloader(cfg)

	// Per-user корень создаётся и место проверяется на нём: ошибка дальше —
	// от попытки скачать мусорный URL, а не про место или права.
	err := d.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{
		Sources:   []string{"not-a-url"},
		Checksums: []string{"SKIP"},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "свободное место")
	assert.DirExists(t, filepath.Join(ucd, "alr", "dl"))
}
