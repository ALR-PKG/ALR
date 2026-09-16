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

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/constants"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: gotext.Get("Print the current ALR version and exit"),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(config.Version)

			siteURL := constants.DefaultSiteURL
			deps, err := appbuilder.New(cmd.Context()).WithConfig().Build()
			if err == nil {
				defer deps.Defer()
				if u := deps.Cfg.Site().URL; u != "" {
					siteURL = u
				}
			}
			fmt.Println(gotext.Get("Site: %s", siteURL))
		},
	}
}
