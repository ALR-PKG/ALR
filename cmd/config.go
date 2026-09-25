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
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
)

type configKeyInfo struct {
	key  string
	desc string
}

func configKeyInfos() []configKeyInfo {
	return []configKeyInfo{
		{"rootCmd", gotext.Get("Command for privilege escalation (default: sudo)")},
		{"useRootCmd", gotext.Get("Run privileged actions via rootCmd (true/false)")},
		{"pagerStyle", gotext.Get("Pager style for viewing build scripts")},
		{"autoPull", gotext.Get("Automatically pull git repos before operations (true/false)")},
		{"logLevel", gotext.Get("Log level (debug, info, warn, error)")},
		{"ignorePkgUpdates", gotext.Get("Packages to exclude from update offers (comma-separated)")},
		{"updateSystemOnUpgrade", gotext.Get("Update system packages during upgrade (true/false)")},
		{"preferALRDeps", gotext.Get("Prefer ALR packages over system ones for dependencies (true/false)")},
		{"site.url", gotext.Get("ALR repository site URL")},
		{"site.timeout", gotext.Get("Site request timeout in seconds")},
		{"site.disableRPC", gotext.Get("Completely disable site RPC (true/false)")},
		{"site.preferRPC", gotext.Get("Prefer site RPC over local DB for search and listing (true/false)")},
	}
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: gotext.Get("Manage config"),
	}

	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigSetCmd(),
		newConfigGetCmd(),
		newConfigHelpCmd(),
	)

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: gotext.Get("Show config"),
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			content, err := deps.Cfg.ToYAML()
			if err != nil {
				return err
			}
			fmt.Println(content)
			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "set",
		Short:             gotext.Get("Set config value"),
		ValidArgsFunction: configKeyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return fmt.Errorf("%s", gotext.Get("missing args"))
			}

			key := args[0]
			value := args[1]

			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			switch key {
			case "rootCmd":
				deps.Cfg.User.SetRootCmd(value)
			case "useRootCmd":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetUseRootCmd(boolValue)
			case "pagerStyle":
				deps.Cfg.User.SetPagerStyle(value)
			case "autoPull":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetAutoPull(boolValue)
			case "logLevel":
				deps.Cfg.User.SetLogLevel(value)
			case "ignorePkgUpdates":
				var updates []string
				if value != "" {
					updates = strings.Split(value, ",")
					for i, update := range updates {
						updates[i] = strings.TrimSpace(update)
					}
				}
				deps.Cfg.User.SetIgnorePkgUpdates(updates)
			case "updateSystemOnUpgrade":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetUpdateSystemOnUpgrade(boolValue)
			case "preferALRDeps":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetPreferALRDeps(boolValue)
			case "site.url":
				deps.Cfg.User.SetSiteURL(value)
			case "site.timeout":
				intValue, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid integer value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetSiteTimeout(intValue)
			case "site.disableRPC":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetSiteDisableRPC(boolValue)
			case "site.preferRPC":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.User.SetSitePreferRPC(boolValue)
			case "repo", "repos":
				return fmt.Errorf("%s", gotext.Get("use 'repo add/remove' commands to manage repositories"))
			default:
				return fmt.Errorf("%s", gotext.Get("unknown config key: %s", key))
			}

			if err := deps.Cfg.User.Save(); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("failed to save config"), err)
			}

			fmt.Println(gotext.Get("Successfully set %s = %s", key, value))
			return nil
		},
	}
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "get",
		Short:             gotext.Get("Get config value"),
		ValidArgsFunction: configKeyCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := appbuilder.
				New(cmd.Context()).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			if len(args) == 0 {
				content, err := deps.Cfg.ToYAML()
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("failed to serialize config"), err)
				}
				fmt.Print(content)
				return nil
			}

			key := args[0]

			switch key {
			case "rootCmd":
				fmt.Println(deps.Cfg.RootCmd())
			case "useRootCmd":
				fmt.Println(deps.Cfg.UseRootCmd())
			case "pagerStyle":
				fmt.Println(deps.Cfg.PagerStyle())
			case "autoPull":
				fmt.Println(deps.Cfg.AutoPull())
			case "logLevel":
				fmt.Println(deps.Cfg.LogLevel())
			case "ignorePkgUpdates":
				updates := deps.Cfg.IgnorePkgUpdates()
				if len(updates) == 0 {
					fmt.Println("[]")
				} else {
					fmt.Println(strings.Join(updates, ", "))
				}
			case "updateSystemOnUpgrade":
				fmt.Println(deps.Cfg.UpdateSystemOnUpgrade())
			case "preferALRDeps":
				fmt.Println(deps.Cfg.PreferALRDeps())
			case "site.url":
				fmt.Println(deps.Cfg.Site().URL)
			case "site.timeout":
				fmt.Println(deps.Cfg.Site().Timeout)
			case "site.disableRPC":
				fmt.Println(deps.Cfg.Site().DisableRPC)
			case "site.preferRPC":
				fmt.Println(deps.Cfg.Site().PreferRPC)
			case "repo", "repos":
				repos := deps.Cfg.Repos()
				if len(repos) == 0 {
					fmt.Println("[]")
				} else {
					repoData, err := yaml.Marshal(repos)
					if err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("failed to serialize repos"), err)
					}
					fmt.Print(string(repoData))
				}
			default:
				return fmt.Errorf("%s", gotext.Get("unknown config key: %s", key))
			}

			return nil
		},
	}
}

func newConfigHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "help",
		Aliases: []string{"h"},
		Short:   gotext.Get("Shows a list of commands or help for one command"),
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				subCmd, _, err := cmd.Parent().Find(args)
				if err != nil {
					return err
				}
				return subCmd.Help()
			}
			return cmd.Parent().Help()
		},
	}
}

func configKeyCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var out []string
	for _, info := range configKeyInfos() {
		if toComplete == "" || strings.HasPrefix(info.key, toComplete) {
			out = append(out, info.key+"\t"+info.desc)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
