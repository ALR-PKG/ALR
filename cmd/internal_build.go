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
	"syscall"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/config"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/logger"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/translations"
)

func newInternalBuildCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_internal-safe-script-executor",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.SetupForGoPlugin()
			translations.Setup()

			slog.Debug("start _internal-safe-script-executor", "uid", syscall.Getuid(), "gid", syscall.Getgid())

			cfg := config.New()
			err := cfg.Load()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Error loading config"), err)
			}

			logger := hclog.New(&hclog.LoggerOptions{
				Name:        "plugin",
				Output:      os.Stderr,
				Level:       hclog.Debug,
				JSONFormat:  false,
				DisableTime: true,
			})

			plugin.Serve(&plugin.ServeConfig{
				HandshakeConfig: build.HandshakeConfig,
				Plugins: map[string]plugin.Plugin{
					"script-executor": &build.ScriptExecutorPlugin{
						Impl: build.NewLocalScriptExecutor(cfg),
					},
				},
				Logger: logger,
			})
			return nil
		},
	}
}
