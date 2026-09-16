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

package fsutils

import (
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

// EnsureGroupAccess рекурсивно выставляет на дереве root привилегированную
// группу и права g+rwX (на директории также setgid), чтобы члены группы
// могли читать и обновлять общий кэш (/var/cache/alr). Git при fetch
// создаёт pack-файлы с правами 0600, и без нормализации пользователи
// из группы не могут работать с репозиторным кэшем.
//
// Имеет смысл только при запуске от root; для остальных — no-op.
func EnsureGroupAccess(root string) error {
	if os.Geteuid() != 0 {
		return nil
	}

	grp, err := user.LookupGroup(GetPrivilegedGroup())
	if err != nil {
		return fmt.Errorf("не удалось найти привилегированную группу: %w", err)
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return fmt.Errorf("некорректный gid группы %s: %w", grp.Name, err)
	}

	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		if err := os.Chown(path, 0, gid); err != nil {
			return fmt.Errorf("chown %s: %w", path, err)
		}

		mode := info.Mode()
		if d.IsDir() {
			mode |= 0o770 | os.ModeSetgid
		} else {
			mode |= 0o660
		}
		if mode != info.Mode() {
			if err := os.Chmod(path, mode); err != nil {
				return fmt.Errorf("chmod %s: %w", path, err)
			}
		}

		return nil
	})
}
