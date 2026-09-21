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

package cmd

import (
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/ALR/ALR/internal/constants"
	"git.alr-pkg.ru/ALR/ALR/internal/manager"
)

var (
	pmArgs      string
	interactive bool
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "alr",
		Short: "Any Linux Repository",
		Long:  "Any Linux Repository — менеджер пакетов для любого Linux дистрибутива\nСайт: " + constants.DefaultSiteURL,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if trimmed := strings.TrimSpace(pmArgs); trimmed != "" {
				parts := strings.Split(trimmed, " ")
				manager.Args = append(manager.Args, parts...)
			}
			return nil
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVarP(&pmArgs, "pm-args", "P", "", gotext.Get("Arguments to be passed on to the package manager"))
	root.PersistentFlags().BoolVarP(&interactive, "interactive", "i", isatty.IsTerminal(0), gotext.Get("Enable interactive questions and prompts"))

	root.AddCommand(
		newVersionCmd(),
		newBuildCmd(),
		newInstallCmd(),
		newRemoveCmd(),
		newUpgradeCmd(),
		newInfoCmd(),
		newListCmd(),
		newRefreshCmd(),
		newFixCmd(),
		newGenCmd(),
		newHelperCmd(),
		newSearchCmd(),
		newRepoCmd(),
		newLegacyAddRepoCmd(),
		newLegacyRemoveRepoCmd(),
		newDBusCmd(),
		newConfigCmd(),
		newInternalBuildCmd(),
		newInternalSandboxCmd(),
		newInternalInstallCmd(),
		newInternalReposCmd(),
		newInternalDBusServiceCmd(),
	)

	return root
}
