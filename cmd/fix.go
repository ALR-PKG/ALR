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

package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
	"git.alr-pkg.ru/ALR/ALR/internal/utils"
)

func execWithPrivileges(name string, args ...string) *exec.Cmd {
	isRoot := os.Geteuid() == 0
	isCI := os.Getenv("CI") == "true"

	if !isRoot && !isCI {
		allArgs := append([]string{name}, args...)
		return exec.Command("sudo", allArgs...)
	}
	return exec.Command(name, args...)
}

type fixActionKind int

const (
	// fixEnsureCacheDir guarantees that the shared cache dir exists with
	// the right ownership before any deletion (fail-fast, root only).
	fixEnsureCacheDir fixActionKind = iota
	// fixClearCacheDir removes all entries of the shared cache dir (root only).
	fixClearCacheDir
	// fixResetDir removes and recreates a directory owned by the current user.
	fixResetDir
	// fixPruneOwnEntries removes only entries owned by the current uid,
	// for per-user dirs that fall back inside the shared cache dir.
	fixPruneOwnEntries
)

type fixAction struct {
	kind fixActionKind
	path string
}

// planFixCleanup decides what `alr fix` is allowed to clean.
//
// Root cleans everything: the guarantee runs first (fail-fast — a failed
// guarantee returns before a single deletion), then the whole CacheDir and
// both working dirs. Non-root never touches the shared CacheDir, so it
// needs no guarantee at all: it only operates on paths owned by the current
// user, and removing one's own data is always possible. This eliminates the
// historical partial failure where a non-root user deleted shared entries
// and then failed to recreate the cache dir.
func planFixCleanup(paths *config.Paths, isRoot bool) []fixAction {
	var plan []fixAction
	if isRoot {
		plan = append(plan,
			fixAction{kind: fixEnsureCacheDir, path: paths.CacheDir},
			fixAction{kind: fixClearCacheDir, path: paths.CacheDir},
		)
	}
	for _, dir := range []string{paths.BuildDir, paths.PkgsDir} {
		if !isRoot && strings.HasPrefix(dir, paths.CacheDir+"/") {
			plan = append(plan, fixAction{kind: fixPruneOwnEntries, path: dir})
		} else {
			plan = append(plan, fixAction{kind: fixResetDir, path: dir})
		}
	}
	return plan
}

func newFixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix",
		Short: gotext.Get("Attempt to fix problems with ALR"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			cfg := deps.Cfg
			paths := cfg.GetPaths()
			isRoot := os.Geteuid() == 0

			slog.Info(gotext.Get("Clearing cache and temporary directories"))

			for _, action := range planFixCleanup(paths, isRoot) {
				// Активная публикация в кэш пакетов (.alr-cache.lock) —
				// пропускаем PkgsDir целиком
				if action.path == paths.PkgsDir {
					cacheLock := filepath.Join(paths.PkgsDir, ".alr-cache.lock")
					if ok, err := pathLocked(cacheLock); err != nil {
						slog.Debug("Failed to check package cache lock", "path", cacheLock, "error", err)
					} else if ok {
						slog.Warn(gotext.Get("Skipping %s: locked by another process", action.path))
						continue
					}
				}
				switch action.kind {
				case fixEnsureCacheDir:
					err = utils.EnsureTempDirWithRootOwner(action.path, 0o2775)
					if err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("Unable to create new cache directory"), err)
					}
				case fixClearCacheDir:
					if err := clearCacheDir(action.path); err != nil {
						return err
					}
				case fixResetDir:
					if err := resetDir(action.path); err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("Unable to reset directory %s", action.path), err)
					}
				case fixPruneOwnEntries:
					if err := pruneOwnEntries(action.path, uint32(os.Geteuid()), ownerUID); err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("Unable to clean directory %s", action.path), err)
					}
				}
			}

			slog.Info(gotext.Get("Rebuilding cache"))

			deps, err = appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithReposForcePull().
				Build()
			if err != nil {
				if isRoot {
					return err
				}
				slog.Warn(gotext.Get("Database rebuild skipped: %s", err.Error()))
			} else {
				defer deps.Defer()
			}

			slog.Info(gotext.Get("Done"))
			return nil
		},
	}
}

func clearCacheDir(cacheDir string) error {
	dir, err := os.Open(cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info(gotext.Get("Cache directory does not exist, will create it"))
			return nil
		}
		return fmt.Errorf("%s: %w", gotext.Get("Unable to open cache directory"), err)
	}
	defer dir.Close()

	entries, err := dir.Readdirnames(-1)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Unable to read cache directory contents"), err)
	}

	locked, err := lockedPaths(cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if locked[entry] || locked[entry+".lock"] {
			slog.Warn(gotext.Get("Skipping %s: locked by another process", entry))
			continue
		}
		fullPath := filepath.Join(cacheDir, entry)

		if info, statErr := os.Stat(fullPath); statErr == nil && info.IsDir() {
			if has, lockErr := hasActiveLockInside(fullPath); lockErr != nil {
				slog.Debug("Failed to check nested locks", "path", fullPath, "error", lockErr)
			} else if has {
				slog.Warn(gotext.Get("Skipping %s: locked by another process", entry))
				continue
			}
		}

		if err := makeWritableRecursive(fullPath); err != nil {
			slog.Debug("Failed to make path writable", "path", fullPath, "error", err)
		}

		err = os.RemoveAll(fullPath)
		if err != nil {
			slog.Warn(gotext.Get("Unable to remove cache item (%s) as current user, trying with sudo", entry))
			sudoCmd := execWithPrivileges("rm", "-rf", fullPath)
			if sudoErr := sudoCmd.Run(); sudoErr != nil {
				slog.Error(gotext.Get("Unable to remove cache item (%s)", entry), "error", err)
				continue
			}
		}
	}
	return nil
}

// resetDir очищает содержимое dirPath, сохраняя записи с активными
// блокировками (<entry>.lock держит другой процесс): удаление .lock-файла
// не снимает flock (он живёт на inode), а новый процесс создал бы новый
// файл и сломал взаимное исключение. Сам каталог гарантированно существует.
func resetDir(dirPath string) error {
	locked, err := lockedPaths(dirPath)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dirPath, 0o755)
		}
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if locked[name] || locked[name+".lock"] {
			slog.Warn(gotext.Get("Skipping %s: locked by another process", name))
			continue
		}
		if entry.IsDir() {
			fullPath := filepath.Join(dirPath, name)
			if has, lockErr := hasActiveLockInside(fullPath); lockErr != nil {
				slog.Debug("Failed to check nested locks", "path", fullPath, "error", lockErr)
			} else if has {
				slog.Warn(gotext.Get("Skipping %s: locked by another process", name))
				continue
			}
		}
		if err := makeTreeWritable(filepath.Join(dirPath, name)); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(dirPath, name)); err != nil {
			return err
		}
	}
	return nil
}

// makeTreeWritable выставляет владельцу право записи на всё дерево:
// сборки оставляют read-only файлы (кэш модулей go — 0444/0555),
// иначе очистка каталога упирается в permission denied.
// Симлинки пропускаются: chmod идёт за ссылкой и упрётся в чужой
// системный файл (package() линкует /usr/...), а удалению симлинка
// права его цели не нужны.
func makeTreeWritable(root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // недоступное уже не удалится — даст о себе знать RemoveAll
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			return os.Chmod(path, info.Mode()|0o700)
		}
		return os.Chmod(path, info.Mode()|0o600)
	})
}

// lockedPaths возвращает имена .lock-файлов (уровень 1 каталога dir),
// блокировку которых держит другой процесс. Свободные (stale) .lock-файлы
// в результат не попадают — их можно удалять.
func lockedPaths(dir string) (map[string]bool, error) {
	locks, err := filepath.Glob(filepath.Join(dir, "*.lock"))
	if err != nil {
		return nil, err
	}

	locked := map[string]bool{}
	for _, lockPath := range locks {
		isLocked, err := pathLocked(lockPath)
		if err != nil {
			slog.Debug("Failed to check lock", "path", lockPath, "error", err)
			continue
		}
		if isLocked {
			locked[filepath.Base(lockPath)] = true
		}
	}
	return locked, nil
}

// pathLocked сообщает, держит ли другой процесс flock на path. Свободный
// лок захватывается и тут же отпускается. Отсутствующий файл считается
// свободным и НЕ создаётся (TryLock иначе оставил бы файл-призрак).
func pathLocked(path string) (bool, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	}
	h, err := fsutils.TryLock(path)
	if err == nil {
		h.Close()
		return false, nil
	}
	if errors.Is(err, fsutils.ErrLocked) {
		return true, nil
	}
	return false, err
}

// hasActiveLockInside сообщает, есть ли внутри каталога dir (уровень 1)
// .lock-файл, который держит другой процесс. Используется для entry-
// директорий: локи могут лежать уровнем глубже чистимого каталога
// (например, CacheDir/repo/<repo>.lock).
func hasActiveLockInside(dir string) (bool, error) {
	locked, err := lockedPaths(dir)
	if err != nil {
		return false, err
	}
	return len(locked) > 0, nil
}

// ownerUID returns the uid of the file owner.
func ownerUID(path string) (uint32, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return 0, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("unable to determine owner of %s", path)
	}
	return st.Uid, nil
}

// pruneOwnEntries removes only the entries of dir owned by uid, skipping
// everything else. The directory itself is kept.
func pruneOwnEntries(dir string, uid uint32, uidOf func(string) (uint32, error)) error {
	locked, err := lockedPaths(dir)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if locked[name] || locked[name+".lock"] {
			slog.Warn(gotext.Get("Skipping %s: locked by another process", name))
			continue
		}
		fullPath := filepath.Join(dir, name)
		if entry.IsDir() {
			if has, lockErr := hasActiveLockInside(fullPath); lockErr != nil {
				slog.Debug("Failed to check nested locks", "path", fullPath, "error", lockErr)
			} else if has {
				slog.Warn(gotext.Get("Skipping %s: locked by another process", name))
				continue
			}
		}
		entryUID, err := uidOf(fullPath)
		if err != nil {
			slog.Debug("Failed to determine entry owner", "path", fullPath, "error", err)
			continue
		}
		if entryUID != uid {
			slog.Debug("Skipping entry owned by another user", "path", fullPath, "uid", entryUID)
			continue
		}
		if err := os.RemoveAll(fullPath); err != nil {
			slog.Warn(gotext.Get("Unable to remove cache item (%s)", entry.Name()), "error", err)
		}
	}
	return nil
}

func makeWritableRecursive(path string) error {
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		newMode := info.Mode() | 0o200
		if d.IsDir() {
			newMode |= 0o100
		}

		return os.Chmod(path, newMode)
	})
}
