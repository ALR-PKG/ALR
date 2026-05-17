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


	"github.com/goccy/go-yaml"
	"github.com/jeandeaual/go-locale"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/overrides"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
)

func newInfoCmd() *cobra.Command {
	var all bool

	cmd := &cobra.Command{
		Use:   "info",
		Short: gotext.Get("Print information about a package"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("%s", gotext.Get("Command info expected at least 1 argument, got %d", len(args)))
			}

			ctx := cmd.Context()

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

			found, _, err := rs.FindPkgs(ctx, args)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error finding packages"), err)
			}

			if len(found) == 0 {
				return fmt.Errorf("%s", gotext.Get("Package not found"))
			}

			pkgs := cliutils.FlattenPkgs(ctx, found, "show", interactive)

			var names []string

			systemLang, err := locale.GetLanguage()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Can't detect system language"), err)
			}
			if systemLang == "" {
				systemLang = "en"
			}

			info, err := distro.ParseOSRelease(ctx)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error parsing os-release file"), err)
			}
			names, err = overrides.Resolve(
				info,
				overrides.DefaultOpts.
					WithLanguages([]string{systemLang}),
			)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error resolving overrides"), err)
			}

			for _, pkg := range pkgs {
				alrsh.ResolvePackage(&pkg, names)
				view := alrsh.NewPackageView(pkg)
				view.Resolved = !all
				err = yaml.NewEncoder(os.Stdout, yaml.UseJSONMarshaler(), yaml.OmitEmpty()).Encode(view)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error encoding script variables"), err)
				}
				fmt.Println("---")
			}

			return nil
		},
		ValidArgsFunction: infoPackageCompletion,
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, gotext.Get("Show all information, not just for the current distro"))

	addFlagCompletion(cmd)
	return cmd
}

func infoPackageCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ctx := cmd.Context()
	deps, err := appbuilder.
		New(ctx).
		WithConfig().
		WithDB().
		WithReposNoPull().
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
	return out, cobra.ShellCompDirectiveNoFileComp
}
