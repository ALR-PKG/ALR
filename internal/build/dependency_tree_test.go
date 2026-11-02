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
	"reflect"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

// При обновлении выбор варианта не спрашивается: запрос уже содержит
// точное имя установленного пакета (zed-bin+repo), его и берём, даже
// если под тем же provides-именем нашлись другие варианты (zed-git).
func TestSelectPackageVariantUpgradeSkipsPrompt(t *testing.T) {
	pkgs := []alrsh.Package{
		{Name: "zed-git", Repository: "repo"},
		{Name: "zed-bin", Repository: "repo"},
	}

	got := selectPackageVariant(context.Background(), "zed-bin+repo", pkgs, "", true, true)

	if len(got) != 1 || got[0].Name != "zed-bin" {
		t.Fatalf("expected [zed-bin], got %v", got)
	}
}

// Без точного совпадения имени при обновлении берётся первый вариант
// (список уже отсортирован FindPkgs по релевантности)
func TestSelectPackageVariantUpgradeFallsBackToFirst(t *testing.T) {
	pkgs := []alrsh.Package{
		{Name: "zed-bin", Repository: "repo"},
		{Name: "zed-git", Repository: "repo"},
	}

	got := selectPackageVariant(context.Background(), "zed+repo", pkgs, "", true, true)

	if len(got) != 1 || got[0].Name != "zed-bin" {
		t.Fatalf("expected [zed-bin], got %v", got)
	}
}

func TestOptDepOwnersAccumulateWithoutDuplicates(t *testing.T) {
	tree := &UnifiedDependencyTree{OptDepOwners: map[string][]string{}}
	visited := map[string]bool{}

	addOptDepOwner(tree, visited, "opt: description", "pkg-a")
	addOptDepOwner(tree, visited, "opt: description", "pkg-b")
	addOptDepOwner(tree, visited, "opt: description", "pkg-b")

	if !reflect.DeepEqual(tree.AllOptDeps, []string{"opt: description"}) {
		t.Fatalf("unexpected dependencies: %v", tree.AllOptDeps)
	}
	wantOwners := []string{"pkg-a", "pkg-b"}
	if !reflect.DeepEqual(tree.OptDepOwners["opt: description"], wantOwners) {
		t.Fatalf("expected owners %v, got %v", wantOwners, tree.OptDepOwners["opt: description"])
	}
}

func TestFormatOptDepOptionShowsOwners(t *testing.T) {
	got := formatOptDepOption("opt: description", []string{"pkg-a", "pkg-b"})
	want := "opt: description (for pkg-a, pkg-b)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	if got := formatOptDepOption("opt: description", nil); got != "opt: description" {
		t.Fatalf("unexpected suffix without owners: %q", got)
	}
}
