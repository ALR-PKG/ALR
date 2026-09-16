// ALR - Any Linux Repository
// Copyright (C) 2025 The ALR Authors
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
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"

	"git.alr-pkg.ru/ALR/ALR/internal/logger"
)

var pluginMap = map[string]plugin.Plugin{
	"script-executor": &ScriptExecutorPlugin{},
	"installer":       &InstallerExecutorPlugin{},
	"repos":           &ReposExecutorPlugin{},
}

var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "ALR_PLUGIN",
	MagicCookieValue: "-",
}

func setCommonCmdEnv(cmd *exec.Cmd) {
	cmd.Env = []string{
		"HOME=" + os.Getenv("HOME"),
		"LOGNAME=" + os.Getenv("USER"),
		"USER=" + os.Getenv("USER"),
		"PATH=/usr/bin:/bin:/usr/local/bin",
	}
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "LANG=") ||
			strings.HasPrefix(env, "LANGUAGE=") ||
			strings.HasPrefix(env, "LC_") ||
			strings.HasPrefix(env, "ALR_LOG_LEVEL=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
}

func GetPluginServeCommonConfig() *plugin.ServeConfig {
	return &plugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Logger: hclog.New(&hclog.LoggerOptions{
			Name:        "plugin",
			Output:      os.Stderr,
			Level:       hclog.Trace,
			JSONFormat:  true,
			DisableTime: true,
		}),
	}
}

func GetSafeInstaller() (InstallerExecutor, func(), error) {
	return getSafeExecutor[InstallerExecutor]("_internal-installer", "installer")
}

func GetSafeScriptExecutor() (ScriptExecutor, func(), error) {
	return getSafeExecutor[ScriptExecutor]("_internal-safe-script-executor", "script-executor")
}

func GetSafeReposExecutor() (ReposExecutor, func(), error) {
	return getSafeExecutor[ReposExecutor]("_internal-repos", "repos")
}

func getSafeExecutor[T any](subCommand, pluginName string) (T, func(), error) {
	var err error

	executable, err := os.Executable()
	if err != nil {
		var zero T
		return zero, nil, err
	}

	cmd := exec.Command(executable, subCommand)
	setCommonCmdEnv(cmd)

	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  HandshakeConfig,
		Plugins:          pluginMap,
		Cmd:              cmd,
		Logger:           logger.GetHCLoggerAdapter(),
		SkipHostEnv:      true,
		UnixSocketConfig: &plugin.UnixSocketConfig{},
		SyncStderr:       os.Stderr,
	})
	rpcClient, err := client.Client()
	if err != nil {
		var zero T
		return zero, nil, err
	}

	var cleanupOnce sync.Once
	cleanup := func() {
		cleanupOnce.Do(func() {
			client.Kill()
		})
	}

	defer func() {
		if err != nil {
			slog.Debug("close executor")
			cleanup()
		}
	}()

	raw, err := rpcClient.Dispense(pluginName)
	if err != nil {
		var zero T
		return zero, nil, err
	}

	executor, ok := raw.(T)
	if !ok {
		var zero T
		err = fmt.Errorf("dispensed object is not a %T (got %T)", zero, raw)
		return zero, nil, err
	}

	return executor, cleanup, nil
}
