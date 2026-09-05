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

package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// addFlagCompletion добавляет автодополнение флагов команды.
// При нажатии Tab без "-" показывает доступные флаги вместо файлов.
// Если у команды есть собственная ValidArgsFunction, её варианты
// (например, имена пакетов) добавляются к списку флагов.
func addFlagCompletion(cmd *cobra.Command) {
	origFn := cmd.ValidArgsFunction
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		// Если toComplete пустой — показываем флаги
		if toComplete == "" {
			seen := make(map[string]struct{})
			var flags []string
			addFlags := func(f *pflag.Flag) {
				if f.Hidden {
					return
				}
				long := "--" + f.Name
				if _, ok := seen[long]; !ok {
					seen[long] = struct{}{}
					flags = append(flags, long)
				}
				if f.Shorthand != "" {
					short := "-" + f.Shorthand
					if _, ok := seen[short]; !ok {
						seen[short] = struct{}{}
						flags = append(flags, short)
					}
				}
			}
			cmd.Flags().VisitAll(addFlags)
			if parent := cmd.Parent(); parent != nil {
				parent.Flags().VisitAll(addFlags)
			}
			if origFn != nil {
				items, _ := origFn(cmd, args, toComplete)
				flags = append(flags, items...)
			}
			return flags, cobra.ShellCompDirectiveNoFileComp
		}
		// Если toComplete начинается с "-" — cobra сама обработает флаги
		if strings.HasPrefix(toComplete, "-") {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		// Иначе — делегируем оригинальной ValidArgsFunction (если есть)
		if origFn != nil {
			return origFn(cmd, args, toComplete)
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}
