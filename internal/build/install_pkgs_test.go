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

package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/manager"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type installTestFinder struct {
	packages map[string][]alrsh.Package
	err      error
}

func (f installTestFinder) FindPkgs(_ context.Context, names []string) (map[string][]alrsh.Package, []string, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	found := make(map[string][]alrsh.Package)
	var notFound []string
	for _, name := range names {
		if packages, ok := f.packages[name]; ok {
			found[name] = packages
		} else {
			notFound = append(notFound, name)
		}
	}
	return found, notFound, nil
}

type installCall struct {
	kind      string
	pkgs      []string
	paths     []string
	noConfirm bool
}

type installTestInstaller struct {
	calls                     []installCall
	installErr                error
	installLocalErr           error
	removeAlreadyInstalled    []string
	removeAlreadyInstalledErr error
	filter                    []alrsh.Package
	filterErr                 error
}

func (i *installTestInstaller) InstallLocal(_ context.Context, paths []string, opts *manager.Opts) error {
	i.calls = append(i.calls, installCall{kind: "local", paths: paths, noConfirm: opts.NoConfirm})
	return i.installLocalErr
}

func (i *installTestInstaller) Install(_ context.Context, pkgs []string, opts *manager.Opts) error {
	i.calls = append(i.calls, installCall{kind: "system", pkgs: pkgs, noConfirm: opts.NoConfirm})
	return i.installErr
}

func (i *installTestInstaller) Remove(_ context.Context, pkgs []string, opts *manager.Opts) error {
	i.calls = append(i.calls, installCall{kind: "remove", pkgs: pkgs, noConfirm: opts.NoConfirm})
	return nil
}

func (i *installTestInstaller) RemoveAlreadyInstalled(_ context.Context, pkgs []string) ([]string, error) {
	if i.removeAlreadyInstalledErr != nil {
		return nil, i.removeAlreadyInstalledErr
	}
	if i.removeAlreadyInstalled != nil {
		return i.removeAlreadyInstalled, nil
	}
	return pkgs, nil
}

func (i *installTestInstaller) FilterPackagesByVersion(_ context.Context, packages []alrsh.Package, _ *distro.OSRelease) ([]alrsh.Package, error) {
	if i.filterErr != nil {
		return nil, i.filterErr
	}
	if i.filter != nil {
		return i.filter, nil
	}
	return packages, nil
}

func (*installTestInstaller) CheckVersionsAfterInstall(context.Context, []string) error {
	return nil
}

func installTestInput(interactive bool) *BuildArgs {
	return &BuildArgs{
		Opts:       &types.BuildOpts{Interactive: interactive},
		Info:       &distro.OSRelease{},
		PkgFormat_: "rpm",
	}
}

func TestInstallPkgsEmptyInput(t *testing.T) {
	builder := &Builder{}

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), nil)

	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestInstallPkgsDependencyTreeError(t *testing.T) {
	findErr := errors.New("repository unavailable")
	builder := &Builder{repos: installTestFinder{err: findErr}}

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, findErr)
	assert.Contains(t, err.Error(), "failed to resolve dependency tree")
}

func TestInstallPkgsSystemDependencyError(t *testing.T) {
	app := alrsh.Package{Name: "app", Version: "1.0"}
	app.Depends.SetResolved([]string{"system-lib"})
	installer := &installTestInstaller{installErr: errors.New("install failed")}
	builder := &Builder{
		repos: installTestFinder{packages: map[string][]alrsh.Package{
			"app": {app},
		}},
		installerExecutor: installer,
	}

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install system dependencies")
	require.Len(t, installer.calls, 1)
	assert.Equal(t, installCall{kind: "system", pkgs: []string{"system-lib"}, noConfirm: true}, installer.calls[0])
}

type installTestResolver struct{}

func (installTestResolver) ResolveScript(_ context.Context, pkg *alrsh.Package) *ScriptInfo {
	return &ScriptInfo{Script: pkg.Name + ".alr.sh", Repository: pkg.Repository}
}

type installTestCache struct {
	paths map[string]string
	err   error
}

func (c installTestCache) CheckForBuiltPackage(_ context.Context, _ *BuildInput, pkg *alrsh.Package, _ string) (string, bool, error) {
	if c.err != nil {
		return "", false, c.err
	}
	path, ok := c.paths[pkg.Name]
	return path, ok, nil
}

type installTestScripts struct {
	firstPassErr error
}

func (installTestScripts) ReadScript(_ context.Context, scriptPath string) (*alrsh.ScriptFile, error) {
	return alrsh.ReadFromIOReader(strings.NewReader(""), scriptPath)
}

func (s installTestScripts) ExecuteFirstPass(context.Context, *BuildInput, *alrsh.ScriptFile) (string, []*alrsh.Package, error) {
	return "", nil, s.firstPassErr
}

func (installTestScripts) PrepareDirs(context.Context, *BuildInput, string) error {
	return nil
}

func (installTestScripts) ExecuteSecondPass(context.Context, *BuildInput, *alrsh.ScriptFile, []*alrsh.Package, []string, []*BuiltDep, string) ([]*BuiltDep, error) {
	return nil, nil
}

func cachedInstallFixture(t *testing.T, appDepends bool, filter []alrsh.Package) (*Builder, *installTestInstaller, string, string) {
	t.Helper()
	dir := t.TempDir()
	libPath := filepath.Join(dir, "lib.rpm")
	appPath := filepath.Join(dir, "app.rpm")
	require.NoError(t, os.WriteFile(libPath, []byte("lib"), 0o600))
	require.NoError(t, os.WriteFile(appPath, []byte("app"), 0o600))

	lib := alrsh.Package{Name: "lib", Version: "1.0"}
	app := alrsh.Package{Name: "app", Version: "1.0"}
	if appDepends {
		app.Depends.SetResolved([]string{"lib"})
	}
	installer := &installTestInstaller{filter: filter}
	builder := &Builder{
		repos: installTestFinder{packages: map[string][]alrsh.Package{
			"app": {app},
			"lib": {lib},
		}},
		scriptResolver:    installTestResolver{},
		scriptExecutor:    installTestScripts{},
		cacheExecutor:     installTestCache{paths: map[string]string{"lib": libPath, "app": appPath}},
		installerExecutor: installer,
	}
	return builder, installer, libPath, appPath
}

func TestInstallPkgsInstallsDependencyBeforeTarget(t *testing.T) {
	builder, installer, libPath, appPath := cachedInstallFixture(t, true, nil)

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "lib", result[0].Name)
	assert.Equal(t, "app", result[1].Name)
	require.Len(t, installer.calls, 2)
	assert.Equal(t, installCall{kind: "local", paths: []string{libPath}, noConfirm: true}, installer.calls[0])
	assert.Equal(t, installCall{kind: "local", paths: []string{appPath}, noConfirm: true}, installer.calls[1])
}

func TestInstallPkgsSkipsInstalledDependency(t *testing.T) {
	builder, installer, _, appPath := cachedInstallFixture(t, true, []alrsh.Package{})

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, installer.calls, 1)
	assert.Equal(t, installCall{kind: "local", paths: []string{appPath}, noConfirm: true}, installer.calls[0])
}

func TestInstallPkgsCachedTargetIsReturned(t *testing.T) {
	builder, installer, _, appPath := cachedInstallFixture(t, false, nil)

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, &BuiltDep{Name: "app", Path: appPath}, result[0])
	assert.Equal(t, []string{appPath}, installer.calls[0].paths)
}

func TestInstallPkgsTargetInstallError(t *testing.T) {
	builder, installer, _, _ := cachedInstallFixture(t, false, nil)
	installer.installLocalErr = errors.New("local install failed")

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, installer.installLocalErr)
}

func TestInstallPkgsNonInteractiveSkipsOptionalDependencies(t *testing.T) {
	builder, installer, _, appPath := cachedInstallFixture(t, false, nil)
	app := builder.repos.(installTestFinder).packages["app"][0]
	app.OptDepends.SetResolved([]string{"optional: feature"})
	builder.repos = installTestFinder{packages: map[string][]alrsh.Package{"app": {app}}}

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, installer.calls, 1)
	assert.Equal(t, []string{appPath}, installer.calls[0].paths)
}

func TestInstallPkgsFilterError(t *testing.T) {
	builder, _, _, _ := cachedInstallFixture(t, false, nil)
	filterErr := errors.New("version check failed")
	builder.installerExecutor.(*installTestInstaller).filterErr = filterErr

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, filterErr)
	assert.Contains(t, err.Error(), "failed to filter package app")
}

func TestInstallOptDepsInstalledCheckError(t *testing.T) {
	checkErr := errors.New("installed check failed")
	installer := &installTestInstaller{removeAlreadyInstalledErr: checkErr}
	builder := &Builder{installerExecutor: installer}

	result, err := builder.installOptDeps(context.Background(), installTestInput(false), []string{"optional"}, nil)

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, checkErr)
	assert.Empty(t, installer.calls)
}

func TestInstallPkgsCleanSkipsCache(t *testing.T) {
	builder, _, _, _ := cachedInstallFixture(t, false, nil)
	buildErr := errors.New("clean build reached")
	builder.scriptExecutor = installTestScripts{firstPassErr: buildErr}
	input := installTestInput(false)
	input.Opts.Clean = true

	result, err := builder.InstallPkgs(context.Background(), input, []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, buildErr)
}

func TestInstallPkgsCacheError(t *testing.T) {
	builder, _, _, _ := cachedInstallFixture(t, false, nil)
	cacheErr := errors.New("cache metadata failed")
	builder.cacheExecutor = installTestCache{err: cacheErr}

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.ErrorIs(t, err, cacheErr)
	assert.Contains(t, err.Error(), "failed to check cache")
}

func TestInstallPkgsDependencyInstallErrorStopsBeforeTarget(t *testing.T) {
	builder, installer, libPath, _ := cachedInstallFixture(t, true, nil)
	installer.installLocalErr = errors.New("dependency install failed")

	result, err := builder.InstallPkgs(context.Background(), installTestInput(false), []string{"app"})

	assert.Nil(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to install cached lib")
	require.Len(t, installer.calls, 1)
	assert.Equal(t, []string{libPath}, installer.calls[0].paths)
}
