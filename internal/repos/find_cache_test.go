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
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/db"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type cacheTestConfig struct{}

func (c *cacheTestConfig) GetPaths() *config.Paths {
	return &config.Paths{DBPath: ":memory:"}
}
func (c *cacheTestConfig) Repos() []types.Repo { return nil }
func (c *cacheTestConfig) Site() types.Site    { return types.Site{} }

func TestFindInCacheVersionConstraints(t *testing.T) {
	ctx := context.Background()
	database := db.New(&cacheTestConfig{})
	require.NoError(t, database.Init(ctx))
	defer database.Close()

	rs := New(&cacheTestConfig{}, database)

	now := time.Now().Unix()
	for i, v := range []string{"1.0", "2.0"} {
		err := database.InsertPackage(ctx, alrsh.Package{
			Name:       "foo",
			Repository: fmt.Sprintf("repo-%d", i),
			Version:    v,
			Release:    1,
			CachedAt:   now,
		})
		require.NoError(t, err)
	}

	// Без ограничения — оба пакета
	found, notFound := rs.findInCache(ctx, []string{"foo"})
	assert.Empty(t, notFound)
	assert.Len(t, found["foo"], 2)

	// Ограничение удовлетворено — только подходящая версия
	found, notFound = rs.findInCache(ctx, []string{"foo>=1.5"})
	assert.Empty(t, notFound)
	require.Len(t, found["foo>=1.5"], 1)
	assert.Equal(t, "2.0", found["foo>=1.5"][0].Version)

	// Ограничение не удовлетворено — пакет считается ненайденным
	found, notFound = rs.findInCache(ctx, []string{"foo>=3.0"})
	assert.Empty(t, found)
	assert.Equal(t, []string{"foo>=3.0"}, notFound)

	// Форма repo/pkg с ограничением
	found, notFound = rs.findInCache(ctx, []string{"repo-1/foo>=1.5"})
	assert.Empty(t, notFound)
	assert.Len(t, found["repo-1/foo>=1.5"], 1)
}
