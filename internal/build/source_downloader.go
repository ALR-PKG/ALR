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

	"git.alr-pkg.ru/ALR/ALR/internal/constants"
	"git.alr-pkg.ru/ALR/ALR/pkg/dl"
	"git.alr-pkg.ru/ALR/ALR/pkg/dlcache"
)

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

		// Используем временную директорию для загрузок
		// dlcache.New добавит свой подкаталог "dl" внутри
		opts.DlCache = dlcache.New(constants.TempDir)

		err := dl.Download(ctx, opts)
		if err != nil {
			return err
		}
	}

	return nil
}
