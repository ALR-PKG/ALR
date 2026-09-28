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

package finddeps

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	"github.com/leonelquinteros/gotext"

	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type FedoraFindProvReq struct{}

func rpmFindDependenciesFedora(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, command string, args []string, updateFunc func(string)) error {
	if _, err := exec.LookPath(command); err != nil {
		slog.Info(gotext.Get("Command %s not found on the system", command))
		return nil
	}

	var paths []string
	for _, content := range pkgInfo.Contents {
		if content.Type != "dir" {
			paths = append(paths,
				path.Join(dirs.PkgDir, content.Destination),
			)
		}
	}

	if len(paths) == 0 {
		return nil
	}

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = bytes.NewBufferString(strings.Join(paths, "\n") + "\n")
	cmd.Env = append(cmd.Env,
		"RPM_BUILD_ROOT="+dirs.PkgDir,
	)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		slog.Error(stderr.String())
		return err
	}
	slog.Debug(stderr.String())

	dependencies := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, dep := range dependencies {
		if dep != "" {
			updateFunc(dep)
		}
	}

	return nil
}

func (o *FedoraFindProvReq) FindProvides(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, skiplist []string) error {
	args := []string{
		"--define=_use_internal_dependency_generator 1",
		"--provides",
	}
	if len(skiplist) > 0 {
		args = append(args, fmt.Sprintf(
			"--define=__provides_exclude_from %s",
			strings.Join(skiplist, "|"),
		))
	}
	return rpmFindDependenciesFedora(
		ctx,
		pkgInfo,
		dirs,
		"/usr/lib/rpm/rpmdeps",
		args,
		func(dep string) {
			slog.Info(gotext.Get("Provided dependency found: %s", dep))
			pkgInfo.Overridables.Provides = append(pkgInfo.Overridables.Provides, dep)
		})
}

func (o *FedoraFindProvReq) FindRequires(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, skiplist []string) error {
	args := []string{
		"--define=_use_internal_dependency_generator 1",
		"--requires",
	}
	if len(skiplist) > 0 {
		args = append(args, fmt.Sprintf(
			"--define=__requires_exclude_from %s",
			strings.Join(skiplist, "|"),
		))
	}
	return rpmFindDependenciesFedora(
		ctx,
		pkgInfo,
		dirs,
		"/usr/lib/rpm/rpmdeps",
		args,
		func(dep string) {
			slog.Info(gotext.Get("Required dependency found: %s", dep))
			pkgInfo.Overridables.Depends = append(pkgInfo.Overridables.Depends, dep)
		})
}
