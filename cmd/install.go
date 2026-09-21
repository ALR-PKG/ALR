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

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/ALR/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/utils"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

func newInstallCmd() *cobra.Command {
	var (
		clean   bool
		pkgName string
	)

	cmd := &cobra.Command{
		Use:     "install",
		Short:   gotext.Get("Install a new package"),
		Aliases: []string{"in"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var packages []string
			if cmd.Flags().Changed("package") {
				packages = []string{pkgName}
			} else if len(args) > 0 {
				packages = args
			} else {
				return fmt.Errorf("%s", gotext.Get("Command install expected at least 1 argument, got 0"))
			}

			ctx := cmd.Context()

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

			installer, installerClose, err := build.GetSafeInstaller()
			if err != nil {
				return err
			}
			defer installerClose()

			scripter, scripterClose, err := build.GetSafeScriptExecutor(deps.Cfg, "")
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

			if err := utils.CheckUserPrivileges(); err != nil {
				return err
			}

			_, err = builder.InstallPkgs(
				ctx,
				&build.BuildArgs{
					Opts: &types.BuildOpts{
						Clean:       clean,
						Interactive: interactive,
					},
					Info:       deps.Info,
					PkgFormat_: build.GetPkgFormat(deps.Manager),
				},
				packages,
			)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error when installing the package"), err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&clean, "clean", "c", false, gotext.Get("Build package from scratch even if there's an already built package available"))
	cmd.Flags().StringVarP(&pkgName, "package", "p", "", gotext.Get("Package to install"))

	_ = cmd.RegisterFlagCompletionFunc("package", installPackageCompletion)

	addFlagCompletion(cmd)
	return cmd
}

func installPackageCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := cmd.Context()
	deps, err := appbuilder.
		New(ctx).
		WithConfig().
		WithDB().
		WithReposNoPull().
		WithManager().
		Build()
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

	seen := make(map[string]struct{})
	for _, name := range out {
		seen[name] = struct{}{}
	}

	if toComplete != "" {
		sysPkgs, err := deps.Manager.ListAvailable(toComplete)
		if err != nil {
			slog.Debug("failed to list system packages", "err", err)
		} else {
			for _, name := range sysPkgs {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					out = append(out, name)
				}
			}
		}
	}

	return out, cobra.ShellCompDirectiveNoFileComp
}
