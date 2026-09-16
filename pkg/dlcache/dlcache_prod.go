//go:build !test

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

package dlcache

import (
	"os"
	"strings"

	"git.alr-pkg.ru/ALR/ALR/internal/utils"
)

// createDir создает директорию с правильными правами для production
func createDir(itemPath string, mode os.FileMode) error {
	// Используем специальную функцию для создания каталогов с setgid битом только для /var/cache/alr/
	// Проверяем с слешем в конце, чтобы исключить похожие тестовые директории
	if strings.HasPrefix(itemPath, "/var/cache/alr/") {
		return utils.EnsureTempDirWithRootOwner(itemPath, mode)
	} else {
		return os.MkdirAll(itemPath, mode)
	}
}
