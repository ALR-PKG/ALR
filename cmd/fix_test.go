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
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

// holdFlockViaProbe берёт flock на path через отдельный open file
// description (без реестра fsutils), имитируя блокировку другим процессом.
// Возвращает функцию снятия блокировки.
func holdFlockViaProbe(t *testing.T, path string) func() {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o664)
	if err != nil {
		t.Fatalf("Failed to open probe lock file %s: %s", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		t.Fatalf("Failed to flock probe file %s: %s", path, err)
	}
	return func() {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
	}
}

func TestPlanFixCleanupRoot(t *testing.T) {
	tmp := t.TempDir()
	paths := &config.Paths{
		CacheDir: filepath.Join(tmp, "cache"),
		BuildDir: filepath.Join(tmp, "home", "build"),
		PkgsDir:  filepath.Join(tmp, "home", "pkgs"),
	}

	plan := planFixCleanup(paths, true)

	want := []fixAction{
		{kind: fixEnsureCacheDir, path: paths.CacheDir},
		{kind: fixClearCacheDir, path: paths.CacheDir},
		{kind: fixResetDir, path: paths.BuildDir},
		{kind: fixResetDir, path: paths.PkgsDir},
	}
	if len(plan) != len(want) {
		t.Fatalf("expected %d actions, got %d: %v", len(want), len(plan), plan)
	}
	for i, a := range plan {
		if a != want[i] {
			t.Errorf("action %d: expected %+v, got %+v", i, want[i], a)
		}
	}
}

func TestPlanFixCleanupNonRootPerUser(t *testing.T) {
	tmp := t.TempDir()
	paths := &config.Paths{
		CacheDir: filepath.Join(tmp, "cache"),
		BuildDir: filepath.Join(tmp, "home", "build"),
		PkgsDir:  filepath.Join(tmp, "home", "pkgs"),
	}

	plan := planFixCleanup(paths, false)

	// Non-root never touches the shared cache dir and needs no guarantee:
	// it operates only on paths owned by the current user.
	for _, a := range plan {
		if a.kind == fixClearCacheDir || a.kind == fixEnsureCacheDir {
			t.Errorf("non-root plan must not contain %+v", a)
		}
	}

	want := []fixAction{
		{kind: fixResetDir, path: paths.BuildDir},
		{kind: fixResetDir, path: paths.PkgsDir},
	}
	if len(plan) != len(want) {
		t.Fatalf("expected %d actions, got %d: %v", len(want), len(plan), plan)
	}
	for i, a := range plan {
		if a != want[i] {
			t.Errorf("action %d: expected %+v, got %+v", i, want[i], a)
		}
	}
}

func TestPlanFixCleanupNonRootFallbackPaths(t *testing.T) {
	tmp := t.TempDir()
	paths := &config.Paths{
		CacheDir: filepath.Join(tmp, "cache"),
		BuildDir: filepath.Join(tmp, "cache", "build"),
		PkgsDir:  filepath.Join(tmp, "cache", "pkgs"),
	}

	plan := planFixCleanup(paths, false)

	// Fallback paths live inside the shared CacheDir: prune own entries
	// only, never recreate the directories.
	want := []fixAction{
		{kind: fixPruneOwnEntries, path: paths.BuildDir},
		{kind: fixPruneOwnEntries, path: paths.PkgsDir},
	}
	if len(plan) != len(want) {
		t.Fatalf("expected %d actions, got %d: %v", len(want), len(plan), plan)
	}
	for i, a := range plan {
		if a != want[i] {
			t.Errorf("action %d: expected %+v, got %+v", i, want[i], a)
		}
	}
}

func TestPruneOwnEntries(t *testing.T) {
	dir := t.TempDir()
	mine := filepath.Join(dir, "mine")
	other := filepath.Join(dir, "other")
	for _, p := range []string{mine, other} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	uid := os.Geteuid()
	// "mine" matches the uid filter, "other" does not.
	if err := pruneOwnEntries(dir, uint32(uid), func(path string) (uint32, error) {
		if path == mine {
			return uint32(uid), nil
		}
		return uint32(uid + 1), nil
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(mine); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed", mine)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("expected %s to be kept: %v", other, err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected dir %s to be kept: %v", dir, err)
	}
}

func TestResetDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "build")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := resetDir(dir); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", dir, err)
	}
	if len(entries) != 0 {
		t.Errorf("expected %s to be empty, got %d entries", dir, len(entries))
	}
}

func TestLockedPaths(t *testing.T) {
	dir := t.TempDir()
	release := holdFlockViaProbe(t, filepath.Join(dir, "a.lock"))
	defer release()
	if err := os.WriteFile(filepath.Join(dir, "b.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	// Файл без суффикса .lock игнорируется
	if err := os.WriteFile(filepath.Join(dir, "c.txt"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	locked, err := lockedPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !locked["a.lock"] {
		t.Errorf("expected a.lock to be reported locked, got %v", locked)
	}
	if locked["b.lock"] {
		t.Errorf("expected b.lock to be reported free, got %v", locked)
	}
	if _, ok := locked["c.txt"]; ok {
		t.Errorf("expected c.txt to be ignored, got %v", locked)
	}
}

func TestLockedPathsMissingDir(t *testing.T) {
	locked, err := lockedPaths(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatal(err)
	}
	if len(locked) != 0 {
		t.Errorf("expected empty map, got %v", locked)
	}
}

func TestResetDirRespectsLocks(t *testing.T) {
	dir := t.TempDir()
	pkgDir := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "file"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := holdFlockViaProbe(t, filepath.Join(dir, "pkg.lock"))
	defer release()

	freeDir := filepath.Join(dir, "free")
	if err := os.MkdirAll(freeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "free.lock"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := resetDir(dir); err != nil {
		t.Fatal(err)
	}

	// Залоченные entry и её .lock сохраняются
	if _, err := os.Stat(pkgDir); err != nil {
		t.Errorf("expected %s to be kept: %v", pkgDir, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg.lock")); err != nil {
		t.Errorf("expected pkg.lock to be kept: %v", err)
	}
	// Свободные entry и stale .lock удаляются
	if _, err := os.Stat(freeDir); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed", freeDir)
	}
	if _, err := os.Stat(filepath.Join(dir, "free.lock")); !os.IsNotExist(err) {
		t.Errorf("expected free.lock to be removed")
	}
	// Сам каталог существует
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected dir %s to exist: %v", dir, err)
	}
}

func TestPathLocked(t *testing.T) {
	dir := t.TempDir()
	lockedPath := filepath.Join(dir, "held.lock")
	release := holdFlockViaProbe(t, lockedPath)
	defer release()
	freePath := filepath.Join(dir, "free.lock")
	if err := os.WriteFile(freePath, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	if ok, err := pathLocked(lockedPath); err != nil || !ok {
		t.Errorf("expected %s locked=true, got %v (err=%v)", lockedPath, ok, err)
	}
	if ok, err := pathLocked(freePath); err != nil || ok {
		t.Errorf("expected %s locked=false, got %v (err=%v)", freePath, ok, err)
	}
}

func TestClearCacheDirRespectsNestedLocks(t *testing.T) {
	cacheDir := t.TempDir()
	repoDir := filepath.Join(cacheDir, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "metadata"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Лок pull'а лежит на уровень 2 от CacheDir: repo/x.lock
	release := holdFlockViaProbe(t, filepath.Join(repoDir, "x.lock"))
	defer release()

	if err := os.WriteFile(filepath.Join(cacheDir, "alr.db"), []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := clearCacheDir(cacheDir); err != nil {
		t.Fatal(err)
	}

	// Каталог с активным вложенным локом сохраняется целиком
	if _, err := os.Stat(filepath.Join(repoDir, "metadata")); err != nil {
		t.Errorf("expected repo to be kept (active nested lock): %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "x.lock")); err != nil {
		t.Errorf("expected repo/x.lock to be kept: %v", err)
	}
	// Свободный sibling удалён
	if _, err := os.Stat(filepath.Join(cacheDir, "alr.db")); !os.IsNotExist(err) {
		t.Errorf("expected alr.db to be removed")
	}
}

func TestResetDirRespectsNestedLocks(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "data"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	release := holdFlockViaProbe(t, filepath.Join(sub, "inner.lock"))
	defer release()

	freeSub := filepath.Join(dir, "free")
	if err := os.MkdirAll(freeSub, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := resetDir(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(sub, "data")); err != nil {
		t.Errorf("expected sub to be kept (active nested lock): %v", err)
	}
	if _, err := os.Stat(freeSub); !os.IsNotExist(err) {
		t.Errorf("expected %s to be removed", freeSub)
	}
}

func TestPathLockedDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, ".alr-cache.lock")

	ok, err := pathLocked(missing)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected missing lock file to be reported free")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("pathLocked must not create %s", missing)
	}
}
