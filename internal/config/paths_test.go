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

package config_test

import (
	"path/filepath"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

func TestLoadSetsPerUserBuildDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)

	cfg := config.New()
	if err := cfg.Load(); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}

	want := filepath.Join(xdg, "alr", "build")
	if cfg.GetPaths().PkgsDir != want {
		t.Errorf("Expected PkgsDir %q, got %q", want, cfg.GetPaths().PkgsDir)
	}
}

func TestLoadKeepsSystemPaths(t *testing.T) {
	cfg := config.New()
	if err := cfg.Load(); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if cfg.GetPaths().CacheDir != "/var/cache/alr" {
		t.Errorf("Expected CacheDir /var/cache/alr, got %q", cfg.GetPaths().CacheDir)
	}
	if cfg.GetPaths().RepoDir != "/var/cache/alr/repo" {
		t.Errorf("Expected RepoDir /var/cache/alr/repo, got %q", cfg.GetPaths().RepoDir)
	}
}
