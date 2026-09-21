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
	"os"

	"github.com/spf13/cobra"

	"git.alr-pkg.ru/ALR/ALR/internal/sandbox"
)

func newInternalSandboxCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_internal-sandbox-init",
		Hidden: true,
		// Все аргументы после имени команды (включая "--") принадлежат
		// целевой команде, а не cobra.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// При успехе RunInit завершается exec'ом целевой команды;
			// возвращаемый код — только сбой настройки песочницы.
			os.Exit(sandbox.RunInit(args))
			return nil
		},
	}
}

func newInternalSandboxLLCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_internal-sandbox-ll",
		Hidden: true,
		// Все аргументы после имени команды (включая "--") принадлежат
		// целевой команде, а не cobra.
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// При успехе RunLandlockInit завершается exec'ом целевой
			// команды; возвращаемый код — только сбой настройки.
			os.Exit(sandbox.RunLandlockInit(args))
			return nil
		},
	}
}
