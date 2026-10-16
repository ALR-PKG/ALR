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
