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
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func envVal(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, e := range env {
		if len(e) > len(prefix) && e[:len(prefix)] == prefix {
			return e[len(prefix):], true
		}
	}
	return "", false
}

func TestGetExecutorCmd(t *testing.T) {
	t.Run("credential and caller env are applied", func(t *testing.T) {
		env := []string{
			"HOME=/home/alice",
			"USER=alice",
			"LOGNAME=alice",
			"PATH=/usr/bin:/bin",
		}
		cred := &syscall.Credential{Uid: 1000, Gid: 1000}

		cmd, err := getExecutorCmd("_internal-safe-script-executor", env, cred)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		require.NotNil(t, cmd.SysProcAttr)
		require.NotNil(t, cmd.SysProcAttr.Credential)
		assert.Equal(t, uint32(1000), cmd.SysProcAttr.Credential.Uid)
		assert.Equal(t, uint32(1000), cmd.SysProcAttr.Credential.Gid)

		home, ok := envVal(cmd.Env, "HOME")
		require.True(t, ok)
		assert.Equal(t, "/home/alice", home)

		user, ok := envVal(cmd.Env, "USER")
		require.True(t, ok)
		assert.Equal(t, "alice", user)

		// env не должно неявно дополняться окружением демона
		assert.Len(t, cmd.Env, len(env))
	})

	t.Run("nil credential keeps current behavior", func(t *testing.T) {
		t.Setenv("HOME", "/root")
		t.Setenv("USER", "root")

		cmd, err := getExecutorCmd("_internal-safe-script-executor", nil, nil)
		require.NoError(t, err)

		assert.Nil(t, cmd.SysProcAttr)

		home, ok := envVal(cmd.Env, "HOME")
		require.True(t, ok)
		assert.Equal(t, os.Getenv("HOME"), home)
	})
}
