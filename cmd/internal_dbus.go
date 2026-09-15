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

	alrdbus "git.alr-pkg.ru/ALR/ALR/internal/dbus"
	"git.alr-pkg.ru/ALR/ALR/internal/logger"
	"git.alr-pkg.ru/ALR/ALR/internal/translations"
)

func newInternalDBusServiceCmd() *cobra.Command {
	var session bool

	cmd := &cobra.Command{
		Use:    "_internal-dbus-service",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger.SetupForGoPlugin()
			translations.Setup()

			session, _ = cmd.Flags().GetBool("session")

			slog.Info("Starting ALR D-Bus service", "session_bus", session)

			service := alrdbus.NewService()
			if err := service.Run(session); err != nil {
				slog.Error("D-Bus service failed", "err", err)
				return fmt.Errorf("%s: %w", gotext.Get("D-Bus service failed"), err)
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&session, "session", "s", false, "Use session bus instead of system bus")

	addFlagCompletion(cmd)
	return cmd
}
