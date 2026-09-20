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
	"os"
	"os/user"
	"strconv"
	"sync"
	"syscall"

	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
)

// lookupGroup вынесено в переменную для подмены в тестах
var lookupGroup = user.LookupGroup

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

// CacheShareGroup возвращает целевую группу для новых общих каталогов
// (/var/cache/alr/*): "alr", если такая группа существует в системе,
// иначе привилегированную группу дистрибутива (GetPrivilegedGroup).
// Кэширования нет: LookupGroup дешёвый, а отсутствие sync.Once упрощает
// подмену lookupGroup в тестах.
func CacheShareGroup() string {
	if _, err := lookupGroup("alr"); err == nil {
		return "alr"
	}
	return GetPrivilegedGroup()
}

// CurrentUserGroupGIDs возвращает множество gid текущего пользователя:
// основную группу процесса и все дополнительные группы.
// При ошибке возвращает nil и ошибку; вызывающему следует трактовать это
// как пустое множество (fast-path не сработает, управление уйдёт в sudo-ветку).
func CurrentUserGroupGIDs() (map[uint32]bool, error) {
	cur, err := user.Current()
	if err != nil {
		return nil, err
	}
	list, err := cur.GroupIds()
	if err != nil {
		return nil, err
	}

	gids := make(map[uint32]bool, len(list)+1)
	// На Linux GroupIds (getgrouplist) обычно уже включает основную группу,
	// но это не гарантировано — добавляем её явно (map дедуплицирует).
	// Проверки членства ядро выполняет по effective gid.
	gids[uint32(os.Getegid())] = true
	for _, s := range list {
		gid, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			continue
		}
		gids[uint32(gid)] = true
	}
	return gids, nil
}

// dirSharedWith проверяет, что path — общий каталог, доступный текущему
// пользователю как члену группы: группа каталога входит в gids, выставлен
// setgid-бит (наследование группы) и у группы есть права rwx.
func dirSharedWith(path string, gids map[uint32]bool) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	mode := info.Mode()
	return gids[stat.Gid] &&
		mode&os.ModeSetgid != 0 &&
		mode.Perm()&0o070 == 0o070
}
