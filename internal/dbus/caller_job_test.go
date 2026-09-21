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

// Тесты подменяют lookupUser/lookupGroup/lookupGroupIds и не параллельны:
// подмена глобальна для процесса.

func stubLookupGroup(t *testing.T, fn func(name string) (*user.Group, error)) {
	t.Helper()
	orig := lookupGroup
	lookupGroup = fn
	t.Cleanup(func() { lookupGroup = orig })
}

func stubLookupGroupIds(t *testing.T, fn func(u *user.User) ([]string, error)) {
	t.Helper()
	orig := lookupGroupIds
	lookupGroupIds = fn
	t.Cleanup(func() { lookupGroupIds = orig })
}

func stubNobodyGroup(t *testing.T) {
	t.Helper()
	stubLookupGroup(t, func(name string) (*user.Group, error) {
		if name == "nobody" {
			return &user.Group{Gid: "65534", Name: "nobody"}, nil
		}
		return nil, errors.New("no such group")
	})
}

func TestCallerIdentity(t *testing.T) {
	t.Run("gid and groups from passwd entry", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: "1000", Gid: "1001", Username: "alice", HomeDir: "/home/alice"}, nil
		})
		stubLookupGroupIds(t, func(u *user.User) ([]string, error) {
			return []string{"1001", "27"}, nil
		})

		info := callerIdentity(1000)
		assert.Equal(t, "/home/alice", info.home)
		assert.Equal(t, "alice", info.username)
		assert.Equal(t, uint32(1001), info.gid)
		assert.Equal(t, []uint32{1001, 27}, info.groups)
	})

	t.Run("group ids lookup error keeps identity without groups", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: "1000", Gid: "1000", Username: "alice", HomeDir: "/home/alice"}, nil
		})
		stubLookupGroupIds(t, func(u *user.User) ([]string, error) {
			return nil, errors.New("nss unavailable")
		})

		info := callerIdentity(1000)
		assert.Equal(t, uint32(1000), info.gid)
		assert.Empty(t, info.groups)
	})

	t.Run("lookup error falls back to nobody", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return nil, errors.New("no such user")
		})
		stubNobodyGroup(t)

		info := callerIdentity(1000)
		assert.Equal(t, systemCacheRoot, info.home)
		assert.Equal(t, nobodyUsername, info.username)
		assert.Equal(t, uint32(65534), info.gid)
		assert.Empty(t, info.groups)
	})

	t.Run("empty home dir falls back to nobody", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: "1000", Gid: "1000", Username: "ghost", HomeDir: ""}, nil
		})
		stubNobodyGroup(t)

		info := callerIdentity(1000)
		assert.Equal(t, systemCacheRoot, info.home)
		assert.Equal(t, nobodyUsername, info.username)
		assert.Equal(t, uint32(65534), info.gid)
	})
}

func TestNobodyGID(t *testing.T) {
	t.Run("resolved via nobody group", func(t *testing.T) {
		stubLookupGroup(t, func(name string) (*user.Group, error) {
			if name == "nobody" {
				return &user.Group{Gid: "99", Name: "nobody"}, nil
			}
			return nil, errors.New("no such group")
		})
		assert.Equal(t, uint32(99), nobodyGID())
	})

	t.Run("nogroup fallback", func(t *testing.T) {
		stubLookupGroup(t, func(name string) (*user.Group, error) {
			if name == "nogroup" {
				return &user.Group{Gid: "65533", Name: "nogroup"}, nil
			}
			return nil, errors.New("no such group")
		})
		assert.Equal(t, uint32(65533), nobodyGID())
	})

	t.Run("hardcoded fallback", func(t *testing.T) {
		stubLookupGroup(t, func(name string) (*user.Group, error) {
			return nil, errors.New("no such group")
		})
		assert.Equal(t, uint32(65534), nobodyGID())
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
