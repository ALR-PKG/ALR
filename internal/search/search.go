// This file was originally part of the project "LURE - Linux User REpository", created by Elara Musayelyan.
// It has been modified as part of "ALR - Any Linux Repository" by the ALR Authors.
//
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

package search

import (
	"context"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

type PackagesProvider interface {
	GetPkgs(ctx context.Context, where string, args ...any) ([]alrsh.Package, error)
}

type Searcher struct {
	pp PackagesProvider
}

func New(pp PackagesProvider) *Searcher {
	return &Searcher{
		pp: pp,
	}
}

func (s *Searcher) Search(
	ctx context.Context,
	opts *SearchOptions,
) ([]alrsh.Package, error) {
	where, args := opts.WhereClause()
	packages, err := s.pp.GetPkgs(ctx, where, args...)
	return packages, err
}
