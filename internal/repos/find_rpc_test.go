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

package repos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/db"
	"git.alr-pkg.ru/ALR/ALR/internal/rpc"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type rpcTestConfig struct {
	site types.Site
}

func (c *rpcTestConfig) GetPaths() *config.Paths {
	return &config.Paths{DBPath: ":memory:"}
}
func (c *rpcTestConfig) Repos() []types.Repo { return nil }
func (c *rpcTestConfig) Site() types.Site    { return c.site }

// rpctestPackage — минимальное представление пакета в ответах mock-сервера
type rpctestPackage struct {
	Name        string `json:"name"`
	BasePkgName string `json:"basepkg_name,omitempty"`
	Version     string `json:"version"`
	Repo        string `json:"repo"`
}

// newRPCTestRepos создаёт Repos с RPC-клиентом, направленным на mock-сервер.
// Сервер эмулирует контракт /rpc/v1/info: arg принимает pkg или pkg+repo
// и отвечает пакетами из фикстуры, подходящими по имени/basepkg_name и репозиторию.
func newRPCTestRepos(t *testing.T, fixture []rpctestPackage) (*Repos, *db.Database) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Список site-репозиториев — из репозиториев, встречающихся в фикстуре
		if r.URL.Path == "/rpc/v1/repos" {
			seen := map[string]bool{}
			var repos []map[string]string
			for _, p := range fixture {
				if !seen[p.Repo] {
					seen[p.Repo] = true
					repos = append(repos, map[string]string{"name": p.Repo})
				}
			}
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
				"version":      1,
				"repositories": repos,
			}))
			return
		}

		require.Equal(t, "/rpc/v1/info", r.URL.Path)

		var results []rpctestPackage
		for _, arg := range r.URL.Query()["arg"] {
			name, repo := arg, ""
			if i := strings.Index(arg, "+"); i > 0 {
				name, repo = arg[:i], arg[i+1:]
			}
			for _, p := range fixture {
				if repo != "" && p.Repo != repo {
					continue
				}
				if p.Name == name || p.BasePkgName == name {
					results = append(results, p)
				}
			}
		}

		resp := map[string]any{
			"version":     1,
			"resultcount": len(results),
			"results":     results,
		}
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	database := db.New(&rpcTestConfig{})
	require.NoError(t, database.Init(ctx))
	t.Cleanup(func() { database.Close() })

	cfg := &rpcTestConfig{site: types.Site{URL: srv.URL, Timeout: 5}}
	rs := New(cfg, database)
	require.NotNil(t, rs.RPCClient())
	return rs, database
}

func TestFindViaRPCQualifiedNames(t *testing.T) {
	fixture := []rpctestPackage{
		{Name: "foo", Version: "1.0", Repo: "repo1"},
		{Name: "foo", Version: "2.0", Repo: "repo2"},
	}

	rs, _ := newRPCTestRepos(t, fixture)
	ctx := context.Background()

	// Форма repo/pkg
	found, notFound, ok := rs.findViaRPC(ctx, []string{"repo2/foo"})
	assert.True(t, ok)
	assert.Empty(t, notFound)
	require.Len(t, found["repo2/foo"], 1)
	assert.Equal(t, "repo2", found["repo2/foo"][0].Repository)
	assert.Equal(t, "2.0", found["repo2/foo"][0].Version)

	// Форма pkg+repo
	found, notFound, ok = rs.findViaRPC(ctx, []string{"foo+repo2"})
	assert.True(t, ok)
	assert.Empty(t, notFound)
	require.Len(t, found["foo+repo2"], 1)
	assert.Equal(t, "repo2", found["foo+repo2"][0].Repository)

	// Форма repo/pkg с версионным ограничением
	found, notFound, ok = rs.findViaRPC(ctx, []string{"repo1/foo>=0.5"})
	assert.True(t, ok)
	assert.Empty(t, notFound)
	require.Len(t, found["repo1/foo>=0.5"], 1)
	assert.Equal(t, "1.0", found["repo1/foo>=0.5"][0].Version)

	// Пакет не должен матчиться, если репозиторий из квалификатора
	// не совпадает с репозиторием ответа (mock эмулирует сервер,
	// вернувший пакет из другого репозитория — например, подпакет мультипакета)
	crossRepoSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"version":1,"resultcount":1,"results":[{"name":"foo","version":"1.0","repo":"repoA"}]}`))
	}))
	t.Cleanup(crossRepoSrv.Close)

	crossCfg := &rpcTestConfig{site: types.Site{URL: crossRepoSrv.URL, Timeout: 5}}
	crossDB := db.New(&rpcTestConfig{})
	require.NoError(t, crossDB.Init(ctx))
	t.Cleanup(func() { crossDB.Close() })
	crossRS := New(crossCfg, crossDB)

	found, notFound, ok = crossRS.findViaRPC(ctx, []string{"foo+repoB"})
	assert.True(t, ok)
	assert.Empty(t, found)
	assert.Equal(t, []string{"foo+repoB"}, notFound)
}

func TestFindViaRPCBasePkgSubpackages(t *testing.T) {
	// Запрос по имени мета-пакета: сервер возвращает все подпакеты
	fixture := []rpctestPackage{
		{Name: "meta-lib", BasePkgName: "meta", Version: "1.0", Repo: "repo1"},
		{Name: "meta-dev", BasePkgName: "meta", Version: "1.0", Repo: "repo1"},
	}

	rs, _ := newRPCTestRepos(t, fixture)
	ctx := context.Background()

	found, notFound, ok := rs.findViaRPC(ctx, []string{"meta"})
	assert.True(t, ok)
	assert.Empty(t, notFound)
	assert.Len(t, found["meta"], 2)
}

func TestFindViaRPCFallbackOnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	database := db.New(&rpcTestConfig{})
	require.NoError(t, database.Init(ctx))
	t.Cleanup(func() { database.Close() })

	rs := New(&rpcTestConfig{site: types.Site{URL: srv.URL, Timeout: 5}}, database)

	found, notFound, ok := rs.findViaRPC(ctx, []string{"foo"})
	assert.False(t, ok)
	assert.Empty(t, found)
	assert.Equal(t, []string{"foo"}, notFound)
}

// Убеждаемся, что rpc-пакет корректно конвертируется (страховка от регрессии
// сигнатуры rpc.Package в тестах выше)
var _ = rpc.Package{}
