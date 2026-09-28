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

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Файл БД создаётся лениво в Sync2 с правами по umask (644) — права
// должны нормализоваться и в этом случае, а не только при Connect
func TestInitNormalizesFreshSharedDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alr.db")

	old := systemDBPath
	systemDBPath = dbPath
	t.Cleanup(func() { systemDBPath = old })

	database := New(&testPathsConfig{dbPath: dbPath})
	if err := database.Init(context.Background()); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer database.Close()

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("Expected database file to exist, got %s", err)
	}
	if info.Mode().Perm() != 0o664 {
		t.Errorf("Expected mode 664, got %o", info.Mode().Perm())
	}
}
