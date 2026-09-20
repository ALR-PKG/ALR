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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

const testScript = `
name="somepkg"
version="1.0"
release=1

# Комментарий вне функций
build() {
	cd "$pkgname"
	make
}

package() {
	make install DESTDIR="$pkgdir"
}
`

func mustParseScript(t *testing.T, src string) *alrsh.ScriptFile {
	t.Helper()
	sf, err := alrsh.ReadFromIOReader(strings.NewReader(src), "alr.sh")
	require.NoError(t, err)
	return sf
}

func funcsKeyOf(t *testing.T, src string) string {
	t.Helper()
	return ScriptFuncsKey(mustParseScript(t, src))
}

func TestScriptFuncsKey(t *testing.T) {
	t.Run("stable for the same script", func(t *testing.T) {
		assert.Equal(t, funcsKeyOf(t, testScript), funcsKeyOf(t, testScript))
	})

	t.Run("changes when build() body changes", func(t *testing.T) {
		changed := strings.Replace(testScript, "\tmake\n", "\tmake -j$(nproc)\n", 1)
		assert.NotEqual(t, funcsKeyOf(t, testScript), funcsKeyOf(t, changed))
	})

	t.Run("changes when package() body changes", func(t *testing.T) {
		changed := strings.Replace(testScript, "DESTDIR=\"$pkgdir\"", "DESTDIR=\"$pkgdir\" PREFIX=/usr", 1)
		assert.NotEqual(t, funcsKeyOf(t, testScript), funcsKeyOf(t, changed))
	})

	t.Run("does not change on comments and whitespace outside functions", func(t *testing.T) {
		changed := "# Новый комментарий сверху\n\n" + testScript + "\n# И снизу\n"
		assert.Equal(t, funcsKeyOf(t, testScript), funcsKeyOf(t, changed))
	})

	t.Run("does not change on comments and formatting inside functions", func(t *testing.T) {
		changed := strings.Replace(
			testScript,
			"\tcd \"$pkgname\"\n\tmake\n",
			"\tcd \"$pkgname\" # переходим в каталог\n\t\tmake\n",
			1,
		)
		assert.Equal(t, funcsKeyOf(t, testScript), funcsKeyOf(t, changed))
	})

	t.Run("does not change when version() body changes", func(t *testing.T) {
		withVersion := testScript + `
version() {
	echo "2.0"
}
`
		changedVersion := testScript + `
version() {
	echo "3.0"
}
`
		assert.Equal(t, funcsKeyOf(t, withVersion), funcsKeyOf(t, changedVersion))
	})

	t.Run("brace and subshell forms hash differently", func(t *testing.T) {
		// subshell изолирует переменные/каталог — это семантически другая
		// функция, поэтому ключ обязан отличаться (принтер формы не сливает).
		subshell := strings.Replace(testScript, "build() {", "build() (", 1)
		subshell = strings.Replace(subshell, "\tmake\n}", "\tmake\n)", 1)
		assert.NotEqual(t, funcsKeyOf(t, testScript), funcsKeyOf(t, subshell))
	})

	t.Run("script without tracked functions", func(t *testing.T) {
		key := funcsKeyOf(t, "name=\"somepkg\"\nversion=\"1.0\"\n")
		assert.NotEmpty(t, key)
		assert.Equal(t, funcsKeyOf(t, "name=\"somepkg\"\n"), key)
	})
}

func newTestKeyPackage() *alrsh.Package {
	pkg := &alrsh.Package{
		Name:          "somepkg",
		BasePkgName:   "somepkg",
		Repository:    "test-repo",
		Version:       "1.0",
		Release:       1,
		Epoch:         0,
		Architectures: []string{"x86_64"},
		Provides:      []string{"somepkg"},
		Conflicts:     []string{"oldpkg"},
		Replaces:      []string{"olderpkg"},
		ExtraFiles:    []alrsh.ExtraFile{{Name: "somepkg.desktop", Path: "/usr/share/applications/somepkg.desktop"}},
	}
	pkg.Summary.SetResolved("summary")
	pkg.Description.SetResolved("description")
	pkg.Group.SetResolved("group")
	pkg.Homepage.SetResolved("https://example.com")
	pkg.Maintainer.SetResolved("maintainer")
	pkg.Depends.SetResolved([]string{"glibc"})
	pkg.BuildDepends.SetResolved([]string{"gcc"})
	pkg.OptDepends.SetResolved([]string{"optpkg"})
	pkg.Sources.SetResolved([]string{"https://example.com/src.tar.gz"})
	pkg.Checksums.SetResolved([]string{"deadbeef"})
	pkg.Backup.SetResolved([]string{"etc/somepkg.conf"})
	pkg.Scripts.SetResolved(alrsh.Scripts{PostInstall: "postinstall.sh"})
	pkg.AutoReq.SetResolved([]string{"yes"})
	pkg.AutoProv.SetResolved([]string{"no"})
	pkg.FireJailed.SetResolved(false)
	return pkg
}

func TestSemanticCacheKey(t *testing.T) {
	const funcsKey = "funcskey"

	t.Run("stable for the same package", func(t *testing.T) {
		assert.Equal(t,
			SemanticCacheKey(newTestKeyPackage(), funcsKey),
			SemanticCacheKey(newTestKeyPackage(), funcsKey),
		)
	})

	t.Run("changes with funcs key", func(t *testing.T) {
		assert.NotEqual(t,
			SemanticCacheKey(newTestKeyPackage(), funcsKey),
			SemanticCacheKey(newTestKeyPackage(), "otherfuncskey"),
		)
	})

	t.Run("changes on build-significant fields", func(t *testing.T) {
		base := SemanticCacheKey(newTestKeyPackage(), funcsKey)

		cases := map[string]func(*alrsh.Package){
			"Version":          func(p *alrsh.Package) { p.Version = "2.0" },
			"Release":          func(p *alrsh.Package) { p.Release = 2 },
			"Epoch":            func(p *alrsh.Package) { p.Epoch = 1 },
			"Architectures":    func(p *alrsh.Package) { p.Architectures = []string{"aarch64"} },
			"Provides":         func(p *alrsh.Package) { p.Provides = []string{"other"} },
			"Conflicts":        func(p *alrsh.Package) { p.Conflicts = []string{"other"} },
			"Replaces":         func(p *alrsh.Package) { p.Replaces = []string{"other"} },
			"Depends":          func(p *alrsh.Package) { p.Depends.SetResolved([]string{"musl"}) },
			"BuildDepends":     func(p *alrsh.Package) { p.BuildDepends.SetResolved([]string{"clang"}) },
			"OptDepends":       func(p *alrsh.Package) { p.OptDepends.SetResolved([]string{"otheropt"}) },
			"Sources":          func(p *alrsh.Package) { p.Sources.SetResolved([]string{"https://example.com/v2.tar.gz"}) },
			"Checksums":        func(p *alrsh.Package) { p.Checksums.SetResolved([]string{"cafe"}) },
			"Backup":           func(p *alrsh.Package) { p.Backup.SetResolved([]string{"etc/other.conf"}) },
			"Scripts":          func(p *alrsh.Package) { p.Scripts.SetResolved(alrsh.Scripts{PreInstall: "pre.sh"}) },
			"AutoReq":          func(p *alrsh.Package) { p.AutoReq.SetResolved([]string{"no"}) },
			"AutoProv":         func(p *alrsh.Package) { p.AutoProv.SetResolved([]string{"yes"}) },
			"AutoReqSkipList":  func(p *alrsh.Package) { p.AutoReqSkipList.SetResolved([]string{"libfoo"}) },
			"AutoProvSkipList": func(p *alrsh.Package) { p.AutoProvSkipList.SetResolved([]string{"libbar"}) },
			"FireJailed":       func(p *alrsh.Package) { p.FireJailed.SetResolved(true) },
			"FireJailProfiles": func(p *alrsh.Package) { p.FireJailProfiles.SetResolved(map[string]string{"somepkg": "default"}) },
			"ExtraFiles":       func(p *alrsh.Package) { p.ExtraFiles = []alrsh.ExtraFile{{Name: "other.desktop", Path: "/x"}} },
		}

		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				pkg := newTestKeyPackage()
				mutate(pkg)
				assert.NotEqual(t, base, SemanticCacheKey(pkg, funcsKey))
			})
		}
	})

	t.Run("does not change on metadata-only fields", func(t *testing.T) {
		base := SemanticCacheKey(newTestKeyPackage(), funcsKey)

		cases := map[string]func(*alrsh.Package){
			"Summary":     func(p *alrsh.Package) { p.Summary.SetResolved("other summary") },
			"Description": func(p *alrsh.Package) { p.Description.SetResolved("other description") },
			"Group":       func(p *alrsh.Package) { p.Group.SetResolved("other group") },
			"Homepage":    func(p *alrsh.Package) { p.Homepage.SetResolved("https://other.example.com") },
			"Maintainer":  func(p *alrsh.Package) { p.Maintainer.SetResolved("other maintainer") },
			"Licenses":    func(p *alrsh.Package) { p.Licenses = []string{"MIT"} },
			"Name":        func(p *alrsh.Package) { p.Name = "othername" },
			"Repository":  func(p *alrsh.Package) { p.Repository = "other-repo" },
			"CachedAt":    func(p *alrsh.Package) { p.CachedAt = 12345 },
			"AlrShSHA256": func(p *alrsh.Package) { p.AlrShSHA256 = "abc" },
		}

		for name, mutate := range cases {
			t.Run(name, func(t *testing.T) {
				pkg := newTestKeyPackage()
				mutate(pkg)
				assert.Equal(t, base, SemanticCacheKey(pkg, funcsKey))
			})
		}
	})
}
