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

package osutils_test

import (
	"os"
	"path/filepath"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/osutils"
)

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.rpm")
	dstPath := filepath.Join(dir, "dst.rpm")

	content := []byte("e4 test package content")
	if err := os.WriteFile(srcPath, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := osutils.Copy(srcPath, dstPath); err != nil {
		t.Fatalf("Copy failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("cannot read destination: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("destination content = %q, want %q", got, content)
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("source must remain in place: %v", err)
	}
	dstInfo, err := os.Stat(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if srcInfo.Mode() != dstInfo.Mode() {
		t.Errorf("destination mode = %v, want %v", dstInfo.Mode(), srcInfo.Mode())
	}
}

func TestCopyMissingSource(t *testing.T) {
	dir := t.TempDir()
	err := osutils.Copy(filepath.Join(dir, "nope"), filepath.Join(dir, "dst"))
	if err == nil {
		t.Error("expected error for missing source, got nil")
	}
}
