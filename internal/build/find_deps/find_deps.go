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
	"context"
	"os/exec"

	"github.com/goreleaser/nfpm/v2"

	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type ProvReqFinder interface {
	FindProvides(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, skiplist []string) error
	FindRequires(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, skiplist []string) error
}

type ProvReqService struct {
	finder ProvReqFinder
}

func New(info *distro.OSRelease, pkgFormat string) *ProvReqService {
	s := &ProvReqService{
		finder: &EmptyFindProvReq{},
	}
	if pkgFormat == "rpm" {
		if info.ID == "altlinux" {
			s.finder = &ALTLinuxFindProvReq{}
		} else if _, err := exec.LookPath("/usr/lib/rpm/rpmdeps"); err == nil {
			s.finder = &FedoraFindProvReq{}
		}
	}
	return s
}

func (s *ProvReqService) FindProvides(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, skiplist []string) error {
	return s.finder.FindProvides(ctx, pkgInfo, dirs, skiplist)
}

func (s *ProvReqService) FindRequires(ctx context.Context, pkgInfo *nfpm.Info, dirs types.Directories, skiplist []string) error {
	return s.finder.FindRequires(ctx, pkgInfo, dirs, skiplist)
}
