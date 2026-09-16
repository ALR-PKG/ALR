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
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// EnsureTempDirWithRootOwner создает каталог в /var/cache/alr с правами для привилегированной группы
// Все каталоги в /var/cache/alr принадлежат root:привилегированная_группа с правами 2775
// Для других каталогов использует стандартные права
func EnsureTempDirWithRootOwner(path string, mode os.FileMode) error {
	needsElevation := strings.HasPrefix(path, "/var/cache/alr")

	if needsElevation {
		// В CI или если мы уже root, не нужно использовать sudo
		isRoot := os.Geteuid() == 0
		isCI := os.Getenv("CI") == "true"

		// В CI создаем директории с обычными правами
		if isCI {
			err := os.MkdirAll(path, mode)
			if err != nil {
				return err
			}
			// В CI не используем группу wheel и не меняем права
			// Устанавливаем базовые права 777 для временных каталогов
			chmodCmd := exec.Command("chmod", "777", path)
			chmodCmd.Run() // Игнорируем ошибки
			return nil
		}

		// Для обычной работы устанавливаем права и привилегированную группу
		permissions := "2775"
		group := GetPrivilegedGroup()

		var mkdirCmd, chmodCmd, chownCmd *exec.Cmd
		if isRoot {
			// Выполняем команды напрямую без sudo
			mkdirCmd = exec.Command("mkdir", "-p", path)
			chmodCmd = exec.Command("chmod", permissions, path)
			chownCmd = exec.Command("chown", "root:"+group, path)
		} else {
			// Используем sudo для всех операций с привилегированными каталогами
			mkdirCmd = exec.Command("sudo", "mkdir", "-p", path)
			chmodCmd = exec.Command("sudo", "chmod", permissions, path)
			chownCmd = exec.Command("sudo", "chown", "root:"+group, path)
		}

		// Создаем директорию через sudo если нужно
		err := mkdirCmd.Run()
		if err != nil {
			// Игнорируем ошибку если директория уже существует
			if !isRoot {
				// Проверяем существует ли директория
				if _, statErr := os.Stat(path); statErr != nil {
					return fmt.Errorf("не удалось создать директорию %s: %w", path, err)
				}
			}
		}

		// Устанавливаем права с setgid битом для наследования группы
		err = chmodCmd.Run()
		if err != nil {
			if !isRoot {
				return fmt.Errorf("не удалось установить права на %s: %w", path, err)
			}
		}

		// Устанавливаем владельца root:группа
		err = chownCmd.Run()
		if err != nil {
			if !isRoot {
				return fmt.Errorf("не удалось установить владельца на %s: %w", path, err)
			}
		}

		return nil
	}

	// Для остальных каталогов обычное создание
	return os.MkdirAll(path, mode)
}
