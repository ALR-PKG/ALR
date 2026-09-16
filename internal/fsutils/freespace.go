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

	"golang.org/x/sys/unix"
)

// CheckFreeSpace проверяет, что на ФС каталога dir свободно не менее
// required байт (для непривилегированного пользователя)
func CheckFreeSpace(dir string, required uint64) error {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return fmt.Errorf("не удалось проверить свободное место на %s: %w", dir, err)
	}
	free := st.Bavail * uint64(st.Bsize)
	if free < required {
		return fmt.Errorf("недостаточно места на %s: свободно %d МБ, требуется %d МБ",
			dir, free/(1<<20), required/(1<<20))
	}
	return nil
}
