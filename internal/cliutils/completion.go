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

package cliutils

import (
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v2"
)

const (
	ansiReset = "\033[0m"
	ansiCyan  = "\033[36m"
	ansiBold  = "\033[1m"
)

// FormatCompletionItem форматирует элемент автодополнения.
// ANSI-цвета применяются только если stdout — TTY.
func FormatCompletionItem(name string, isAlr bool) string {
	if isAlr && isatty.IsTerminal(os.Stdout.Fd()) {
		return ansiCyan + ansiBold + name + ansiReset
	}
	return name
}

// PrintCompletionItem выводит отформатированный элемент автодополнения.
func PrintCompletionItem(name string, isAlr bool) {
	fmt.Println(FormatCompletionItem(name, isAlr))
}

// CompletionPrefix возвращает текущий префикс для автодополнения из аргументов контекста.
// Возвращает пустую строку, если пользователь вводит флаг (начинается с '-').
func CompletionPrefix(c *cli.Context) string {
	var prefix string
	if c.Args().Len() > 0 {
		prefix = c.Args().Get(c.Args().Len() - 1)
		if strings.HasPrefix(prefix, "-") {
			prefix = ""
		}
	}
	return prefix
}

// MaybePrintFlags выводит флаги команды, если пользователь вводит флаг (начинает с '-').
// Возвращает true, если флаги были выведены.
func MaybePrintFlags(c *cli.Context) bool {
	if c.Args().Len() > 0 && strings.HasPrefix(c.Args().Get(c.Args().Len()-1), "-") {
		PrintCompletionFlags(c.Command.Flags)
		return true
	}
	return false
}

// PrintCompletionFlags выводит флаги команды для автодополнения.
func PrintCompletionFlags(flags []cli.Flag) {
	for _, f := range flags {
		for _, name := range f.Names() {
			if len(name) > 1 {
				fmt.Println("--" + name)
			} else {
				fmt.Println("-" + name)
			}
		}
	}
}

// BuildPkgQuery строет условие WHERE и аргументы для запроса ALR-пакетов по префиксу.
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
