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
	"os/exec"

	"github.com/hashicorp/go-plugin"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/ALR/ALR/internal/build"
	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/logger"
	"git.alr-pkg.ru/ALR/ALR/internal/translations"
)

func newInternalReposCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_internal-repos",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.SetupForGoPlugin()
			translations.Setup()

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			if os.Getuid() != 0 {
				if !deps.Cfg.UseRootCmd() {
					return fmt.Errorf("%s", gotext.Get("You need to be root to perform this action"))
				}
				executable, err := os.Executable()
				if err != nil {
					return fmt.Errorf("failed to get executable path: %w", err)
				}
				allArgs := append([]string{executable}, os.Args[1:]...)
				c := exec.Command(deps.Cfg.RootCmd(), allArgs...)
				c.Stdin = os.Stdin
				c.Stdout = os.Stdout
				c.Stderr = os.Stderr
				return c.Run()
			}

			deps, err = appbuilder.
				New(cmd.Context()).
				WithConfig().
				WithDB().
				WithReposNoPull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			pluginCfg := build.GetPluginServeCommonConfig()
			pluginCfg.Plugins = map[string]plugin.Plugin{
				"repos": &build.ReposExecutorPlugin{
					Impl: build.NewRepos(
						deps.Repos,
					),
				},
			}
			plugin.Serve(pluginCfg)
			return nil
		},
	}
}
