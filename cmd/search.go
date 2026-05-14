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
	"os"
	"text/template"

	"github.com/jeandeaual/go-locale"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/overrides"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/search"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
)

func newSearchCmd() *cobra.Command {
	var (
		all         bool
		name        string
		description string
		repository  string
		provides    string
		format      string
	)

	cmd := &cobra.Command{
		Use:     "search",
		Short:   gotext.Get("Search packages"),
		Aliases: []string{"s"},
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			var names []string

			all, _ = cmd.Flags().GetBool("all")

			systemLang, err := locale.GetLanguage()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Can't detect system language"), err)
			}
			if systemLang == "" {
				systemLang = "en"
			}
			if !all {
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
			}

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			database := deps.DB

			s := search.New(database)

			name, _ = cmd.Flags().GetString("name")
			description, _ = cmd.Flags().GetString("description")
			repository, _ = cmd.Flags().GetString("repository")
			provides, _ = cmd.Flags().GetString("provides")

			packages, err := s.Search(
				ctx,
				search.NewSearchOptions().
					WithName(name).
					WithDescription(description).
					WithRepository(repository).
					WithProvides(provides).
					Build(),
			)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error while executing search"), err)
			}

			format, _ = cmd.Flags().GetString("format")
			var tmpl *template.Template
			if format != "" {
				tmpl, err = template.New("format").Parse(format)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Error parsing format template"), err)
				}
			}

			for _, pkg := range packages {
				alrsh.ResolvePackage(&pkg, names)
				if tmpl != nil {
					err = tmpl.Execute(os.Stdout, &pkg)
					if err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("Error executing template"), err)
					}
					fmt.Println()
				} else {
					fmt.Println(pkg.Name)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&all, "all", "a", false, gotext.Get("Show all information, not just for the current distro"))
	cmd.Flags().StringVarP(&name, "name", "n", "", gotext.Get("Search by name"))
	cmd.Flags().StringVarP(&description, "description", "d", "", gotext.Get("Search by description"))
	cmd.Flags().StringVarP(&repository, "repository", "r", "", gotext.Get("Search by repository"))
	cmd.Flags().StringVarP(&provides, "provides", "p", "", gotext.Get("Search by provides"))
	cmd.Flags().StringVarP(&format, "format", "f", "", gotext.Get("Format output using a Go template"))

	return cmd
}
