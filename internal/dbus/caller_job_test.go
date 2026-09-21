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

package dbus

import (
	"errors"
	"os/user"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

// Тесты подменяют lookupUser и не параллельны: подмена глобальна для процесса.

func TestCallerIdentity(t *testing.T) {
	t.Run("gid from passwd entry", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: "1000", Gid: "1001", Username: "alice", HomeDir: "/home/alice"}, nil
		})

		home, username, gid := callerIdentity(1000)
		assert.Equal(t, "/home/alice", home)
		assert.Equal(t, "alice", username)
		assert.Equal(t, uint32(1001), gid)
	})

	t.Run("lookup error falls back to nobody", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return nil, errors.New("no such user")
		})

		home, username, gid := callerIdentity(1000)
		assert.Equal(t, systemCacheRoot, home)
		assert.Equal(t, nobodyUsername, username)
		assert.Equal(t, uint32(nobodyUID), gid)
	})

	t.Run("empty home dir falls back to nobody", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: "1000", Gid: "1000", Username: "ghost", HomeDir: ""}, nil
		})

		home, username, gid := callerIdentity(1000)
		assert.Equal(t, systemCacheRoot, home)
		assert.Equal(t, nobodyUsername, username)
		assert.Equal(t, uint32(nobodyUID), gid)
	})
}

type fakeJobConfig struct {
	paths *config.Paths
}

func (f *fakeJobConfig) GetPaths() *config.Paths { return f.paths }
func (f *fakeJobConfig) PagerStyle() string      { return "native" }
func (f *fakeJobConfig) PreferALRDeps() bool     { return false }
func (f *fakeJobConfig) Site() types.Site        { return types.Site{} }

func TestCallerJobConfig(t *testing.T) {
	stubLookupUser(t, lookupAlice)

	orig := &fakeJobConfig{paths: &config.Paths{
		CacheDir: "/var/cache/alr",
		RepoDir:  "/var/cache/alr/repo",
		BuildDir: "/root/.cache/alr/build",
		PkgsDir:  "/root/.cache/alr/pkgs",
		DBPath:   "/var/cache/alr/alr.db",
	}}

	jobCfg := callerJobConfig(orig, 1000)

	// BuildDir/PkgsDir подменены на XDG-кэш вызвавшего
	assert.Equal(t, "/home/alice/.cache/alr/build", jobCfg.GetPaths().BuildDir)
	assert.Equal(t, "/home/alice/.cache/alr/pkgs", jobCfg.GetPaths().PkgsDir)

	// Остальные пути сохранены
	assert.Equal(t, "/var/cache/alr", jobCfg.GetPaths().CacheDir)
	assert.Equal(t, "/var/cache/alr/repo", jobCfg.GetPaths().RepoDir)
	assert.Equal(t, "/var/cache/alr/alr.db", jobCfg.GetPaths().DBPath)

	// Остальной конфиг делегируется оригиналу
	assert.Equal(t, "native", jobCfg.PagerStyle())
	assert.False(t, jobCfg.PreferALRDeps())

	// Общий конфиг демона НЕ мутирован
	assert.Equal(t, "/root/.cache/alr/build", orig.GetPaths().BuildDir)
	assert.Equal(t, "/root/.cache/alr/pkgs", orig.GetPaths().PkgsDir)
}

func TestCallerJobConfigNobody(t *testing.T) {
	stubLookupUser(t, lookupAlice)

	orig := &fakeJobConfig{paths: &config.Paths{BuildDir: "/b", PkgsDir: "/p"}}
	jobCfg := callerJobConfig(orig, 65534)

	require.Equal(t, "/var/cache/alr/build", jobCfg.GetPaths().BuildDir)
	require.Equal(t, "/var/cache/alr/pkgs", jobCfg.GetPaths().PkgsDir)
}
