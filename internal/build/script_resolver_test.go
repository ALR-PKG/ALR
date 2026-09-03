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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/config"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

type fakeResolverConfig struct {
	paths *config.Paths
	site  types.Site
}

func (f *fakeResolverConfig) GetPaths() *config.Paths { return f.paths }
func (f *fakeResolverConfig) PagerStyle() string      { return "" }
func (f *fakeResolverConfig) PreferALRDeps() bool     { return false }
func (f *fakeResolverConfig) Site() types.Site        { return f.site }

func newTestResolver(t *testing.T, site types.Site) (*ScriptResolver, string) {
	t.Helper()
	repodir := t.TempDir()
	cfg := &fakeResolverConfig{
		paths: &config.Paths{RepoDir: repodir},
		site:  site,
	}
	return &ScriptResolver{cfg}, repodir
}

func TestSafeJoin(t *testing.T) {
	base := t.TempDir()

	ok, err := safeJoin(base, "repo", "pkg", "alr.sh")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, "repo", "pkg", "alr.sh"), ok)

	for _, elems := range [][]string{
		{".."},
		{"repo", "..", "..", "etc"},
		{"/etc/passwd"},
		{"repo/../../etc", "passwd"},
		{""},
	} {
		_, err := safeJoin(base, elems...)
		assert.Error(t, err, "elems: %v", elems)
	}
}

func TestValidateDownloadURL(t *testing.T) {
	r, _ := newTestResolver(t, types.Site{
		URL:     "https://alr-pkg.ru",
		Mirrors: []string{"https://mirror.example.com"},
	})

	assert.NoError(t, r.validateDownloadURL("https://alr-pkg.ru/rpc/v1/file/alr-repo/pkg/alr.sh"))
	assert.NoError(t, r.validateDownloadURL("https://mirror.example.com/files/alr.sh"))

	// Другой хост
	assert.Error(t, r.validateDownloadURL("https://evil.example.com/alr.sh"))
	// Подмена схемы https -> http
	assert.Error(t, r.validateDownloadURL("http://alr-pkg.ru/alr.sh"))
	// Мусор вместо URL
	assert.Error(t, r.validateDownloadURL("://bad"))
	assert.Error(t, r.validateDownloadURL("alr.sh"))
}

func TestResolveScriptRejectsUntrustedHost(t *testing.T) {
	r, repodir := newTestResolver(t, types.Site{URL: "https://alr-pkg.ru"})

	pkg := &alrsh.Package{
		Name:       "somepkg",
		Repository: "alr-repo",
		AlrShURL:   "https://evil.example.com/alr.sh",
	}

	info := r.ResolveScript(context.Background(), pkg)

	// Скачивание не должно произойти, файл не должен появиться
	_, err := os.Stat(filepath.Join(repodir, "alr-repo", "somepkg", "alr.sh"))
	assert.True(t, os.IsNotExist(err))
	assert.Equal(t, filepath.Join(repodir, "alr-repo", "somepkg", "alr.sh"), info.Script)
}

func TestResolveScriptRejectsTraversal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("name='pwned'"))
	}))
	defer srv.Close()

	r, repodir := newTestResolver(t, types.Site{URL: srv.URL})

	pkg := &alrsh.Package{
		Name:       "../../../tmp/pwned",
		Repository: "alr-repo",
		AlrShURL:   srv.URL + "/alr.sh",
	}

	r.ResolveScript(context.Background(), pkg)

	_, err := os.Stat(filepath.Join(repodir, "..", "..", "..", "tmp", "pwned", "alr.sh"))
	assert.True(t, os.IsNotExist(err))
}

func TestResolveScriptDownloadsFromTrustedHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("name='somepkg'"))
	}))
	defer srv.Close()

	r, repodir := newTestResolver(t, types.Site{URL: srv.URL})

	pkg := &alrsh.Package{
		Name:       "somepkg",
		Repository: "alr-repo",
		AlrShURL:   srv.URL + "/alr.sh",
		ExtraFiles: []alrsh.ExtraFile{
			{Name: "postinstall.sh", Path: "somepkg/postinstall.sh", DownloadURL: srv.URL + "/postinstall.sh"},
			{Name: "evil.sh", Path: "../../evil.sh", DownloadURL: srv.URL + "/evil.sh"},
		},
	}

	r.ResolveScript(context.Background(), pkg)

	// alr.sh и доп. файл скачаны
	_, err := os.Stat(filepath.Join(repodir, "alr-repo", "somepkg", "alr.sh"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(repodir, "alr-repo", "somepkg", "postinstall.sh"))
	require.NoError(t, err)

	// Файл с traversal в пути не скачан
	_, err = os.Stat(filepath.Join(repodir, "evil.sh"))
	assert.True(t, os.IsNotExist(err))
}
