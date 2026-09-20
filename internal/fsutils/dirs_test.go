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

package fsutils

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// Тесты подменяют systemCachePrefix и PATH (фейковый sudo), поэтому
// не параллельны: глобальное состояние восстанавливается через t.Cleanup.

func stubSystemCachePrefix(t *testing.T, dir string) {
	t.Helper()
	orig := systemCachePrefix
	systemCachePrefix = dir
	t.Cleanup(func() { systemCachePrefix = orig })
}

// stubFailingSudo кладёт в PATH фейковый sudo, который записывает свои
// аргументы в файл и завершается с ошибкой. Возвращает путь к файлу вызовов.
func stubFailingSudo(t *testing.T) string {
	t.Helper()
	binDir := t.TempDir()
	callsFile := filepath.Join(t.TempDir(), "sudo-calls")
	script := "#!/bin/sh\necho \"$@\" >> \"" + callsFile + "\"\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "sudo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return callsFile
}

func sudoCalled(t *testing.T, callsFile string) bool {
	t.Helper()
	data, err := os.ReadFile(callsFile)
	if err != nil {
		return false
	}
	return len(data) > 0
}

// makeSharedDir создаёт каталог, принадлежащий основной группе текущего
// пользователя, с указанными правами.
func makeSharedDir(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	// chown в свою группу разрешён без root
	if err := os.Chown(path, os.Getuid(), os.Getegid()); err != nil {
		t.Fatalf("chown: %s", err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatalf("chmod: %s", err)
	}
}

func statGID(t *testing.T, path string) uint32 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("не удалось получить Stat_t для %s", path)
	}
	return stat.Gid
}

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("fast-path не используется под root")
	}
}

func TestEnsureTempDirExistingSharedDir(t *testing.T) {
	skipIfRoot(t)
	t.Setenv("CI", "")
	callsFile := stubFailingSudo(t)
	prefix := t.TempDir()
	stubSystemCachePrefix(t, prefix)

	dir := filepath.Join(prefix, "cache")
	makeSharedDir(t, dir, os.ModeSetgid|0o0775)

	if err := EnsureTempDirWithRootOwner(dir, 0o2775); err != nil {
		t.Fatalf("ожидался nil для общего каталога, получено: %s", err)
	}
	if sudoCalled(t, callsFile) {
		t.Error("sudo не должен вызываться для корректного общего каталога")
	}
}

func TestEnsureTempDirCreatesUnderSharedAncestor(t *testing.T) {
	skipIfRoot(t)
	t.Setenv("CI", "")
	callsFile := stubFailingSudo(t)
	prefix := t.TempDir()
	stubSystemCachePrefix(t, prefix)

	parent := filepath.Join(prefix, "cache")
	makeSharedDir(t, parent, os.ModeSetgid|0o0775)
	dir := filepath.Join(parent, "child")

	if err := EnsureTempDirWithRootOwner(dir, 0o2775); err != nil {
		t.Fatalf("ожидался nil при создании под общим предком, получено: %s", err)
	}
	if sudoCalled(t, callsFile) {
		t.Error("sudo не должен вызываться при создании под общим предком")
	}

	myGID := uint32(os.Getegid())
	if got := statGID(t, dir); got != myGID {
		t.Errorf("группа нового каталога = %d, ожидалось наследование %d", got, myGID)
	}
	if !dirSharedWith(dir, map[uint32]bool{myGID: true}) {
		t.Error("новый каталог должен быть 2775 (setgid + group rwx)")
	}
}

func TestEnsureTempDirCreatesIntermediateDirs(t *testing.T) {
	skipIfRoot(t)
	t.Setenv("CI", "")
	callsFile := stubFailingSudo(t)
	prefix := t.TempDir()
	stubSystemCachePrefix(t, prefix)

	parent := filepath.Join(prefix, "cache")
	makeSharedDir(t, parent, os.ModeSetgid|0o0775)
	dir := filepath.Join(parent, "a", "b")

	if err := EnsureTempDirWithRootOwner(dir, 0o2775); err != nil {
		t.Fatalf("ожидался nil при создании вложенных каталогов, получено: %s", err)
	}
	if sudoCalled(t, callsFile) {
		t.Error("sudo не должен вызываться при создании под общим предком")
	}

	myGID := uint32(os.Getegid())
	// Промежуточный каталог наследует группу и setgid от родителя (ядро)
	intermediate := filepath.Join(parent, "a")
	if got := statGID(t, intermediate); got != myGID {
		t.Errorf("группа промежуточного каталога = %d, ожидалось %d", got, myGID)
	}
	if !dirSharedWith(intermediate, map[uint32]bool{myGID: true}) {
		t.Error("промежуточный каталог должен унаследовать setgid")
	}
	if !dirSharedWith(dir, map[uint32]bool{myGID: true}) {
		t.Error("конечный каталог должен быть 2775 (setgid + group rwx)")
	}
}

func TestEnsureTempDirEscalatesOnForeignDir(t *testing.T) {
	skipIfRoot(t)
	t.Setenv("CI", "")
	prefix := t.TempDir()
	stubSystemCachePrefix(t, prefix)

	t.Run("без setgid", func(t *testing.T) {
		callsFile := stubFailingSudo(t)
		dir := filepath.Join(prefix, "nosetgid")
		makeSharedDir(t, dir, 0o0775)

		err := EnsureTempDirWithRootOwner(dir, 0o2775)
		if err == nil {
			t.Error("ожидалась ошибка эскалации (фейковый sudo падает)")
		}
		if !sudoCalled(t, callsFile) {
			t.Error("ожидался вызов sudo для каталога без setgid")
		}
	})

	t.Run("предок не общий", func(t *testing.T) {
		callsFile := stubFailingSudo(t)
		// prefix (t.TempDir) — 0700 без setgid, fast-path не сработает
		dir := filepath.Join(prefix, "missing", "child")

		err := EnsureTempDirWithRootOwner(dir, 0o2775)
		if err == nil {
			t.Error("ожидалась ошибка эскалации (фейковый sudo падает)")
		}
		if !sudoCalled(t, callsFile) {
			t.Error("ожидался вызов sudo для каталога без общего предка")
		}
	})
}

func TestEnsureTempDirPlainPath(t *testing.T) {
	callsFile := stubFailingSudo(t)
	stubSystemCachePrefix(t, t.TempDir())

	dir := filepath.Join(t.TempDir(), "plain", "nested")
	if err := EnsureTempDirWithRootOwner(dir, 0o755); err != nil {
		t.Fatalf("ожидался nil для обычного пути, получено: %s", err)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("каталог не создан: %v", err)
	}
	if sudoCalled(t, callsFile) {
		t.Error("sudo не должен вызываться для путей вне префикса кэша")
	}
}
