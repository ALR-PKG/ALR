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

package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strings"
	"text/template"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/manager"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/overrides"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
)

func newListCmd() *cobra.Command {
	var (
		installed  bool
		upgradable bool
		formatFlag string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   gotext.Get("List ALR repo packages"),
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithManager().
				WithDistroInfo().
				// autoPull only
				WithRepos().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			cfg := deps.Cfg
			db := deps.DB
			mgr := deps.Manager
			info := deps.Info

			if upgradable {
				updates, err := checkForUpdates(ctx, mgr, db, info, cfg, deps.Repos)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error getting packages for upgrade"), err)
				}
				if len(updates) == 0 {
					slog.Info(gotext.Get("No packages for upgrade"))
					return nil
				}

				format := formatFlag
				if format == "" {
					format = "{{.Package.Repository}}/{{.Package.Name}} {{.FromVersion}} -> {{.ToVersion}}\n"
				}
				tmpl, err := template.New("format").Parse(format)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error parsing format template"), err)
				}

				for _, updateInfo := range updates {
					err = tmpl.Execute(os.Stdout, updateInfo)
					if err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("Error executing template"), err)
					}
				}

				return nil
			}

			where := "true"
			queryArgs := []any(nil)
			if len(args) > 0 {
				where = "name LIKE ? OR json_array_contains(provides, ?)"
				queryArgs = []any{args[0], args[0]}
			}

			var result []alrsh.Package

			if deps.Repos != nil && deps.Repos.PreferRPC() {
				if len(args) == 0 {
					result, err = deps.Repos.ListAllPackages(ctx)
				} else {
					result, err = deps.Repos.SearchPackages(ctx, args[0])
				}
			} else {
				result, err = db.GetPkgs(ctx, where, queryArgs...)
			}
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error getting packages"), err)
			}

			sort.Slice(result, func(i, j int) bool {
				return result[i].Name < result[j].Name
			})

			type verInfo struct {
				Version string
				Release int
			}

			installedAlrPackages := map[string]verInfo{}
			if installed {
				mgr := manager.Detect()
				if mgr == nil {
					return fmt.Errorf("%s", gotext.Get("Unable to detect a supported package manager on the system"))
				}

				installedList, err := mgr.ListInstalled(&manager.Opts{})
				if err != nil {
					slog.Error(gotext.Get("Error listing installed packages"), "err", err)
					return fmt.Errorf("%s: %w", gotext.Get("Error listing installed packages"), err)
				}

				for pkgName, version := range installedList {
					matches := build.RegexpALRPackageName.FindStringSubmatch(pkgName)
					if matches != nil {
						packageName := matches[build.RegexpALRPackageName.SubexpIndex("package")]
						repoName := matches[build.RegexpALRPackageName.SubexpIndex("repo")]

						verInfo := verInfo{
							Version: version,
							Release: 0,
						}

						if i := strings.LastIndex(version, "-"); i != -1 {
							rel, err := overrides.ParseReleasePlatformSpecific(version[i+1:], info)
							if err != nil {
								// Системный пакет, случайно совпавший с форматом
								// имени ALR (например, libstdc++6 с релизом
								// 1ubuntu1) — пропускаем, не роняя команду
								slog.Debug("list: не удалось разобрать релиз системного пакета, пропускаем",
									"pkg", pkgName, "version", version, "err", err)
								continue
							}
							verInfo.Version = version[:i]
							verInfo.Release = rel
						}

						installedAlrPackages[fmt.Sprintf("%s/%s", repoName, packageName)] = verInfo
					}
				}
			}

			for _, pkg := range result {
				if slices.Contains(cfg.IgnorePkgUpdates(), pkg.Name) {
					continue
				}

				type packageInfo struct {
					Package *alrsh.Package
				}

				pkgInfo := &packageInfo{}
				pkgInfo.Package = &pkg
				if installed {
					instVersion, ok := installedAlrPackages[fmt.Sprintf("%s/%s", pkg.Repository, pkg.Name)]
					if !ok {
						continue
					} else {
						pkg.Version = instVersion.Version
						pkg.Release = instVersion.Release
					}
				}

				format := formatFlag
				if format == "" {
					format = "{{.Package.Repository}}/{{.Package.Name}} {{.Package.Version}}-{{.Package.Release}}\n"
				}
				tmpl, err := template.New("format").Parse(format)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error parsing format template"), err)
				}
				err = tmpl.Execute(os.Stdout, pkgInfo)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error executing template"), err)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&installed, "installed", "I", false, "")
	cmd.Flags().BoolVarP(&upgradable, "upgradable", "U", false, "")
	cmd.Flags().StringVarP(&formatFlag, "format", "f", "", gotext.Get("Format output using a Go template"))

	addFlagCompletion(cmd)
	return cmd
}
