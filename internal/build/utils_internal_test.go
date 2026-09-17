// ALR - Any Linux Repository
// Copyright (C) 2025 The ALR Authors
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
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

func TestRemoveDuplicatesSources(t *testing.T) {
	type testCase struct {
		Name         string
		Sources      []string
		Checksums    []string
		NewSources   []string
		NewChecksums []string
	}

	for _, tc := range []testCase{{
		Name:         "prefer non-skip values",
		Sources:      []string{"a", "b", "c", "a"},
		Checksums:    []string{"skip", "skip", "skip", "1"},
		NewSources:   []string{"a", "b", "c"},
		NewChecksums: []string{"1", "skip", "skip"},
	}} {
		t.Run(tc.Name, func(t *testing.T) {
			s, c := removeDuplicatesSources(tc.Sources, tc.Checksums)
			assert.Equal(t, s, tc.NewSources)
			assert.Equal(t, c, tc.NewChecksums)
		})
	}
}

func TestCopyToCache(t *testing.T) {
	t.Run("copies file to pkgs dir and removes source", func(t *testing.T) {
		pkgsDir := filepath.Join(t.TempDir(), "pkgs")
		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("package-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		dstPath, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst"), dstPath)

		data, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("package-data"), data)

		fi, err := os.Stat(dstPath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm())

		_, err = os.Stat(srcPath)
		assert.True(t, os.IsNotExist(err), "источник должен быть удалён из дерева сборки")
	})

	t.Run("replaces existing file in cache", func(t *testing.T) {
		pkgsDir := t.TempDir()
		dstPath := filepath.Join(pkgsDir, "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(dstPath, []byte("old-data"), 0o644))

		srcPath := filepath.Join(t.TempDir(), "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, os.WriteFile(srcPath, []byte("new-data"), 0o644))

		cfg := &fakeResolverConfig{
			paths: &config.Paths{PkgsDir: pkgsDir},
		}

		gotPath, err := copyToCache(cfg, srcPath, "pkg-1.0-1-x86_64.pkg.tar.zst")
		require.NoError(t, err)
		assert.Equal(t, dstPath, gotPath)

		data, err := os.ReadFile(dstPath)
		require.NoError(t, err)
		assert.Equal(t, []byte("new-data"), data)
	})
}
