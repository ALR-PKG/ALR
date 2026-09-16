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

package fsutils

import (
	"context"
	"os/user"
	"sync"

	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
)

var (
	privilegedGroupCache string
	privilegedGroupOnce  sync.Once
)

// GetPrivilegedGroup определяет правильную привилегированную группу для текущего дистрибутива.
// Дистрибутивы на базе Debian/Ubuntu используют группу "sudo", остальные - "wheel".
func GetPrivilegedGroup() string {
	privilegedGroupOnce.Do(func() {
		privilegedGroupCache = detectPrivilegedGroup()
	})
	return privilegedGroupCache
}

func detectPrivilegedGroup() string {
	// Попробуем определить дистрибутив
	ctx := context.Background()
	osInfo, err := distro.ParseOSRelease(ctx)
	if err != nil {
		// Если не можем определить дистрибутив, проверяем какие группы существуют
		return detectGroupByAvailability()
	}

	// Проверяем ID и семейство дистрибутива
	// Debian и его производные (Ubuntu, Mint, PopOS и т.д.) используют sudo
	if osInfo.ID == "debian" || osInfo.ID == "ubuntu" {
		return "sudo"
	}

	// Проверяем семейство дистрибутива через ID_LIKE
	for _, like := range osInfo.Like {
		if like == "debian" || like == "ubuntu" {
			return "sudo"
		}
	}

	// Для остальных дистрибутивов (Fedora, RHEL, Arch, openSUSE, ALT Linux) используется wheel
	return "wheel"
}

// detectGroupByAvailability проверяет существование групп в системе
func detectGroupByAvailability() string {
	// Сначала проверяем группу sudo (более распространена)
	if _, err := user.LookupGroup("sudo"); err == nil {
		return "sudo"
	}

	// Если sudo не найдена, возвращаем wheel
	return "wheel"
}
