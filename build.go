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
	"path/filepath"
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v2"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/osutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

// isFileFlag возвращает true, если arg — это флаг, ожидающий путь к файлу.
// Учитывает как отдельные флаги (-s foo), так и --flag=value (--script=foo).
func isFileFlag(arg string) bool {
	return arg == "-s" || arg == "--script" ||
		strings.HasPrefix(arg, "-s=") || strings.HasPrefix(arg, "--script=")
}

// isPackageFlag возвращает true, если arg — это флаг -p/--package (с или без значения).
func isPackageFlag(arg string) bool {
	return arg == "-p" || arg == "--package" ||
		strings.HasPrefix(arg, "-p=") || strings.HasPrefix(arg, "--package=")
}

// isSubpackageFlag возвращает true, если arg — это флаг -sb/--subpackage (с или без значения).
func isSubpackageFlag(arg string) bool {
	return arg == "-sb" || arg == "--subpackage" ||
		strings.HasPrefix(arg, "-sb=") || strings.HasPrefix(arg, "--subpackage=")
}

func BuildCmd() *cli.Command {
	return &cli.Command{
		Name:  "build",
		Usage: gotext.Get("Build a local package"),
		BashComplete: cliutils.BashCompleteWithError(func(c *cli.Context) error {
			var lastArg, prevArg string
			if len(os.Args) > 2 {
				lastArg = os.Args[len(os.Args)-2]
			}
			if len(os.Args) > 3 {
				prevArg = os.Args[len(os.Args)-3]
			}

			// Если ожидается значение флага — позволяем shell сделать file completion
			if isFileFlag(lastArg) || isFileFlag(prevArg) {
				return nil
			}

			// Для -sb/--subpackage не можем предложить completion (нужен парсинг скрипта)
			if isSubpackageFlag(lastArg) || isSubpackageFlag(prevArg) {
				return nil
			}

			// Для -p/--package выводим ALR-пакеты в формате repo/name
			if isPackageFlag(lastArg) || isPackageFlag(prevArg) {
				ctx := c.Context
				deps, err := appbuilder.New(ctx).WithConfig().WithDB().Build()
				if err != nil {
					return err
				}
				defer deps.Defer()

				prefix := cliutils.CompletionPrefix(c)

				var result []alrsh.Package
				parts := strings.SplitN(prefix, "/", 2)
				if len(parts) == 2 {
					result, err = deps.DB.GetPkgs(ctx,
						"name LIKE ? ESCAPE ? AND repository = ?",
						cliutils.EscapeLikePattern(parts[1])+"%", "\\", parts[0])
				} else {
					where, args := cliutils.BuildPkgQuery(prefix)
					result, err = deps.DB.GetPkgs(ctx, where, args...)
				}
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("Error getting packages"), err)
				}

				for _, pkg := range result {
					fmt.Printf("%s/%s\n", pkg.Repository, pkg.Name)
				}
				return nil
			}

			// Выводим флаги команды build
			cliutils.PrintCompletionFlags(c.Command.Flags)
			return nil
		}),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "script",
				Aliases: []string{"s"},
				Value:   "alr.sh",
				Usage:   gotext.Get("Path to the build script"),
			},
			&cli.StringFlag{
				Name:    "subpackage",
				Aliases: []string{"sb"},
				Usage:   gotext.Get("Specify subpackage in script (for multi package script only)"),
			},
			&cli.StringFlag{
				Name:    "package",
				Aliases: []string{"p"},
				Usage:   gotext.Get("Name of the package to build and its repo (example: default/go-bin)"),
			},
			&cli.BoolFlag{
				Name:    "clean",
				Aliases: []string{"c"},
				Usage:   gotext.Get("Build package from scratch even if there's an already built package available"),
			},
		},
		Action: func(c *cli.Context) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			wd, err := os.Getwd()
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error getting working directory"), err)
			}

			ctx := c.Context

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithReposNoPull().
				WithDistroInfo().
				WithManager().
				Build()
			if err != nil {
				return cli.Exit(err, 1)
			}
			defer deps.Defer()

			var script string
			var packages []string

			var res []*build.BuiltDep

			var scriptArgs *build.BuildPackageFromScriptArgs
			var dbArgs *build.BuildPackageFromDbArgs

			buildArgs := &build.BuildArgs{
				Opts: &types.BuildOpts{
					Clean:       c.Bool("clean"),
					Interactive: c.Bool("interactive"),
				},
				PkgFormat_: build.GetPkgFormat(deps.Manager),
				Info:       deps.Info,
			}

			switch {
			case c.IsSet("script"):
				script, err = filepath.Abs(c.String("script"))
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("Cannot get absolute script path"), err)
				}

				subpackage := c.String("subpackage")

				if subpackage != "" {
					packages = append(packages, subpackage)
				}

				scriptArgs = &build.BuildPackageFromScriptArgs{
					Script:    script,
					Packages:  packages,
					BuildArgs: *buildArgs,
				}
			case c.IsSet("package"):
				// TODO: handle multiple packages
				packageInput := c.String("package")

				pkgs, _, err := deps.Repos.FindPkgs(ctx, []string{packageInput})
				if err != nil {
					return cliutils.FormatCliExit("failed to find pkgs", err)
				}

				pkg := cliutils.FlattenPkgs(ctx, pkgs, "build", c.Bool("interactive"))

				if len(pkg) < 1 {
					return cliutils.FormatCliExit(gotext.Get("Package not found"), nil)
				}

				if pkg[0].BasePkgName != "" {
					packages = append(packages, pkg[0].Name)
				}

				dbArgs = &build.BuildPackageFromDbArgs{
					Package:   &pkg[0],
					Packages:  packages,
					BuildArgs: *buildArgs,
				}
			default:
				return cliutils.FormatCliExit(gotext.Get("Nothing to build"), nil)
			}

			installer, installerClose, err := build.GetSafeInstaller()
			if err != nil {
				return err
			}
			defer installerClose()

			scripter, scripterClose, err := build.GetSafeScriptExecutor()
			if err != nil {
				return err
			}
			defer scripterClose()

			builder, err := build.NewMainBuilder(
				deps.Cfg,
				deps.Manager,
				deps.Repos,
				scripter,
				installer,
			)
			if err != nil {
				return err
			}

			if scriptArgs != nil {
				res, err = builder.BuildPackageFromScript(
					ctx,
					scriptArgs,
				)
			} else if dbArgs != nil {
				res, err = builder.BuildPackageFromDb(
					ctx,
					dbArgs,
				)
			}

			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error building package"), err)
			}

			for _, pkg := range res {
				name := filepath.Base(pkg.Path)

				// Проверяем, существует ли файл перед перемещением
				if _, err := os.Stat(pkg.Path); os.IsNotExist(err) {
					slog.Info(gotext.Get("Package file already moved or removed, skipping"), "path", pkg.Path)
					continue
				}

				err = osutils.Move(pkg.Path, filepath.Join(wd, name))
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("Error moving the package"), err)
				}
			}

			slog.Info(gotext.Get("Done"))

			return nil
		},
	}
}
