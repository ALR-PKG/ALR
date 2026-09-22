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
	"golang.org/x/sys/unix"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/sandbox"
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
		cred := &syscall.Credential{Uid: 1000, Gid: 1000, Groups: []uint32{1000, 27}}

		cmd, err := getExecutorCmd("_internal-safe-script-executor", env, cred)
		require.NoError(t, err)
		require.NotNil(t, cmd)

		require.NotNil(t, cmd.SysProcAttr)
		require.NotNil(t, cmd.SysProcAttr.Credential)
		assert.Equal(t, uint32(1000), cmd.SysProcAttr.Credential.Uid)
		assert.Equal(t, uint32(1000), cmd.SysProcAttr.Credential.Gid)
		assert.Equal(t, []uint32{1000, 27}, cmd.SysProcAttr.Credential.Groups)

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

func TestScriptSandboxLayout(t *testing.T) {
	cfg := &fakeResolverConfig{paths: &config.Paths{
		RepoDir:  "/var/cache/alr/repo",
		BuildDir: "/home/user/.cache/alr/build",
		PkgsDir:  "/home/user/.cache/alr/pkgs",
	}}

	findMount := func(l *sandbox.Layout, target string) *sandbox.Mount {
		for i := range l.Mounts {
			if l.Mounts[i].Target == target {
				return &l.Mounts[i]
			}
		}
		return nil
	}

	t.Run("explicit scriptDir is ro", func(t *testing.T) {
		l := ScriptSandboxLayout(cfg, "/home/user/proj")
		m := findMount(l, "/home/user/proj")
		require.NotNil(t, m)
		assert.False(t, m.RW)
		assert.Nil(t, findMount(l, "/var/cache/alr/repo"))
		assert.True(t, findMount(l, "/home/user/.cache/alr/build").RW)
	})

	t.Run("empty scriptDir falls back to RepoDir ro", func(t *testing.T) {
		l := ScriptSandboxLayout(cfg, "")
		m := findMount(l, "/var/cache/alr/repo")
		require.NotNil(t, m)
		assert.False(t, m.RW)
	})
}

func TestScriptSandboxAttempts(t *testing.T) {
	cfg := &fakeResolverConfig{paths: &config.Paths{
		RepoDir:  "/var/cache/alr/repo",
		BuildDir: "/cache/build",
		PkgsDir:  "/cache/pkgs",
	}}
	layout := ScriptSandboxLayout(cfg, "/proj")
	attempts := scriptSandboxAttempts(layout, 1000, 1000)
	require.Len(t, attempts, 2)
	assert.Equal(t, "namespaces", attempts[0].name)
	assert.Equal(t, "landlock", attempts[1].name)

	t.Run("namespaces attempt wraps and clears credential", func(t *testing.T) {
		cmd, err := getExecutorCmd("_internal-safe-script-executor", nil,
			&syscall.Credential{Uid: 1000, Gid: 1000})
		require.NoError(t, err)
		require.NoError(t, attempts[0].wrap(cmd))

		assert.Equal(t, "_internal-sandbox-init", cmd.Args[1])
		require.NotNil(t, cmd.SysProcAttr)
		assert.Nil(t, cmd.SysProcAttr.Credential,
			"хостовый Credential внутри userns невалиден — заменяется маппингом")
		assert.NotZero(t, cmd.SysProcAttr.Cloneflags&unix.CLONE_NEWUSER)
		require.Len(t, cmd.SysProcAttr.UidMappings, 1)
		assert.Equal(t, 1000, cmd.SysProcAttr.UidMappings[0].HostID)
		assert.Len(t, cmd.ExtraFiles, 1)
	})

	t.Run("landlock attempt wraps and keeps credential", func(t *testing.T) {
		cred := &syscall.Credential{Uid: 1000, Gid: 1000}
		cmd, err := getExecutorCmd("_internal-safe-script-executor", nil, cred)
		require.NoError(t, err)
		require.NoError(t, attempts[1].wrap(cmd))

		assert.Equal(t, "_internal-sandbox-ll", cmd.Args[1])
		require.NotNil(t, cmd.SysProcAttr)
		assert.Same(t, cred, cmd.SysProcAttr.Credential,
			"без namespaces Credential остаётся — процесс идёт от uid вызвавшего")
		assert.Zero(t, cmd.SysProcAttr.Cloneflags)
		assert.Len(t, cmd.ExtraFiles, 1)
	})
}

func TestScriptSandboxAttemptsDaemonPath(t *testing.T) {
	// Демон-путь: GetSafeScriptExecutorAs(callerUID, gid, groups, env, cfg).
	// layout строится из callerJobConfig — BuildDir/PkgsDir вызвавшего rw,
	// RepoDir ro (scriptDir=""), uid/gid вызвавшего идут в маппинг.
	cfg := &fakeResolverConfig{paths: &config.Paths{
		RepoDir:  "/var/cache/alr/repo",
		BuildDir: "/home/alice/.cache/alr/build",
		PkgsDir:  "/home/alice/.cache/alr/pkgs",
	}}
	callerEnv := []string{
		"HOME=/home/alice",
		"USER=alice",
		"LOGNAME=alice",
		"PATH=/usr/bin:/bin",
	}
	cred := &syscall.Credential{Uid: 1000, Gid: 100, Groups: []uint32{100, 998}}

	layout := ScriptSandboxLayout(cfg, "")
	attempts := scriptSandboxAttempts(layout, int(cred.Uid), int(cred.Gid))

	t.Run("layout uses caller cache paths", func(t *testing.T) {
		rw := map[string]bool{}
		for _, m := range layout.Mounts {
			rw[m.Target] = m.RW
		}
		assert.True(t, rw["/home/alice/.cache/alr/build"])
		assert.True(t, rw["/home/alice/.cache/alr/pkgs"])
		ro, ok := rw["/var/cache/alr/repo"]
		require.True(t, ok)
		assert.False(t, ro, "RepoDir ro при пустом scriptDir (пакет из репозитория)")
	})

	t.Run("namespaces: caller uid/gid mapped to root, credential cleared", func(t *testing.T) {
		cmd, err := getExecutorCmd("_internal-safe-script-executor", callerEnv, cred)
		require.NoError(t, err)
		require.NoError(t, attempts[0].wrap(cmd))

		require.NotNil(t, cmd.SysProcAttr)
		assert.Nil(t, cmd.SysProcAttr.Credential,
			"в userns хостовый Credential невалиден — его заменяет маппинг 0→caller")
		require.Equal(t,
			[]syscall.SysProcIDMap{{ContainerID: 0, HostID: 1000, Size: 1}},
			cmd.SysProcAttr.UidMappings)
		require.Equal(t,
			[]syscall.SysProcIDMap{{ContainerID: 0, HostID: 100, Size: 1}},
			cmd.SysProcAttr.GidMappings)

		home, ok := envVal(cmd.Env, "HOME")
		require.True(t, ok)
		assert.Equal(t, "/home/alice", home, "env вызвавшего проходит в ns-попытку")
	})

	t.Run("landlock: credential with groups preserved", func(t *testing.T) {
		cmd, err := getExecutorCmd("_internal-safe-script-executor", callerEnv, cred)
		require.NoError(t, err)
		require.NoError(t, attempts[1].wrap(cmd))

		require.NotNil(t, cmd.SysProcAttr)
		require.NotNil(t, cmd.SysProcAttr.Credential)
		assert.Equal(t, uint32(1000), cmd.SysProcAttr.Credential.Uid)
		assert.Equal(t, uint32(100), cmd.SysProcAttr.Credential.Gid)
		assert.Equal(t, []uint32{100, 998}, cmd.SysProcAttr.Credential.Groups,
			"supplementary groups работают только в landlock/прямом режиме")

		home, ok := envVal(cmd.Env, "HOME")
		require.True(t, ok)
		assert.Equal(t, "/home/alice", home)
	})
}

func TestSandboxAttemptsNotAppliedToInstaller(t *testing.T) {
	// installer и repos executor'ы остаются вне песочницы: их команды
	// не проходят через sandbox.Wrap
	cmd, err := getExecutorCmd("_internal-installer", nil, nil)
	require.NoError(t, err)
	require.NotContains(t, cmd.Args, "_internal-sandbox-init")
	require.NotContains(t, cmd.Args, "_internal-sandbox-ll")
	assert.Empty(t, cmd.ExtraFiles)
}
