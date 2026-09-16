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

package build

import (
	"context"

	"git.alr-pkg.ru/ALR/ALR/internal/manager"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

//go:generate go run ../../generators/plugin-generator InstallerExecutor ScriptExecutor ReposExecutor

// The Executors interfaces must use context.Context as the first parameter,
// because the plugin-generator cannot generate code without it.

type InstallerExecutor interface {
	InstallLocal(ctx context.Context, paths []string, opts *manager.Opts) error
	Install(ctx context.Context, pkgs []string, opts *manager.Opts) error
	Remove(ctx context.Context, pkgs []string, opts *manager.Opts) error
	RemoveAlreadyInstalled(ctx context.Context, pkgs []string) ([]string, error)
	FilterPackagesByVersion(ctx context.Context, packages []alrsh.Package, osRelease *distro.OSRelease) ([]alrsh.Package, error)
	CheckVersionsAfterInstall(ctx context.Context, pkgs []string) error
}

type ScriptExecutor interface {
	ReadScript(ctx context.Context, scriptPath string) (*alrsh.ScriptFile, error)
	ExecuteFirstPass(ctx context.Context, input *BuildInput, sf *alrsh.ScriptFile) (string, []*alrsh.Package, error)
	PrepareDirs(
		ctx context.Context,
		input *BuildInput,
		basePkg string,
	) error
	ExecuteSecondPass(
		ctx context.Context,
		input *BuildInput,
		sf *alrsh.ScriptFile,
		varsOfPackages []*alrsh.Package,
		repoDeps []string,
		builtDeps []*BuiltDep,
		basePkg string,
	) ([]*BuiltDep, error)
}

type ReposExecutor interface {
	PullOneAndUpdateFromConfig(ctx context.Context, repo *types.Repo) (types.Repo, error)
}
