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
	"path/filepath"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

type testPathsConfig struct {
	dbPath string
}

func (c *testPathsConfig) GetPaths() *config.Paths {
	return &config.Paths{
		DBPath: c.dbPath,
	}
}

func TestBusyTimeout(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "alr.db")

	database := New(&testPathsConfig{dbPath: dbPath})
	if err := database.Connect(); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer database.Close()

	var timeout int
	if err := database.engine.DB().QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if timeout != 5000 {
		t.Errorf("Expected busy_timeout 5000, got %d", timeout)
	}
}
