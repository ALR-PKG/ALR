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

package fsutils_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"

	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
)

// flockHeld проверяет, удерживается ли flock на path каким-либо
// open file description (в т.ч. другим fd этого же процесса)
func flockHeld(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open %s: %s", path, err)
	}
	defer f.Close()
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		return false
	}
	return errors.Is(err, syscall.EWOULDBLOCK)
}

func TestTryLockCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected lock file to exist, got %s", err)
	}
}

func TestTryLockAlreadyLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()

	// В пределах процесса TryLock реентерабелен, поэтому ErrLocked
	// проверяем из отдельного процесса
	cmd := exec.Command(os.Args[0], "-test.run=TestTryLockHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_TRYLOCK_HELPER=1", "TRYLOCK_PATH="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("Expected ErrLocked in helper process, got: %s (%s)", out, err)
	}
}

// TestTryLockHelperProcess — не тест, а subprocess для TestTryLockAlreadyLocked
func TestTryLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TRYLOCK_HELPER") != "1" {
		return
	}
	_, err := fsutils.TryLock(os.Getenv("TRYLOCK_PATH"))
	if errors.Is(err, fsutils.ErrLocked) {
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "expected ErrLocked, got %v", err)
	os.Exit(1)
}

func TestTryLockAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Failed to close lock: %s", err)
	}
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after Close")
	}

	l2, err := fsutils.TryLock(path)
	if err != nil {
		t.Errorf("Expected no error after Close, got %s", err)
	} else {
		l2.Close()
	}
}

func TestTryLockReentrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l1, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}

	l2, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected reentrant TryLock to succeed, got %s", err)
	}

	if err := l1.Close(); err != nil {
		t.Fatalf("Failed to close first lock: %s", err)
	}
	if !flockHeld(t, path) {
		t.Error("Expected flock to still be held after first Close")
	}

	if err := l2.Close(); err != nil {
		t.Fatalf("Failed to close second lock: %s", err)
	}
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after last Close")
	}

	l3, err := fsutils.TryLock(path)
	if err != nil {
		t.Errorf("Expected no error after all locks closed, got %s", err)
	} else {
		l3.Close()
	}
}

func TestTryLockDoubleClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Failed to close lock: %s", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Expected double Close to be a no-op, got %s", err)
	}
}

func TestTryLockConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				l, err := fsutils.TryLock(path)
				if err != nil {
					t.Errorf("Expected no error, got %s", err)
					return
				}
				if err := l.Close(); err != nil {
					t.Errorf("Failed to close lock: %s", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after all goroutines finished")
	}
}

func TestTryLockCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected lock file to exist, got %s", err)
	}
}
