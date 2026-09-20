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
	"path/filepath"
	"strings"
)

// systemCachePrefix вынесено в переменную для подмены в тестах
var systemCachePrefix = "/var/cache/alr"

// EnsureTempDirWithRootOwner создает каталог в /var/cache/alr с правами для целевой группы кэша
// Все каталоги в /var/cache/alr принадлежат root:CacheShareGroup() с правами 2775
// Для других каталогов использует стандартные права
func EnsureTempDirWithRootOwner(path string, mode os.FileMode) error {
	needsElevation := path == systemCachePrefix || strings.HasPrefix(path, systemCachePrefix+"/")

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

		// Для обычной работы устанавливаем права и целевую группу кэша
		permissions := "2775"
		group := CacheShareGroup()

		if !isRoot {
			// Fast-path: член группы-владельца с корректными правами
			// обходится без sudo
			if ensureSharedDirFast(path) {
				return nil
			}
		}

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

// ensureSharedDirFast пытается обеспечить общий каталог без эскалации
// привилегий — для члена группы-владельца каталога (или его setgid-предка).
// Возвращает true, если каталог готов; иначе false и вызывающему следует
// перейти к sudo-ветке.
func ensureSharedDirFast(path string) bool {
	gids, err := CurrentUserGroupGIDs()
	if err != nil {
		return false
	}

	_, statErr := os.Stat(path)
	switch {
	case statErr == nil:
		// Каталог существует: если он уже общий для нас, ничего делать не нужно
		return dirSharedWith(path, gids)
	case !os.IsNotExist(statErr):
		return false
	}

	// Каталога нет: создаём без sudo, если ближайший существующий предок
	// общий для нас (setgid — новые каталоги наследуют его группу)
	ancestor := nearestExistingDir(path)
	if ancestor == "" || !dirSharedWith(ancestor, gids) {
		return false
	}
	if err := os.MkdirAll(path, 0o2775); err != nil {
		return false
	}
	// MkdirAll теряет setgid и group-write из-за umask — восстанавливаем
	// права на всех созданных каталогах от path до предка
	for dir := path; dir != ancestor; {
		if err := os.Chmod(dir, os.ModeSetgid|0o0775); err != nil {
			return false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return dirSharedWith(path, gids)
}

// nearestExistingDir возвращает ближайший существующий каталог-предок
// path или пустую строку, если такого нет.
func nearestExistingDir(path string) string {
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return ""
		}
		info, err := os.Stat(parent)
		if err == nil {
			if info.IsDir() {
				return parent
			}
			return ""
		}
		path = parent
	}
}
