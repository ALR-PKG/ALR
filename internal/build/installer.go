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
	"fmt"
	"log/slog"
	"regexp"

	"git.alr-pkg.ru/xpamych/vercmp"
	"github.com/leonelquinteros/gotext"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/manager"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/overrides"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/depver"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
)

func NewInstaller(mgr manager.Manager) *Installer {
	return &Installer{
		mgr: mgr,
	}
}

type Installer struct{ mgr manager.Manager }

func (i *Installer) InstallLocal(ctx context.Context, paths []string, opts *manager.Opts) error {
	return i.mgr.InstallLocal(opts, paths...)
}

func (i *Installer) Install(ctx context.Context, pkgs []string, opts *manager.Opts) error {
	// Convert dependencies to manager-specific format
	converted := make([]string, len(pkgs))
	for idx, pkg := range pkgs {
		dep := depver.Parse(pkg)
		converted[idx] = dep.ForManager(i.mgr.Name())
	}

	return i.mgr.Install(opts, converted...)
}

func (i *Installer) Remove(ctx context.Context, pkgs []string, opts *manager.Opts) error {
	return i.mgr.Remove(opts, pkgs...)
}

func (i *Installer) RemoveAlreadyInstalled(ctx context.Context, pkgs []string) ([]string, error) {
	filteredPackages := []string{}

	for _, dep := range pkgs {
		parsed := depver.Parse(dep)

		// Check if package is installed
		installed, err := i.mgr.IsInstalled(parsed.Name)
		if err != nil {
			return nil, err
		}

		if !installed {
			filteredPackages = append(filteredPackages, dep)
			continue
		}

		// If there's a version constraint, check if installed version satisfies it
		if parsed.HasVersionConstraint() {
			installedVer, err := i.mgr.GetInstalledVersion(parsed.Name)
			if err != nil {
				return nil, err
			}

			if !parsed.Satisfies(installedVer) {
				// Installed version doesn't satisfy constraint - need to upgrade
				slog.Debug("installed version doesn't satisfy constraint",
					"package", parsed.Name,
					"required", dep,
					"installed", installedVer)
				filteredPackages = append(filteredPackages, dep)
			}
		}
	}

	return filteredPackages, nil
}

func (i *Installer) CheckVersionsAfterInstall(ctx context.Context, pkgs []string) error {
	for _, pkg := range pkgs {
		parsed := depver.Parse(pkg)
		if !parsed.HasVersionConstraint() {
			continue
		}

		installedVer, err := i.mgr.GetInstalledVersion(parsed.Name)
		if err != nil {
			slog.Warn(gotext.Get("Failed to get installed version"),
				"package", parsed.Name,
				"error", err)
			continue
		}

		if installedVer == "" {
			slog.Warn(gotext.Get("Package was not installed"),
				"package", parsed.Name)
			continue
		}

		if !parsed.Satisfies(installedVer) {
			slog.Warn(gotext.Get("Installed version doesn't satisfy requirement"),
				"package", parsed.Name,
				"required", pkg,
				"installed", installedVer)
		}
	}
	return nil
}

func (i *Installer) FilterPackagesByVersion(ctx context.Context, packages []alrsh.Package, osRelease *distro.OSRelease) ([]alrsh.Package, error) {
	installedPkgs, err := i.mgr.ListInstalled(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed packages: %w", err)
	}

	var filteredPackages []alrsh.Package

	for _, pkg := range packages {
		alrPkgName := fmt.Sprintf("%s+%s", pkg.Name, pkg.Repository)
		installedVer, isInstalled := installedPkgs[alrPkgName]

		if !isInstalled {
			filteredPackages = append(filteredPackages, pkg)
			continue
		}

		repoVer := pkg.Version
		releaseStr := overrides.ReleasePlatformSpecific(pkg.Release, osRelease)

		if pkg.Release != 0 && pkg.Epoch == 0 {
			repoVer = fmt.Sprintf("%s-%s", pkg.Version, releaseStr)
		} else if pkg.Release != 0 && pkg.Epoch != 0 {
			repoVer = fmt.Sprintf("%d:%s-%s", pkg.Epoch, pkg.Version, releaseStr)
		}

		// Нормализуем версии перед сравнением (убираем dist суффиксы)
		normalizedRepo := normalizeVersion(repoVer)
		normalizedInstalled := normalizeVersion(installedVer)
		cmp := vercmp.Compare(normalizedRepo, normalizedInstalled)
		slog.Debug("FilterPackagesByVersion: comparing versions",
			"package", alrPkgName,
			"installed", installedVer,
			"installedNorm", normalizedInstalled,
			"repo", repoVer,
			"repoNorm", normalizedRepo,
			"cmp", cmp)

		if cmp > 0 {
			slog.Info(gotext.Get("Package %s is installed with older version %s, will rebuild with version %s", alrPkgName, installedVer, repoVer))
			filteredPackages = append(filteredPackages, pkg)
		} else if cmp == 0 {
			slog.Info(gotext.Get("Package %s is already installed with version %s, skipping build", alrPkgName, installedVer))
		} else {
			slog.Info(gotext.Get("Package %s is installed with newer version %s (repo has %s), skipping build", alrPkgName, installedVer, repoVer))
		}
	}

	return filteredPackages, nil
}

// distSuffixRegex находит dist суффиксы в версии (.red80, .el9, .fc41, .alt1 и т.д.)
var distSuffixRegex = regexp.MustCompile(`^((?:\d+:)?\d[\d\.]*-\d+)(\.[a-z]+\d*)$`)

// normalizeVersion убирает dist суффикс из версии пакета
func normalizeVersion(version string) string {
	matches := distSuffixRegex.FindStringSubmatch(version)
	if len(matches) >= 2 {
		return matches[1]
	}
	return version
}
