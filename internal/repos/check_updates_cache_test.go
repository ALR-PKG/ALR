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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/db"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

// Регрессия: проверка обновлений идёт по свежим данным сайта (живой RPC),
// а сборка — по локальному кэшу FindPkgs (TTL 1 час). Когда сайт уже
// знает о новом релизе, а кэш ещё нет, пакет собирался по протухшим
// метаданным с той же версией, и обновление становилось вечным no-op'ом.
// CheckUpdates обязан инвалидировать кэш обновляемых пакетов.
func TestCheckUpdatesInvalidatesStaleCache(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rpc/v1/repos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version":      1,
				"repositories": []map[string]string{{"name": "alr-default"}},
			})
		case "/rpc/v1/updates":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"version": 1,
				"updates": []map[string]string{
					{"name": "alr-git", "repo": "alr-default", "from_version": "628.5962b98-1.red80", "to_version": "628.5962b98-2.red80"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	ctx := context.Background()
	database := db.New(&rpcTestConfig{})
	require.NoError(t, database.Init(ctx))
	t.Cleanup(func() { database.Close() })

	// Протухшие записи кэша: сам пакет и подпакет с тем же basepkg_name
	stale := []alrsh.Package{
		{Name: "alr-git", Repository: "alr-default", Version: "628.5962b98", Release: 1, CachedAt: time.Now().Unix()},
		{Name: "alr-git-doc", BasePkgName: "alr-git", Repository: "alr-default", Version: "628.5962b98", Release: 1, CachedAt: time.Now().Unix()},
		// Другой репозиторий с тем же именем трогать нельзя
		{Name: "alr-git", Repository: "other-repo", Version: "1.0", Release: 1, CachedAt: time.Now().Unix()},
	}
	for _, pkg := range stale {
		require.NoError(t, database.InsertPackage(ctx, pkg))
	}

	rs := New(&rpcTestConfig{site: types.Site{URL: srv.URL, Timeout: 5}}, database)

	updates, err := rs.CheckUpdates(ctx, map[string]string{"alr-git+alr-default": "628.5962b98-1.red80"}, nil)
	require.NoError(t, err)
	require.Len(t, updates, 1)

	remaining, err := database.GetPkgs(ctx, "name = ? OR basepkg_name = ?", "alr-git", "alr-git")
	require.NoError(t, err)
	require.Len(t, remaining, 1)
	assert.Equal(t, "other-repo", remaining[0].Repository)
}
