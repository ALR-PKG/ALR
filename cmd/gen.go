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

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/gen"
)

func newGenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "generate",
		Short:   gotext.Get("Generate a ALR script from a template"),
		Aliases: []string{"gen"},
	}

	cmd.AddCommand(
		newGenPipCmd(),
		newGenAurCmd(),
	)

	return cmd
}

func newGenPipCmd() *cobra.Command {
	var (
		name        string
		version     string
		description string
	)

	c := &cobra.Command{
		Use:   "pip",
		Short: gotext.Get("Generate a ALR script for a pip module"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ = cmd.Flags().GetString("name")
			version, _ = cmd.Flags().GetString("version")
			description, _ = cmd.Flags().GetString("description")
			return gen.Pip(os.Stdout, gen.PipOptions{
				Name:        name,
				Version:     version,
				Description: description,
			})
		},
	}

	c.Flags().StringVarP(&name, "name", "n", "", "")
	c.Flags().StringVarP(&version, "version", "v", "", "")
	c.Flags().StringVarP(&description, "description", "d", "", "")

	_ = c.MarkFlagRequired("name")
	_ = c.MarkFlagRequired("version")

	return c
}

func newGenAurCmd() *cobra.Command {
	var (
		name    string
		version string
	)

	c := &cobra.Command{
		Use:   "aur",
		Short: gotext.Get("Generate a ALR script for an AUR package"),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, _ = cmd.Flags().GetString("name")
			version, _ = cmd.Flags().GetString("version")
			return gen.AUR(os.Stdout, gen.AUROptions{
				Name:    name,
				Version: version,
			})
		},
	}

	c.Flags().StringVarP(&name, "name", "n", "", gotext.Get("Name of the AUR package"))
	c.Flags().StringVarP(&version, "version", "v", "", gotext.Get("Version of the package (optional, uses latest if not specified)"))

	_ = c.MarkFlagRequired("name")

	return c
}
