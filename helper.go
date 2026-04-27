// This file was originally part of the project "LURE - Linux User REpository", created by Elara Musayelyan.
// It has been modified as part of "ALR - Any Linux Repository" by the ALR Authors.
//
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

package main

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v2"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cpu"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/shutils/helpers"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
)

func HelperCmd() *cli.Command {
	helperListCmd := &cli.Command{
		Name:    "list",
		Usage:   gotext.Get("List all the available helper commands"),
		Aliases: []string{"ls"},
		Action: func(ctx *cli.Context) error {
			for name := range helpers.Helpers {
				fmt.Println(name)
			}
			return nil
		},
	}

	helperHelpCmd := &cli.Command{
		Name:      "help",
		Aliases:   []string{"h"},
		Usage:     gotext.Get("Shows a list of commands or help for one command"),
		ArgsUsage: "[command]",
		Action: func(cCtx *cli.Context) error {
			args := cCtx.Args()
			if args.Present() {
				return cli.ShowCommandHelp(cCtx, args.First())
			}
			cli.ShowSubcommandHelp(cCtx)
			return nil
		},
	}

	return &cli.Command{
		Name:        "helper",
		Usage:       gotext.Get("Run a ALR helper command"),
		ArgsUsage:   `<helper_name|"list">`,
		Subcommands: []*cli.Command{helperListCmd, helperHelpCmd},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "dest-dir",
				Aliases: []string{"d"},
				Usage:   gotext.Get("The directory that the install commands will install to"),
				Value:   "dest",
			},
		},
		Action: func(c *cli.Context) error {
			ctx := c.Context

			if c.Args().Len() < 1 {
				cli.ShowSubcommandHelpAndExit(c, 1)
			}

			helper, ok := helpers.Helpers[c.Args().First()]
			if !ok {
				slog.Error(gotext.Get("No such helper command"), "name", c.Args().First())
				return cli.Exit(gotext.Get("No such helper command"), 1)
			}

			wd, err := os.Getwd()
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error getting working directory"), err)
			}

			info, err := distro.ParseOSRelease(ctx)
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error parsing os-release file"), err)
			}

			hc := interp.HandlerContext{
				Env: expand.ListEnviron(
					"pkgdir="+c.String("dest-dir"),
					"DISTRO_ID="+info.ID,
					"DISTRO_ID_LIKE="+strings.Join(info.Like, " "),
					"ARCH="+cpu.Arch(),
				),
				Dir:    wd,
				Stdin:  os.Stdin,
				Stdout: os.Stdout,
				Stderr: os.Stderr,
			}

			return helper(hc, c.Args().First(), c.Args().Slice()[1:])
		},
		BashComplete: cliutils.BashCompleteWithError(func(ctx *cli.Context) error {
			if cliutils.MaybePrintFlags(ctx) {
				return nil
			}

			prefix := cliutils.CompletionPrefix(ctx)
			names := make([]string, 0, len(helpers.Helpers))
			for name := range helpers.Helpers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				if prefix == "" || strings.HasPrefix(name, prefix) {
					cliutils.PrintCompletionItem(name, false)
				}
			}
			return nil
		}),
	}
}
