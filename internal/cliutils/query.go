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

package cliutils

import "strings"

// BuildPkgQuery строит условие WHERE и аргументы для запроса ALR-пакетов по префиксу.
// Если prefix пустой — возвращает "true" (все пакеты).
func BuildPkgQuery(prefix string) (where string, args []any) {
	if prefix != "" {
		return "name LIKE ? ESCAPE ?", []any{EscapeLikePattern(prefix) + "%", "\\"}
	}
	return "true", nil
}

// EscapeLikePattern экранирует спецсимволы SQL LIKE (_ и %) в строке.
func EscapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}
