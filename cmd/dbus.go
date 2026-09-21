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
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	alrdbus "git.alr-pkg.ru/ALR/ALR/internal/dbus"
)

func newDBusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dbus",
		Short: gotext.Get("Manage D-Bus service"),
	}

	cmd.AddCommand(
		newDBusStatusCmd(),
		newDBusStartCmd(),
		newDBusStopCmd(),
		newDBusSearchCmd(),
		newDBusInstallCmd(),
		newDBusRemoveCmd(),
	)

	addFlagCompletion(cmd)
	return cmd
}

func newDBusStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: gotext.Get("Show D-Bus service status"),
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := dbus.ConnectSessionBus()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to connect to D-Bus"), err)
			}
			defer conn.Close()

			obj := conn.Object(alrdbus.DBusWellKnownName, alrdbus.DBusObjectPath)
			var version string
			err = obj.Call(alrdbus.ManagerInterfaceName+".GetVersion", 0).Store(&version)
			if err != nil {
				fmt.Println(gotext.Get("D-Bus service: not running"))
				return nil
			}

			fmt.Println(gotext.Get("D-Bus service: running"))
			fmt.Printf("Version: %s\n", version)

			return nil
		},
	}
}

func newDBusStartCmd() *cobra.Command {
	var session bool

	cmd := &cobra.Command{
		Use:   "start",
		Short: gotext.Get("Start D-Bus service"),
		RunE: func(cmd *cobra.Command, args []string) error {
			executable, err := os.Executable()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to start D-Bus service"), err)
			}

			c := exec.Command(executable, "_internal-dbus-service")
			if session {
				c.Args = append(c.Args, "--session")
			}

			if err := c.Start(); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to start D-Bus service"), err)
			}

			fmt.Println(gotext.Get("D-Bus service started"))
			return nil
		},
	}

	cmd.Flags().BoolVar(&session, "session", true, "Use session bus (default)")

	addFlagCompletion(cmd)
	return cmd
}

func newDBusStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: gotext.Get("Stop D-Bus service"),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := exec.Command("pkill", "-f", "alr _internal-dbus-service")
			if err := c.Run(); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to stop D-Bus service"), err)
			}

			fmt.Println(gotext.Get("D-Bus service stopped"))
			return nil
		},
	}
}

func newDBusSearchCmd() *cobra.Command {
	var (
		name        string
		description string
		repository  string
	)

	cmd := &cobra.Command{
		Use:     "search",
		Short:   gotext.Get("Search packages via D-Bus"),
		Aliases: []string{"s"},
		RunE: func(cmd *cobra.Command, args []string) error {
			conn, err := dbus.ConnectSessionBus()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to connect to D-Bus"), err)
			}
			defer conn.Close()

			filters := make(map[string]dbus.Variant)
			if cmd.Flags().Changed("name") {
				filters["name"] = dbus.MakeVariant(name)
			}
			if cmd.Flags().Changed("description") {
				filters["description"] = dbus.MakeVariant(description)
			}
			if cmd.Flags().Changed("repository") {
				filters["repository"] = dbus.MakeVariant(repository)
			}

			obj := conn.Object(alrdbus.DBusWellKnownName, alrdbus.DBusObjectPath)

			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			if query == "" {
				query = name
			}

			var results []alrdbus.PackageInfo
			err = obj.Call(alrdbus.ManagerInterfaceName+".SearchPackages", 0, query, filters).Store(&results)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Search failed"), err)
			}

			if len(results) == 0 {
				fmt.Println(gotext.Get("No packages found"))
				return nil
			}

			fmt.Printf("Found %d packages:\n", len(results))
			for _, pkg := range results {
				installed := ""
				if pkg.Installed {
					installed = " [installed]"
				}
				fmt.Printf("  %s/%s %s%s\n", pkg.Repository, pkg.Name, pkg.Version, installed)
				if pkg.Summary != "" {
					fmt.Printf("    %s\n", pkg.Summary)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", gotext.Get("Search by name"))
	cmd.Flags().StringVarP(&description, "description", "d", "", gotext.Get("Search by description"))
	cmd.Flags().StringVarP(&repository, "repository", "r", "", gotext.Get("Search by repository"))

	addFlagCompletion(cmd)
	return cmd
}

func newDBusInstallCmd() *cobra.Command {
	var (
		clean      bool
		noProgress bool
	)

	cmd := &cobra.Command{
		Use:     "install",
		Short:   gotext.Get("Install package via D-Bus"),
		Aliases: []string{"in"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("%s", gotext.Get("Package name required"))
			}

			pkgArg := args[0]

			var repo, name string
			if strings.Contains(pkgArg, "/") {
				parts := strings.SplitN(pkgArg, "/", 2)
				repo = parts[0]
				name = parts[1]
			} else {
				name = pkgArg
				repo = ""
			}

			conn, err := dbus.ConnectSessionBus()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to connect to D-Bus"), err)
			}
			defer conn.Close()

			obj := conn.Object(alrdbus.DBusWellKnownName, alrdbus.DBusObjectPath)

			var pkgPath dbus.ObjectPath
			err = obj.Call(alrdbus.ManagerInterfaceName+".GetPackage", 0, name, repo).Store(&pkgPath)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to get package"), err)
			}

			opts := make(map[string]dbus.Variant)
			opts["clean"] = dbus.MakeVariant(clean)
			opts["interactive"] = dbus.MakeVariant(interactive)

			pkgObj := conn.Object(alrdbus.DBusWellKnownName, pkgPath)
			var jobPath dbus.ObjectPath
			err = pkgObj.Call(alrdbus.PackageInterfaceName+".Install", 0, opts).Store(&jobPath)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to start installation"), err)
			}

			fmt.Printf("Installation started: %s\n", jobPath)

			if !noProgress {
				if err := watchJob(conn, jobPath); err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVarP(&clean, "clean", "c", false, gotext.Get("Build from scratch"))
	cmd.Flags().BoolVar(&noProgress, "no-progress", false, gotext.Get("Don't show progress"))

	addFlagCompletion(cmd)
	return cmd
}

func newDBusRemoveCmd() *cobra.Command {
	var noProgress bool

	cmd := &cobra.Command{
		Use:     "remove",
		Short:   gotext.Get("Remove package via D-Bus"),
		Aliases: []string{"rm"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("%s", gotext.Get("Package name required"))
			}

			pkgArg := args[0]

			var repo, name string
			if strings.Contains(pkgArg, "/") {
				parts := strings.SplitN(pkgArg, "/", 2)
				repo = parts[0]
				name = parts[1]
			} else {
				name = pkgArg
				repo = ""
			}

			conn, err := dbus.ConnectSessionBus()
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to connect to D-Bus"), err)
			}
			defer conn.Close()

			obj := conn.Object(alrdbus.DBusWellKnownName, alrdbus.DBusObjectPath)

			var pkgPath dbus.ObjectPath
			err = obj.Call(alrdbus.ManagerInterfaceName+".GetPackage", 0, name, repo).Store(&pkgPath)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to get package"), err)
			}

			opts := make(map[string]dbus.Variant)
			opts["interactive"] = dbus.MakeVariant(interactive)

			pkgObj := conn.Object(alrdbus.DBusWellKnownName, pkgPath)
			var jobPath dbus.ObjectPath
			err = pkgObj.Call(alrdbus.PackageInterfaceName+".Remove", 0, opts).Store(&jobPath)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Failed to start removal"), err)
			}

			fmt.Printf("Removal started: %s\n", jobPath)

			if !noProgress {
				if err := watchJob(conn, jobPath); err != nil {
					return err
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&noProgress, "no-progress", false, gotext.Get("Don't show progress"))

	addFlagCompletion(cmd)
	return cmd
}

func watchJob(conn *dbus.Conn, jobPath dbus.ObjectPath) error {
	jobObj := conn.Object(alrdbus.DBusWellKnownName, jobPath)

	var info alrdbus.JobInfo
	err := jobObj.Call(alrdbus.JobInterfaceName+".GetInfo", 0).Store(&info)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Failed to get job info"), err)
	}

	slog.Info("Job started", "type", info.Type, "package", info.PackageName)

	var success bool
	var errMsg string
	err = jobObj.Call(alrdbus.JobInterfaceName+".Wait", 0, int32(0)).Store(&success, &errMsg)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("Failed to wait for job"), err)
	}

	if success {
		fmt.Println(gotext.Get("Operation completed successfully"))
	} else {
		return fmt.Errorf("%s: %w", gotext.Get("Operation failed"), fmt.Errorf("%s", errMsg))
	}

	return nil
}
