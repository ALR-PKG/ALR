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
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/leonelquinteros/gotext"
	"github.com/spf13/cobra"

	appbuilder "git.alr-pkg.ru/ALR/ALR/internal/cliutils/app_builder"
	"git.alr-pkg.ru/ALR/ALR/internal/utils"
)

func execWithPrivileges(name string, args ...string) *exec.Cmd {
	isRoot := os.Geteuid() == 0
	isCI := os.Getenv("CI") == "true"

	if !isRoot && !isCI {
		allArgs := append([]string{name}, args...)
		return exec.Command("sudo", allArgs...)
	}
	return exec.Command(name, args...)
}

func newFixCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fix",
		Short: gotext.Get("Attempt to fix problems with ALR"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			cfg := deps.Cfg
			paths := cfg.GetPaths()

			slog.Info(gotext.Get("Clearing cache and temporary directories"))

			dir, err := os.Open(paths.CacheDir)
			if err != nil {
				if os.IsNotExist(err) {
					slog.Info(gotext.Get("Cache directory does not exist, will create it"))
				} else {
					return fmt.Errorf("%s: %w", gotext.Get("Unable to open cache directory"), err)
				}
			} else {
				defer dir.Close()

				entries, err := dir.Readdirnames(-1)
				if err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Unable to read cache directory contents"), err)
				}

				for _, entry := range entries {
					fullPath := filepath.Join(paths.CacheDir, entry)

					if err := makeWritableRecursive(fullPath); err != nil {
						slog.Debug("Failed to make path writable", "path", fullPath, "error", err)
					}

					err = os.RemoveAll(fullPath)
					if err != nil {
						slog.Warn(gotext.Get("Unable to remove cache item (%s) as current user, trying with sudo", entry))
						sudoCmd := execWithPrivileges("rm", "-rf", fullPath)
						if sudoErr := sudoCmd.Run(); sudoErr != nil {
							slog.Error(gotext.Get("Unable to remove cache item (%s)", entry), "error", err)
							continue
						}
					}
				}
			}

			slog.Info(gotext.Get("Clearing build directory"))
			for _, dirPath := range []string{paths.BuildDir, paths.PkgsDir} {
				if _, err := os.Stat(dirPath); err == nil {
					if err := os.RemoveAll(dirPath); err != nil {
						return fmt.Errorf("%s: %w", gotext.Get("Unable to remove build directory"), err)
					}
				}
				if err := os.MkdirAll(dirPath, 0o755); err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Unable to create build directory"), err)
				}
			}

			slog.Info(gotext.Get("Rebuilding cache"))
			slog.Info(gotext.Get("Creating cache directory"))
			err = utils.EnsureTempDirWithRootOwner(paths.CacheDir, 0o2775)
			if err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Unable to create new cache directory"), err)
			}

			deps, err = appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithReposForcePull().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			slog.Info(gotext.Get("Done"))
			return nil
		},
	}
}

func makeWritableRecursive(path string) error {
	return filepath.WalkDir(path, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		newMode := info.Mode() | 0o200
		if d.IsDir() {
			newMode |= 0o100
		}

		return os.Chmod(path, newMode)
	})
}
