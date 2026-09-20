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
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
	"git.alr-pkg.ru/ALR/ALR/pkg/dl"
	"git.alr-pkg.ru/ALR/ALR/pkg/dlcache"
)

// minDownloadSpace — минимум свободного места в CacheDir перед скачиванием
// исходников — 1 ГиБ. dlcache — отдельная ФС от дерева сборки (оно проверяется
// minBuildSpace в prepareDirs); точный размер скачиваний неизвестен заранее,
// это страховка от ENOSPC посреди скачивания.
const minDownloadSpace = 1 << 30

type SourceDownloader struct {
	cfg Config
}

func NewSourceDownloader(cfg Config) *SourceDownloader {
	return &SourceDownloader{
		cfg,
	}
}

func (s *SourceDownloader) DownloadSources(
	ctx context.Context,
	input *BuildInput,
	basePkg string,
	si SourcesInput,
) error {
	// Проверяем место только если есть что скачивать: без скачиваний dlcache
	// не трогает ФС (каталог dl создаётся на первое скачивание), а CacheDir
	// может ещё не существовать.
	if len(si.Sources) > 0 {
		if err := fsutils.CheckFreeSpace(s.cfg.GetPaths().CacheDir, minDownloadSpace); err != nil {
			return err
		}
	}

	for i, src := range si.Sources {

		opts := dl.Options{
			Name:        fmt.Sprintf("[%d]", i),
			URL:         src,
			Destination: getSrcDir(s.cfg, basePkg),
			Progress:    os.Stderr,
			LocalDir:    getScriptDir(input.script),
		}

		if !strings.EqualFold(si.Checksums[i], "SKIP") {
			// Если контрольная сумма содержит двоеточие, используйте часть до двоеточия
			// как алгоритм, а часть после как фактическую контрольную сумму.
			// В противном случае используйте sha256 по умолчанию с целой строкой как контрольной суммой.
			algo, hashData, ok := strings.Cut(si.Checksums[i], ":")
			if ok {
				checksum, err := hex.DecodeString(hashData)
				if err != nil {
					return err
				}
				opts.Hash = checksum
				opts.HashAlgorithm = algo
			} else {
				checksum, err := hex.DecodeString(si.Checksums[i])
				if err != nil {
					return err
				}
				opts.Hash = checksum
			}
		}

		// Используем системный кэш для загрузок
		// dlcache.New добавит свой подкаталог "dl" внутри
		opts.DlCache = dlcache.New(s.cfg.GetPaths().CacheDir)

		err := dl.Download(ctx, opts)
		if err != nil {
			return err
		}
	}

	return nil
}
