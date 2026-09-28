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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

// Регрессия: в nameToQuery без квалификатора репозитория условие OR не было
// в скобках, и дописанный findInCache "AND cached_at > ?" из-за приоритета
// операторов относился только к provides — протухшие записи, найденные по
// точному имени, считались свежими и никогда не обновлялись через RPC
func TestExpiredCacheEntryIsRefreshedViaRPC(t *testing.T) {
	fixture := []rpctestPackage{
		{Name: "oldpkg", Version: "2.0", Repo: "alr-repo"},
	}
	rs, database := newRPCTestRepos(t, fixture)

	// Протухшая запись в локальном кэше (старше rpcCacheTTL)
	stale := alrsh.Package{Name: "oldpkg", Repository: "alr-repo", Version: "1.0"}
	stale.CachedAt = time.Now().Add(-2 * time.Hour).Unix()
	require.NoError(t, database.InsertPackage(context.Background(), stale))

	found, notFound, err := rs.FindPkgs(context.Background(), []string{"oldpkg"})
	require.NoError(t, err)
	assert.Empty(t, notFound)
	require.Len(t, found["oldpkg"], 1)
	assert.Equal(t, "2.0", found["oldpkg"][0].Version,
		"протухший кэш должен обновиться через RPC, а не отдать старую версию")
}
