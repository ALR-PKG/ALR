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
	"log/slog"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"
)

// TODO: remove
//
// Deprecated: use "alr repo add <name> <url>"
func newLegacyAddRepoCmd() *cobra.Command {
	var name, url string

	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "addrepo",
		Short:   gotext.Get("Add a new repository"),
		Aliases: []string{"ar"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.Warn(gotext.Get(
				"This command is deprecated and would be removed in the future, use \"%s\" instead!",
				"alr repo add <name> <url>",
			))
			return runRepoAdd(cmd, name, url)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", gotext.Get("Name of the new repo"))
	cmd.Flags().StringVarP(&url, "url", "u", "", gotext.Get("URL of the new repo"))
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("url")

	addFlagCompletion(cmd)
	return cmd
}

// TODO: remove
//
// Deprecated: use "alr repo remove <name>"
func newLegacyRemoveRepoCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Hidden:  true,
		Use:     "removerepo",
		Short:   gotext.Get("Remove an existing repository"),
		Aliases: []string{"rr"},
		Args:    cobra.NoArgs,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return repoNameCompletion(cmd, args, toComplete)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			slog.Warn(gotext.Get(
				"This command is deprecated and would be removed in the future, use \"%s\" instead!",
				"alr repo remove <name>",
			))
			return runRepoRemove(cmd, name)
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", gotext.Get("Name of the repo to be deleted"))
	_ = cmd.MarkFlagRequired("name")

	addFlagCompletion(cmd)
	return cmd
}
