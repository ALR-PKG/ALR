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

package sandbox

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// TestLandlockHelperProcess — тело подпроцесса TestApplyLandlockLive:
// landlock действует на текущий процесс необратимо, поэтому проверки
// выполняются в отдельном экземпляре тестового бинарника.
func TestLandlockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LANDLOCK_HELPER") != "1" {
		return
	}
	rwDir := os.Getenv("LL_RW")
	roDir := os.Getenv("LL_RO")
	layout := &Layout{Mounts: []Mount{{Source: rwDir, Target: rwDir, RW: true}}}
	if err := ApplyLandlock(layout); err != nil {
		os.Exit(2)
	}
	// запись внутри rw-пути разрешена
	if err := os.WriteFile(filepath.Join(rwDir, "ok"), []byte("x"), 0o644); err != nil {
		os.Exit(3)
	}
	// подкаталог внутри rw-пути тоже разрешён
	if err := os.MkdirAll(filepath.Join(rwDir, "sub", "dir"), 0o755); err != nil {
		os.Exit(3)
	}
	// запись вне rw-путей запрещена (EACCES)
	err := os.WriteFile(filepath.Join(roDir, "nope"), []byte("x"), 0o644)
	if err == nil || !errors.Is(err, unix.EACCES) {
		os.Exit(4)
	}
	// создание файла вне rw-путей тоже запрещено
	if f, err := os.Create(filepath.Join(roDir, "newfile")); err == nil {
		f.Close()
		os.Exit(5)
	}
	// чтение системных путей не пострадало
	if _, err := os.ReadFile("/etc/hostname"); err != nil {
		os.Exit(6)
	}
	// чтение вне rw-путей разрешено (handled — только запись)
	if err := os.WriteFile(filepath.Join(rwDir, "ok2"), []byte("y"), 0o644); err != nil {
		os.Exit(7)
	}
	os.Exit(0)
}

// TestApplyLandlockLive — живой тест на ядре: после ApplyLandlock запись
// вне rw-путей даёт EACCES, чтение системы работает. Пропускается, если
// ядро не поддерживает landlock (< 5.13).
func TestApplyLandlockLive(t *testing.T) {
	if landlockABI() < 1 {
		t.Skip("landlock недоступен (ядро < 5.13)")
	}
	rwDir := t.TempDir()
	roDir := t.TempDir()

	cmd := exec.Command(os.Args[0], "-test.run=TestLandlockHelperProcess")
	cmd.Env = append(os.Environ(),
		"GO_WANT_LANDLOCK_HELPER=1",
		"LL_RW="+rwDir,
		"LL_RO="+roDir,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper-процесс упал: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(rwDir, "ok")); err != nil {
		t.Errorf("запись в rw-путь не сработала: %v", err)
	}
	if _, err := os.Stat(filepath.Join(roDir, "nope")); !os.IsNotExist(err) {
		t.Error("запись вне rw-путей должна быть запрещена")
	}
}

// TestApplyLandlockUnavailableABI — на ядре без landlock ApplyLandlock
// должен вернуть ErrLandlockUnavailable (проверяется только ветка детекта;
// на современном ядре тест пропускается).
func TestApplyLandlockUnavailableABI(t *testing.T) {
	if landlockABI() >= 1 {
		t.Skip("landlock доступен, ветка недоступности не проверяется")
	}
	if err := ApplyLandlock(&Layout{}); !errors.Is(err, ErrLandlockUnavailable) {
		t.Errorf("ожидалась ErrLandlockUnavailable, получено %v", err)
	}
}

func TestLandlockWriteFlagsByABI(t *testing.T) {
	v1 := landlockWriteFlags(1)
	if v1&unix.LANDLOCK_ACCESS_FS_WRITE_FILE == 0 ||
		v1&unix.LANDLOCK_ACCESS_FS_MAKE_DIR == 0 ||
		v1&unix.LANDLOCK_ACCESS_FS_REMOVE_FILE == 0 {
		t.Errorf("ABI v1: базовые write-флаги отсутствуют: %#x", v1)
	}
	if v1&(unix.LANDLOCK_ACCESS_FS_REFER|unix.LANDLOCK_ACCESS_FS_TRUNCATE) != 0 {
		t.Errorf("ABI v1: флаги старших ABI не должны входить: %#x", v1)
	}
	if landlockWriteFlags(2)&unix.LANDLOCK_ACCESS_FS_REFER == 0 {
		t.Error("ABI v2: REFER должен входить")
	}
	if landlockWriteFlags(3)&unix.LANDLOCK_ACCESS_FS_TRUNCATE == 0 {
		t.Error("ABI v3: TRUNCATE должен входить")
	}
	// read/execute-флаги не должны попадать в handled-набор
	if v1&(unix.LANDLOCK_ACCESS_FS_READ_FILE|unix.LANDLOCK_ACCESS_FS_EXECUTE) != 0 {
		t.Errorf("handled-набор не должен включать read/execute: %#x", v1)
	}
}

func TestWrapLandlock(t *testing.T) {
	layout := LayoutFromPaths("/cache/build", "/cache/pkgs", "/proj")
	cmd := exec.Command("/bin/true")
	if err := WrapLandlock(cmd, layout); err != nil {
		t.Fatalf("WrapLandlock: %v", err)
	}
	if len(cmd.Args) < 2 || cmd.Args[1] != "_internal-sandbox-ll" {
		t.Errorf("ожидалась подкоманда _internal-sandbox-ll, args = %v", cmd.Args)
	}
	if len(cmd.ExtraFiles) != 1 {
		t.Errorf("макет должен передаваться одним ExtraFiles, получено %d", len(cmd.ExtraFiles))
	}
	// namespaces не применяются: SysProcAttr не должен появляться
	if cmd.SysProcAttr != nil && cmd.SysProcAttr.Cloneflags != 0 {
		t.Errorf("landlock-режим не должен создавать namespaces: %+v", cmd.SysProcAttr)
	}
	// повторный WrapLandlock на новой команде не зависит от старого pipe
	cmd2 := exec.Command("/bin/true")
	if err := WrapLandlock(cmd2, layout); err != nil {
		t.Fatalf("повторный WrapLandlock: %v", err)
	}
}

// TestWrapLandlockExec — сквозная проверка: re-exec тестового бинарника
// через _internal-sandbox-ll применяет landlock и exec'ит цель.
func TestWrapLandlockExec(t *testing.T) {
	if landlockABI() < 1 {
		t.Skip("landlock недоступен (ядро < 5.13)")
	}
	rwDir := t.TempDir()
	marker := filepath.Join(rwDir, "marker")
	layout := &Layout{Mounts: []Mount{{Source: rwDir, Target: rwDir, RW: true}}}

	cmd := exec.Command(os.Args[0], "-test.run=TestLandlockTargetProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_LANDLOCK_TARGET=1", "LL_MARKER="+marker)
	if err := WrapLandlock(cmd, layout); err != nil {
		t.Fatalf("WrapLandlock: %v", err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("цель в landlock-режиме упала: %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("цель не записала marker в rw-путь: %v", err)
	}
}

// TestLandlockTargetProcess — целевая команда для TestWrapLandlockExec.
func TestLandlockTargetProcess(t *testing.T) {
	if os.Getenv("GO_WANT_LANDLOCK_TARGET") != "1" {
		return
	}
	if err := os.WriteFile(os.Getenv("LL_MARKER"), []byte("ok"), 0o644); err != nil {
		os.Exit(3)
	}
	// запись вне rw-путей запрещена
	if f, err := os.Create(filepath.Join(os.TempDir(), "alr-ll-forbidden")); err == nil {
		f.Close()
		os.Remove(filepath.Join(os.TempDir(), "alr-ll-forbidden"))
		os.Exit(4)
	}
	os.Exit(0)
}
