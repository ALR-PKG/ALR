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

package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v2"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"
)

func ConfigCmd() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: gotext.Get("Manage config"),
		Subcommands: []*cli.Command{
			ShowCmd(),
			SetConfig(),
			GetConfig(),
			ConfigHelpCmd(),
		},
	}
}

func ConfigHelpCmd() *cli.Command {
	return &cli.Command{
		Name:      "help",
		Aliases:   []string{"h"},
		Usage:     gotext.Get("Shows a list of commands or help for one command"),
		ArgsUsage: "[command]",
		Action: func(cCtx *cli.Context) error {
			args := cCtx.Args()
			if args.Present() {
				return cli.ShowCommandHelp(cCtx, args.First())
			}
			cli.ShowSubcommandHelp(cCtx)
			return nil
		},
	}
}

func ShowCmd() *cli.Command {
	return &cli.Command{
		Name:  "show",
		Usage: gotext.Get("Show config"),
		Action: func(c *cli.Context) error {
			deps, err := appbuilder.
				New(c.Context).
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

var configKeys = []string{
	"rootCmd",
	"useRootCmd",
	"pagerStyle",
	"autoPull",
	"logLevel",
	"ignorePkgUpdates",
	"updateSystemOnUpgrade",
	"preferALRDeps",
}

func SetConfig() *cli.Command {
	return &cli.Command{
		Name:      "set",
		Usage:     gotext.Get("Set config value"),
		ArgsUsage: gotext.Get("<key> <value>"),
		BashComplete: cliutils.BashCompleteWithError(func(c *cli.Context) error {
			if cliutils.MaybePrintFlags(c) {
				return nil
			}
			if c.Args().Len() > 0 {
				return nil
			}

			prefix := cliutils.CompletionPrefix(c)
			for _, key := range configKeys {
				if prefix == "" || strings.HasPrefix(key, prefix) {
					cliutils.PrintCompletionItem(key, false)
				}
			}
			return nil
		}),
		Action: utils.RootNeededAction(func(c *cli.Context) error {
			if c.Args().Len() < 2 {
				return cliutils.FormatCliExit("missing args", nil)
			}

			key := c.Args().Get(0)
			value := c.Args().Get(1)

			deps, err := appbuilder.
				New(c.Context).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			switch key {
			case "rootCmd":
				deps.Cfg.System.SetRootCmd(value)
			case "useRootCmd":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.System.SetUseRootCmd(boolValue)
			case "pagerStyle":
				deps.Cfg.System.SetPagerStyle(value)
			case "autoPull":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.System.SetAutoPull(boolValue)
			case "logLevel":
				deps.Cfg.System.SetLogLevel(value)
			case "ignorePkgUpdates":
				var updates []string
				if value != "" {
					updates = strings.Split(value, ",")
					for i, update := range updates {
						updates[i] = strings.TrimSpace(update)
					}
				}
				deps.Cfg.System.SetIgnorePkgUpdates(updates)
			case "updateSystemOnUpgrade":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.System.SetUpdateSystemOnUpgrade(boolValue)
			case "preferALRDeps":
				boolValue, err := strconv.ParseBool(value)
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("invalid boolean value for %s: %s", key, value), err)
				}
				deps.Cfg.System.SetPreferALRDeps(boolValue)
			case "repo", "repos":
				return cliutils.FormatCliExit(gotext.Get("use 'repo add/remove' commands to manage repositories"), nil)
			default:
				return cliutils.FormatCliExit(gotext.Get("unknown config key: %s", key), nil)
			}

			if err := deps.Cfg.System.Save(); err != nil {
				return cliutils.FormatCliExit(gotext.Get("failed to save config"), err)
			}

			fmt.Println(gotext.Get("Successfully set %s = %s", key, value))
			return nil
		}),
	}
}

func GetConfig() *cli.Command {
	return &cli.Command{
		Name:      "get",
		Usage:     gotext.Get("Get config value"),
		ArgsUsage: gotext.Get("<key>"),
		BashComplete: cliutils.BashCompleteWithError(func(c *cli.Context) error {
			if cliutils.MaybePrintFlags(c) {
				return nil
			}

			prefix := cliutils.CompletionPrefix(c)
			for _, key := range configKeys {
				if prefix == "" || strings.HasPrefix(key, prefix) {
					cliutils.PrintCompletionItem(key, false)
				}
			}
			return nil
		}),
		Action: func(c *cli.Context) error {
			deps, err := appbuilder.
				New(c.Context).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			if c.Args().Len() == 0 {
				content, err := deps.Cfg.ToYAML()
				if err != nil {
					return cliutils.FormatCliExit("failed to serialize config", err)
				}
				fmt.Print(content)
				return nil
			}

			key := c.Args().Get(0)

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
			case "repo", "repos":
				repos := deps.Cfg.Repos()
				if len(repos) == 0 {
					fmt.Println("[]")
				} else {
					repoData, err := yaml.Marshal(repos)
					if err != nil {
						return cliutils.FormatCliExit("failed to serialize repos", err)
					}
					fmt.Print(string(repoData))
				}
			default:
				return cliutils.FormatCliExit(gotext.Get("unknown config key: %s", key), nil)
			}

			return nil
		},
	}
}
