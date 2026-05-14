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
	"strings"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/manager"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
)

func newRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "remove",
		Short:             gotext.Get("Remove an installed package"),
		Aliases:           []string{"rm"},
		ValidArgsFunction: removePackageCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			if len(args) < 1 {
				return fmt.Errorf("%s", gotext.Get("Command remove expected at least 1 argument, got %d", len(args)))
			}

			ctx := cmd.Context()

			deps, err := appbuilder.
				New(ctx).
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			resolvedPkgs, err := resolveInstalledALRNames(deps.Manager, args)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error removing packages"), err)
			}

			if err := deps.Manager.Remove(&manager.Opts{
				NoConfirm: !interactive,
			}, resolvedPkgs...); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error removing packages"), err)
			}

			return nil
		},
	}

	return cmd
}

func removePackageCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := cmd.Context()
	deps, err := appbuilder.
		New(ctx).
		WithConfig().
		WithDB().
		WithManager().
		Build()
	if err != nil {
		slog.Debug("completion: ошибка инициализации appbuilder", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}
	defer deps.Defer()

	prefix := toComplete
	seen := make(map[string]struct{})

	installedAlrPackages := map[string]string{}
	installed, err := deps.Manager.ListInstalled(&manager.Opts{})
	if err != nil {
		slog.Debug("completion: ошибка получения установленных пакетов", "error", err)
		return nil, cobra.ShellCompDirectiveError
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
		where, queryArgs := cliutils.BuildPkgQuery(prefix)
		result, err = deps.DB.GetPkgs(ctx, where, queryArgs...)
	} else if len(installedAlrPackages) > 0 {
		nameSet := make(map[string]struct{})
		for key := range installedAlrPackages {
			parts := strings.SplitN(key, "/", 2)
			if len(parts) == 2 {
				nameSet[parts[1]] = struct{}{}
			}
		}
		if len(nameSet) > 900 {
			result, err = deps.DB.GetPkgs(ctx, "true")
		} else if len(nameSet) > 0 {
			names := make([]string, 0, len(nameSet))
			for n := range nameSet {
				names = append(names, n)
			}
			placeholders := strings.Repeat("?,", len(names))
			placeholders = placeholders[:len(placeholders)-1]
			queryArgs := make([]any, len(names))
			for i, n := range names {
				queryArgs[i] = n
			}
			result, err = deps.DB.GetPkgs(ctx, "name IN ("+placeholders+")", queryArgs...)
		}
	}
	if err != nil {
		slog.Debug("completion: ошибка получения пакетов", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}

	var out []string
	for _, pkg := range result {
		_, ok := installedAlrPackages[fmt.Sprintf("%s/%s", pkg.Repository, pkg.Name)]
		if !ok {
			continue
		}
		if prefix == "" || strings.HasPrefix(pkg.Name, prefix) {
			if _, exists := seen[pkg.Name]; !exists {
				seen[pkg.Name] = struct{}{}
				out = append(out, pkg.Name)
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
					out = append(out, pkgName)
				}
			}
		}
	}

	return out, cobra.ShellCompDirectiveNoFileComp
}

func resolveInstalledALRNames(mgr manager.Manager, names []string) ([]string, error) {
	installed, err := mgr.ListInstalled(nil)
	if err != nil {
		return nil, err
	}

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
