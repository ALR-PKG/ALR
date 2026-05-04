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
	"strings"
	"time"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v2"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/manager"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

func InstallCmd() *cli.Command {
	return &cli.Command{
		Name:    "install",
		Usage:   gotext.Get("Install a new package"),
		Aliases: []string{"in"},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "clean",
				Aliases: []string{"c"},
				Usage:   gotext.Get("Build package from scratch even if there's an already built package available"),
			},
		},
		Action: utils.RootNeededAction(func(c *cli.Context) error {
			args := c.Args()
			if args.Len() < 1 {
				return cliutils.FormatCliExit(gotext.Get("Command install expected at least 1 argument, got %d", args.Len()), nil)
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

			ctx := c.Context

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithRepos().
				WithDistroInfo().
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

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

			_, err = builder.InstallPkgs(
				ctx,
				&build.BuildArgs{
					Opts: &types.BuildOpts{
						Clean:       c.Bool("clean"),
						Interactive: c.Bool("interactive"),
					},
					Info:       deps.Info,
					PkgFormat_: build.GetPkgFormat(deps.Manager),
				},
				args.Slice(),
			)
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error when installing the package"), err)
			}

			return nil
		}),
		BashComplete: cliutils.BashCompleteWithError(func(c *cli.Context) error {
			if cliutils.MaybePrintFlags(c) {
				return nil
			}

			ctx := c.Context
			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithReposNoPull().
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			seen := make(map[string]struct{})
			prefix := cliutils.CompletionPrefix(c)

			where, args := cliutils.BuildPkgQuery(prefix)
			result, err := deps.DB.GetPkgs(c.Context, where, args...)
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error getting packages"), err)
			}

			for _, pkg := range result {
				if prefix == "" || strings.HasPrefix(pkg.Name, prefix) {
					if _, ok := seen[pkg.Name]; !ok {
						seen[pkg.Name] = struct{}{}
						cliutils.PrintCompletionItem(pkg.Name, true)
					}
				}
			}

			// Если RPC доступен и есть префикс — запрашиваем пакеты с сайта для подстановки
			if prefix != "" && deps.Repos != nil && deps.Repos.HasRPC() {
				slog.Debug("BashComplete: запрос пакетов через RPC для подстановки", "prefix", prefix)
				rpcPkgs, err := deps.Repos.RPCClient().SearchPackages(ctx, "name", prefix)
				if err == nil {
					for _, p := range rpcPkgs {
						pkg := *p.ToALRShPackage()
						pkg.CachedAt = time.Now().Unix()
						if _, ok := seen[pkg.Name]; !ok {
							seen[pkg.Name] = struct{}{}
							cliutils.PrintCompletionItem(pkg.Name, true)
						}
						// Сохраняем в локальную БД для будущих подстановок
						if dbErr := deps.DB.InsertPackage(ctx, pkg); dbErr != nil {
							slog.Debug("BashComplete: не удалось сохранить пакет в кэш", "name", pkg.Name, "error", dbErr)
						}
					}
				} else if err != nil {
					slog.Debug("BashComplete: RPC поиск не удался", "prefix", prefix, "error", err)
				}
			}

			if prefix != "" {
				sysPkgs, err := deps.Manager.ListAvailable(prefix)
				if err != nil {
					slog.Debug("failed to list system packages", "err", err)
				} else {
					for _, name := range sysPkgs {
						if _, ok := seen[name]; !ok {
							seen[name] = struct{}{}
							cliutils.PrintCompletionItem(name, false)
						}
					}
				}
			}

			return nil
		}),
	}
}

func RemoveCmd() *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Usage:   gotext.Get("Remove an installed package"),
		Aliases: []string{"rm"},
		BashComplete: cliutils.BashCompleteWithError(func(c *cli.Context) error {
			if cliutils.MaybePrintFlags(c) {
				return nil
			}

			ctx := c.Context
			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			prefix := cliutils.CompletionPrefix(c)
			seen := make(map[string]struct{})

			installedAlrPackages := map[string]string{}
			installed, err := deps.Manager.ListInstalled(&manager.Opts{})
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error listing installed packages"), err)
			}
			for pkgName, version := range installed {
				matches := build.RegexpALRPackageName.FindStringSubmatch(pkgName)
				if matches != nil {
					packageName := matches[build.RegexpALRPackageName.SubexpIndex("package")]
					repoName := matches[build.RegexpALRPackageName.SubexpIndex("repo")]
					installedAlrPackages[fmt.Sprintf("%s/%s", repoName, packageName)] = version
				}
			}

			var result []alrsh.Package
			if prefix != "" {
				where, args := cliutils.BuildPkgQuery(prefix)
				result, err = deps.DB.GetPkgs(c.Context, where, args...)
			} else if len(installedAlrPackages) > 0 {
				nameSet := make(map[string]struct{})
				for key := range installedAlrPackages {
					parts := strings.SplitN(key, "/", 2)
					if len(parts) == 2 {
						nameSet[parts[1]] = struct{}{}
					}
				}
				if len(nameSet) > 900 {
					// SQLite лимит на 999 параметров; fallback на полную выборку
					result, err = deps.DB.GetPkgs(c.Context, "true")
				} else if len(nameSet) > 0 {
					names := make([]string, 0, len(nameSet))
					for n := range nameSet {
						names = append(names, n)
					}
					placeholders := strings.Repeat("?,", len(names))
					placeholders = placeholders[:len(placeholders)-1]
					args := make([]any, len(names))
					for i, n := range names {
						args[i] = n
					}
					result, err = deps.DB.GetPkgs(c.Context, "name IN ("+placeholders+")", args...)
				}
			}
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error getting packages"), err)
			}

			for _, pkg := range result {
				_, ok := installedAlrPackages[fmt.Sprintf("%s/%s", pkg.Repository, pkg.Name)]
				if !ok {
					continue
				}
				if prefix == "" || strings.HasPrefix(pkg.Name, prefix) {
					if _, exists := seen[pkg.Name]; !exists {
						seen[pkg.Name] = struct{}{}
						cliutils.PrintCompletionItem(pkg.Name, true)
					}
				}
			}

			if prefix != "" {
				for pkgName := range installed {
					if build.RegexpALRPackageName.MatchString(pkgName) {
						continue
					}
					if strings.HasPrefix(pkgName, prefix) {
						if _, exists := seen[pkgName]; !exists {
							seen[pkgName] = struct{}{}
							cliutils.PrintCompletionItem(pkgName, false)
						}
					}
				}
			}

			return nil
		}),
		Action: utils.RootNeededAction(func(c *cli.Context) error {
			args := c.Args()
			if args.Len() < 1 {
				return cliutils.FormatCliExit(gotext.Get("Command remove expected at least 1 argument, got %d", args.Len()), nil)
			}

			deps, err := appbuilder.
				New(c.Context).
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			// Транслируем короткие имена ALR-пакетов в полные (name+repo)
			resolvedPkgs, err := resolveInstalledALRNames(deps.Manager, c.Args().Slice())
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error removing packages"), err)
			}

			if err := deps.Manager.Remove(&manager.Opts{
				NoConfirm: !c.Bool("interactive"),
			}, resolvedPkgs...); err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error removing packages"), err)
			}

			return nil
		}),
	}
}

// resolveInstalledALRNames транслирует короткие имена пакетов в полные имена ALR (name+repo).
// Если ALR-пакет с таким именем не установлен, имя передаётся как есть.
func resolveInstalledALRNames(mgr manager.Manager, names []string) ([]string, error) {
	installed, err := mgr.ListInstalled(nil)
	if err != nil {
		return nil, err
	}

	// Строим карту: короткое имя → полное имя (name+repo)
	shortToFull := make(map[string]string)
	for fullName := range installed {
		matches := build.RegexpALRPackageName.FindStringSubmatch(fullName)
		if matches != nil {
			pkgName := matches[build.RegexpALRPackageName.SubexpIndex("package")]
			shortToFull[pkgName] = fullName
		}
	}

	resolved := make([]string, len(names))
	for i, name := range names {
		if fullName, ok := shortToFull[name]; ok {
			resolved[i] = fullName
		} else {
			resolved[i] = name
		}
	}

	return resolved, nil
}
