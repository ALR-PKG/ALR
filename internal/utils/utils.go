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

package utils

import (
	"fmt"
	"os"
	"os/user"

	"golang.org/x/sys/unix"

	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
)

func NoNewPrivs() error {
	return unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0)
}

// EnsureTempDirWithRootOwner создает каталог в /var/cache/alr с правами для привилегированной группы
// Обёртка для обратной совместимости, делегирует вызов в fsutils
func EnsureTempDirWithRootOwner(path string, mode os.FileMode) error {
	return fsutils.EnsureTempDirWithRootOwner(path, mode)
}

// GetPrivilegedGroup возвращает привилегированную группу для текущей системы
// Обёртка для обратной совместимости, делегирует вызов в fsutils
func GetPrivilegedGroup() string {
	return fsutils.GetPrivilegedGroup()
}

// IsUserInGroup проверяет, состоит ли пользователь в указанной группе
func IsUserInGroup(username, groupname string) bool {
	u, err := user.Lookup(username)
	if err != nil {
		return false
	}

	groups, err := u.GroupIds()
	if err != nil {
		return false
	}

	targetGroup, err := user.LookupGroup(groupname)
	if err != nil {
		return false
	}

	for _, gid := range groups {
		if gid == targetGroup.Gid {
			return true
		}
	}
	return false
}

// CheckUserPrivileges проверяет, что пользователь имеет необходимые привилегии для работы с ALR
// Пользователь должен быть root или состоять в группе wheel/sudo
func CheckUserPrivileges() error {
	// Если пользователь root - все в порядке
	if os.Geteuid() == 0 {
		return nil
	}

	// В CI не проверяем привилегии
	if os.Getenv("CI") == "true" {
		return nil
	}

	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("не удалось получить информацию о текущем пользователе: %w", err)
	}

	privilegedGroup := fsutils.GetPrivilegedGroup()

	// Проверяем членство в привилегированной группе
	if !IsUserInGroup(currentUser.Username, privilegedGroup) {
		return fmt.Errorf("пользователь %s не имеет необходимых привилегий для работы с ALR.\n"+
			"Для работы с ALR необходимо быть пользователем root или состоять в группе %s.\n"+
			"Выполните команду: sudo usermod -a -G %s %s\n"+
			"Затем перезайдите в систему или выполните: newgrp %s",
			currentUser.Username, privilegedGroup, privilegedGroup, currentUser.Username, privilegedGroup)
	}

	return nil
}
