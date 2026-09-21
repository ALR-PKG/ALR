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
	"log/slog"
	"os"
	"path/filepath"
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

// Подменяемые в тестах зависимости выбора корня кэша скачиваний
var (
	geteuid              = os.Geteuid
	userCacheDir         = os.UserCacheDir
	currentUserGroupGIDs = fsutils.CurrentUserGroupGIDs
	userInGroups         = fsutils.UserInGroups
	ensureSharedDir      = fsutils.EnsureTempDirWithRootOwner
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
	// Корень кэша выбирается и место проверяется только если есть что
	// скачивать: без скачиваний dlcache не трогает ФС (каталог dl создаётся
	// на первое скачивание), а корень кэша может ещё не существовать.
	cacheDir := s.cfg.GetPaths().CacheDir
	if len(si.Sources) > 0 {
		dlRoot, err := resolveDlCacheRoot(s.cfg)
		if err != nil {
			return err
		}
		slog.Debug("download cache root", "path", dlRoot)
		if err := fsutils.CheckFreeSpace(dlRoot, minDownloadSpace); err != nil {
			return err
		}
		cacheDir = dlRoot
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

		// Кэш загрузок: общий CacheDir или per-user фолбэк для
		// пользователей без групп; dlcache.New добавит подкаталог "dl"
		opts.DlCache = dlcache.New(cacheDir)

		err := dl.Download(ctx, opts)
		if err != nil {
			return err
		}
	}

	return nil
}

// resolveDlCacheRoot выбирает корень кэша скачиваний для текущего
// пользователя (dlcache сам добавит подкаталог "dl"):
//  1. root — общий CacheDir;
//  2. член группы "alr" или привилегированной группы дистрибутива — общий
//     CacheDir, предварительно гарантируется CacheDir/dl (fast-path внутри
//     EnsureTempDirWithRootOwner обходится без sudo); ошибка — фолбэк
//     per-user;
//  3. остальные — сразу per-user (без sudo: прав у пользователя всё равно
//     нет).
func resolveDlCacheRoot(cfg Config) (string, error) {
	shared := cfg.GetPaths().CacheDir

	if geteuid() == 0 {
		return shared, nil
	}

	gids, err := currentUserGroupGIDs()
	if err != nil {
		// Трактуем как отсутствие членства — уйдём в per-user без sudo
		slog.Debug("cannot list user groups, using per-user download cache", "error", err)
		gids = nil
	}

	if !userInGroups(gids, "alr", fsutils.GetPrivilegedGroup()) {
		return perUserDlCacheRoot()
	}

	if err := ensureSharedDir(filepath.Join(shared, "dl"), 0o2775); err != nil {
		slog.Warn("cannot prepare shared download cache, falling back to per-user", "path", shared, "error", err)
		root, perr := perUserDlCacheRoot()
		if perr != nil {
			// Исходная ошибка общего пути информативнее
			return "", err
		}
		return root, nil
	}
	return shared, nil
}

// perUserDlCacheRoot создаёт и возвращает per-user корень кэша скачиваний
// (<UserCacheDir>/alr, dlcache сам добавит "dl").
func perUserDlCacheRoot() (string, error) {
	ucd, err := userCacheDir()
	if err != nil {
		return "", err
	}
	root := filepath.Join(ucd, "alr")
	if err := os.MkdirAll(filepath.Join(root, "dl"), 0o755); err != nil {
		return "", err
	}
	return root, nil
}
