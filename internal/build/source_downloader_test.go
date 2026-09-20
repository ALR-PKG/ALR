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

package build

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

// Контракт проверки свободного места: CheckFreeSpace для CacheDir
// выполняется только если есть что скачивать. Без скачиваний dlcache
// вообще не трогает ФС (каталог dl создаётся на первое скачивание),
// а CacheDir может ещё не существовать — требовать место не у чего.

func TestDownloadSources_NoSpaceCheckWithoutSources(t *testing.T) {
	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: filepath.Join(t.TempDir(), "missing")},
	}
	dl := NewSourceDownloader(cfg)

	err := dl.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{})
	require.NoError(t, err)
}

func TestDownloadSources_ChecksSpaceBeforeDownloading(t *testing.T) {
	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: filepath.Join(t.TempDir(), "missing")},
	}
	d := NewSourceDownloader(cfg)

	// Несуществующий CacheDir: Statfs внутри CheckFreeSpace падает раньше,
	// чем начнётся скачивание мусорного URL — ошибка именно про место.
	err := d.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{
		Sources:   []string{"not-a-url"},
		Checksums: []string{"SKIP"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось проверить свободное место")
}

func TestDownloadSources_EnoughSpacePassesCheck(t *testing.T) {
	cfg := &fakeResolverConfig{
		paths: &config.Paths{CacheDir: t.TempDir()},
	}
	d := NewSourceDownloader(cfg)

	// Места достаточно — проверка проходит, ошибка дальше уже от попытки
	// скачать мусорный URL, а не от CheckFreeSpace.
	err := d.DownloadSources(context.Background(), &BuildInput{}, "pkg", SourcesInput{
		Sources:   []string{"not-a-url"},
		Checksums: []string{"SKIP"},
	})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "свободное место")
}
