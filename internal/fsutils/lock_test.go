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
	"os"
	"path/filepath"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
)

func TestTryLockCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	f, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer f.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected lock file to exist, got %s", err)
	}
}

func TestTryLockAlreadyLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	f, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer f.Close()

	_, err = fsutils.TryLock(path)
	if !errors.Is(err, fsutils.ErrLocked) {
		t.Errorf("Expected ErrLocked, got %v", err)
	}
}

func TestTryLockAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	f, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Failed to close lock file: %s", err)
	}

	f2, err := fsutils.TryLock(path)
	if err != nil {
		t.Errorf("Expected no error after Close, got %s", err)
	} else {
		f2.Close()
	}
}

func TestTryLockCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "test.lock")
	f, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer f.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected lock file to exist, got %s", err)
	}
}
