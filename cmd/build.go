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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/osutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

func newBuildCmd() *cobra.Command {
	var (
		script     string
		subpackage string
		pkgName    string
		clean      bool
	)

	cmd := &cobra.Command{
		Use:   "build",
		Short: gotext.Get("Build a local package"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			wd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error getting working directory"), err)
			}

			ctx := cmd.Context()

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithReposNoPull().
				WithDistroInfo().
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			var res []*build.BuiltDep
			buildArgs := &build.BuildArgs{
				Opts: &types.BuildOpts{
					Clean:       clean,
					Interactive: interactive,
				},
				PkgFormat_: build.GetPkgFormat(deps.Manager),
				Info:       deps.Info,
			}

			switch {
			case cmd.Flags().Changed("script"):
				scriptPath, err := filepath.Abs(script)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Cannot get absolute script path"), err)
				}

				var packages []string
				if subpackage != "" {
					packages = append(packages, subpackage)
				}

				scriptArgs := &build.BuildPackageFromScriptArgs{
					Script:    scriptPath,
					Packages:  packages,
					BuildArgs: *buildArgs,
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

				builder, err := build.NewMainBuilder(deps.Cfg, deps.Manager, deps.Repos, scripter, installer)
				if err != nil {
					return err
				}

				res, err = builder.BuildPackageFromScript(ctx, scriptArgs)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error building package"), err)
				}

			case cmd.Flags().Changed("package"):
				pkgs, _, err := deps.Repos.FindPkgs(ctx, []string{pkgName})
				if err != nil {
					return fmt.Errorf("failed to find pkgs: %w", err)
				}

				pkg := cliutils.FlattenPkgs(ctx, pkgs, "build", interactive)
				if len(pkg) < 1 {
					return fmt.Errorf("%s", gotext.Get("Package not found"))
				}

				var packages []string
				if pkg[0].BasePkgName != "" {
					packages = append(packages, pkg[0].Name)
				}

				dbArgs := &build.BuildPackageFromDbArgs{
					Package:   &pkg[0],
					Packages:  packages,
					BuildArgs: *buildArgs,
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

				builder, err := build.NewMainBuilder(deps.Cfg, deps.Manager, deps.Repos, scripter, installer)
				if err != nil {
					return err
				}

				res, err = builder.BuildPackageFromDb(ctx, dbArgs)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error building package"), err)
				}

			default:
				return fmt.Errorf("%s", gotext.Get("Nothing to build"))
			}

			for _, pkg := range res {
				name := filepath.Base(pkg.Path)
				if _, err := os.Stat(pkg.Path); os.IsNotExist(err) {
					slog.Info(gotext.Get("Package file already moved or removed, skipping"), "path", pkg.Path)
					continue
				}
				if err := osutils.Move(pkg.Path, filepath.Join(wd, name)); err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error moving the package"), err)
				}
			}

			slog.Info(gotext.Get("Done"))
			return nil
		},
	}

	cmd.Flags().StringVarP(&script, "script", "s", "alr.sh", gotext.Get("Path to the build script"))
	cmd.Flags().StringVar(&subpackage, "subpackage", "", gotext.Get("Specify subpackage in script (for multi package script only)"))
	cmd.Flags().StringVarP(&pkgName, "package", "p", "", gotext.Get("Name of the package to build and its repo (example: default/go-bin)"))
	cmd.Flags().BoolVarP(&clean, "clean", "c", false, gotext.Get("Build package from scratch even if there's an already built package available"))

	_ = cmd.RegisterFlagCompletionFunc("package", packageCompletion)

	return cmd
}

func packageCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := cmd.Context()
	deps, err := appbuilder.New(ctx).WithConfig().WithDB().WithReposNoPull().Build()
	if err != nil {
		slog.Debug("completion: ошибка инициализации appbuilder", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}
	defer deps.Defer()

	prefix := toComplete
	var result []alrsh.Package
	parts := strings.SplitN(prefix, "/", 2)
	if len(parts) == 2 {
		result, err = deps.DB.GetPkgs(ctx,
			"name LIKE ? ESCAPE ? AND repository = ?",
			cliutils.EscapeLikePattern(parts[1])+"%", "\\", parts[0])
	} else {
		where, queryArgs := cliutils.BuildPkgQuery(prefix)
		result, err = deps.DB.GetPkgs(ctx, where, queryArgs...)
	}
	if err != nil {
		slog.Debug("completion: ошибка получения пакетов", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}

	seen := make(map[string]struct{})
	var out []string
	for _, pkg := range result {
		key := fmt.Sprintf("%s/%s", pkg.Repository, pkg.Name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}

	if deps.Repos != nil && deps.Repos.HasRPC() {
		searchTerm := prefix
		repoFilter := ""
		if len(parts) == 2 {
			searchTerm = parts[1]
			repoFilter = parts[0]
		}
		rpcPkgs, err := deps.Repos.RPCClient().SearchPackages(ctx, "name", searchTerm)
		if err == nil {
			for _, p := range rpcPkgs {
				pkg := *p.ToALRShPackage()
				pkg.CachedAt = time.Now().Unix()
				if repoFilter != "" && pkg.Repository != repoFilter {
					continue
				}
				key := fmt.Sprintf("%s/%s", pkg.Repository, pkg.Name)
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				out = append(out, key)
				if dbErr := deps.DB.InsertPackage(ctx, pkg); dbErr != nil {
					slog.Debug("completion: не удалось сохранить пакет в кэш", "name", pkg.Name, "error", dbErr)
				}
			}
		} else {
			slog.Debug("completion: RPC поиск не удался", "prefix", prefix, "error", err)
		}
	}

	return out, cobra.ShellCompDirectiveNoFileComp
}

func buildPkgQuery(prefix string) (where string, args []any) {
	if prefix != "" {
		return "name LIKE ? ESCAPE ?", []any{cliutils.EscapeLikePattern(prefix) + "%", "\\"}
	}
	return "true", nil
}


