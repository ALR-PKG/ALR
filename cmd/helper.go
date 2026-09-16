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
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"

	"git.alr-pkg.ru/ALR/ALR/internal/cpu"
	"git.alr-pkg.ru/ALR/ALR/internal/shutils/helpers"
	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
)

func newHelperCmd() *cobra.Command {
	var destDir string

	cmd := &cobra.Command{
		Use:   "helper",
		Short: gotext.Get("Run a ALR helper command"),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			helper, ok := helpers.Helpers[args[0]]
			if !ok {
				slog.Error(gotext.Get("No such helper command"), "name", args[0])
				return fmt.Errorf("%s", gotext.Get("No such helper command"))
			}

			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error getting working directory"), err)
			}

			info, err := distro.ParseOSRelease(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error parsing os-release file"), err)
			}

			hc := interp.HandlerContext{
				Env: expand.ListEnviron(
					"pkgdir="+destDir,
					"DISTRO_ID="+info.ID,
					"DISTRO_ID_LIKE="+strings.Join(info.Like, " "),
					"ARCH="+cpu.Arch(),
				),
				Dir:    wd,
				Stdin:  os.Stdin,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			}

			return helper(hc, args[0], args[1:])
		},
		ValidArgsFunction: helperCompletion,
	}

	cmd.Flags().StringVarP(&destDir, "dest-dir", "d", "dest", gotext.Get("The directory that the install commands will install to"))

	cmd.AddCommand(
		newHelperListCmd(),
	)

	addFlagCompletion(cmd)
	return cmd
}

func newHelperListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   gotext.Get("List all the available helper commands"),
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			for name := range helpers.Helpers {
				fmt.Println(name)
			}
			return nil
		},
	}
}

func helperCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	names := make([]string, 0, len(helpers.Helpers))
	for name := range helpers.Helpers {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []string
	for _, name := range names {
		if toComplete == "" || strings.HasPrefix(name, toComplete) {
			out = append(out, name)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
