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
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/ALR/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/logger"
	"git.alr-pkg.ru/ALR/ALR/internal/manager"
	"git.alr-pkg.ru/ALR/ALR/internal/translations"
)

func newInternalInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_internal-installer",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.SetupForGoPlugin()
			translations.Setup()

			ctx := cmd.Context()

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			logger := hclog.New(&hclog.LoggerOptions{
				Name:        "plugin",
				Output:      os.Stderr,
				Level:       hclog.Trace,
				JSONFormat:  true,
				DisableTime: true,
			})

			plugin.Serve(&plugin.ServeConfig{
				HandshakeConfig: build.HandshakeConfig,
				Plugins: map[string]plugin.Plugin{
					"installer": &build.InstallerExecutorPlugin{
						Impl: build.NewInstaller(
							manager.Detect(),
						),
					},
				},
				Logger: logger,
			})
			return nil
		},
	}
}
