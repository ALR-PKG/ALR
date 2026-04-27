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
	"os"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/jeandeaual/go-locale"
	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v2"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/overrides"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
)

func InfoCmd() *cli.Command {
	return &cli.Command{
		Name:  "info",
		Usage: gotext.Get("Print information about a package"),
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "all",
				Aliases: []string{"a"},
				Usage:   gotext.Get("Show all information, not just for the current distro"),
			},
		},
		BashComplete: cliutils.BashCompleteWithError(func(c *cli.Context) error {
			if cliutils.MaybePrintFlags(c) {
				return nil
			}

			ctx := c.Context
			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			prefix := cliutils.CompletionPrefix(c)

			where, args := cliutils.BuildPkgQuery(prefix)
			result, err := deps.DB.GetPkgs(c.Context, where, args...)
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error getting packages"), err)
			}

			seen := make(map[string]struct{})
			for _, pkg := range result {
				if prefix == "" || strings.HasPrefix(pkg.Name, prefix) {
					if _, ok := seen[pkg.Name]; !ok {
						seen[pkg.Name] = struct{}{}
						cliutils.PrintCompletionItem(pkg.Name, true)
					}
				}
			}
			return nil
		}),
		Action: func(c *cli.Context) error {
			// Запуск от текущего пользователя

			args := c.Args()
			if args.Len() < 1 {
				return cliutils.FormatCliExit(gotext.Get("Command info expected at least 1 argument, got %d", args.Len()), nil)
			}

			ctx := c.Context

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithDistroInfo().
				WithRepos().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			rs := deps.Repos

			found, _, err := rs.FindPkgs(ctx, args.Slice())
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error finding packages"), err)
			}

			if len(found) == 0 {
				return cliutils.FormatCliExit(gotext.Get("Package not found"), err)
			}

			pkgs := cliutils.FlattenPkgs(ctx, found, "show", c.Bool("interactive"))

			var names []string
			all := c.Bool("all")

			systemLang, err := locale.GetLanguage()
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Can't detect system language"), err)
			}
			if systemLang == "" {
				systemLang = "en"
			}

			info, err := distro.ParseOSRelease(ctx)
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error parsing os-release file"), err)
			}
			names, err = overrides.Resolve(
				info,
				overrides.DefaultOpts.
					WithLanguages([]string{systemLang}),
			)
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error resolving overrides"), err)
			}

			for _, pkg := range pkgs {
				alrsh.ResolvePackage(&pkg, names)
				view := alrsh.NewPackageView(pkg)
				view.Resolved = !all
				err = yaml.NewEncoder(os.Stdout, yaml.UseJSONMarshaler(), yaml.OmitEmpty()).Encode(view)
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("Error encoding script variables"), err)
				}
				fmt.Println("---")
			}

			return nil
		},
	}
}
