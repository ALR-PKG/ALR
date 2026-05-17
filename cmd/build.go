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



	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/osutils"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"

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
	addFlagCompletion(cmd)

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

	out, err := completePackages(ctx, deps.DB, deps.Repos, toComplete)
	if err != nil {
		slog.Debug("completion: ошибка", "error", err)
		return nil, cobra.ShellCompDirectiveError
	}

	return out, cobra.ShellCompDirectiveNoFileComp
}


