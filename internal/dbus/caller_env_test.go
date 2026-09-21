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
)

// Тесты подменяют lookupUser и не параллельны: подмена глобальна для процесса.
func stubLookupUser(t *testing.T, fn func(uid string) (*user.User, error)) {
	t.Helper()
	orig := lookupUser
	lookupUser = fn
	t.Cleanup(func() { lookupUser = orig })
}

func lookupAlice(uid string) (*user.User, error) {
	if uid == "1000" {
		return &user.User{Uid: "1000", Username: "alice", HomeDir: "/home/alice"}, nil
	}
	return nil, errors.New("no such user")
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if len(e) > len(prefix) && e[:len(prefix)] == prefix {
			return e[len(prefix):], true
		}
	}
	return "", false
}

func TestCallerEnv(t *testing.T) {
	t.Run("env from passwd entry", func(t *testing.T) {
		stubLookupUser(t, lookupAlice)

		env, err := callerEnv(1000)
		require.NoError(t, err)

		home, ok := envValue(env, "HOME")
		require.True(t, ok)
		assert.Equal(t, "/home/alice", home)

		username, ok := envValue(env, "USER")
		require.True(t, ok)
		assert.Equal(t, "alice", username)

		logname, ok := envValue(env, "LOGNAME")
		require.True(t, ok)
		assert.Equal(t, "alice", logname)

		path, ok := envValue(env, "PATH")
		require.True(t, ok)
		assert.NotEmpty(t, path)
	})

	t.Run("lookup error falls back to nobody", func(t *testing.T) {
		stubLookupUser(t, lookupAlice)

		env, err := callerEnv(65534)
		require.NoError(t, err)

		home, ok := envValue(env, "HOME")
		require.True(t, ok)
		assert.Equal(t, "/var/cache/alr", home)

		username, ok := envValue(env, "USER")
		require.True(t, ok)
		assert.Equal(t, "nobody", username)

		logname, ok := envValue(env, "LOGNAME")
		require.True(t, ok)
		assert.Equal(t, "nobody", logname)
	})

	t.Run("uid 65534 is nobody even with passwd entry", func(t *testing.T) {
		stubLookupUser(t, func(uid string) (*user.User, error) {
			return &user.User{Uid: uid, Username: "someone", HomeDir: "/nonexistent"}, nil
		})

		env, err := callerEnv(65534)
		require.NoError(t, err)

		home, _ := envValue(env, "HOME")
		assert.Equal(t, "/var/cache/alr", home)

		username, _ := envValue(env, "USER")
		assert.Equal(t, "nobody", username)
	})
}

func TestCallerCachePaths(t *testing.T) {
	t.Run("paths under caller home", func(t *testing.T) {
		stubLookupUser(t, lookupAlice)

		buildDir, pkgsDir := callerCachePaths(1000)
		assert.Equal(t, "/home/alice/.cache/alr/build", buildDir)
		assert.Equal(t, "/home/alice/.cache/alr/pkgs", pkgsDir)
	})

	t.Run("nobody falls back to system cache", func(t *testing.T) {
		stubLookupUser(t, lookupAlice)

		buildDir, pkgsDir := callerCachePaths(65534)
		assert.Equal(t, "/var/cache/alr/build", buildDir)
		assert.Equal(t, "/var/cache/alr/pkgs", pkgsDir)
	})
}
