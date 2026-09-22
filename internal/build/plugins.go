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
	"syscall"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"

	"git.alr-pkg.ru/ALR/ALR/internal/logger"
	"git.alr-pkg.ru/ALR/ALR/internal/sandbox"
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

// ScriptSandboxLayout вычисляет макет песочницы для script executor'а:
// rw — дерево сборки, кэш пакетов и /tmp; ro — каталог скрипта и системные
// каталоги. Если scriptDir пуст (пакет из репозитория, точный путь скрипта
// ещё не известен на момент запуска plugin'а), ro-покрытием служит весь
// RepoDir — чтение репо-кэша безопасно.
func ScriptSandboxLayout(cfg Config, scriptDir string) *sandbox.Layout {
	paths := cfg.GetPaths()
	if scriptDir == "" {
		scriptDir = paths.RepoDir
	}
	return sandbox.LayoutFromPaths(paths.BuildDir, paths.PkgsDir, scriptDir)
}

// GetSafeScriptExecutor запускает script executor в песочнице. Каскад
// деградации: user+mount+pid namespaces → landlock → прямой запуск с
// предупреждением. ALR_SANDBOX=off отключает песочницу полностью.
// scriptDir — каталог скрипта сборки (ro в песочнице); "" — для пакетов
// из репозитория (покрывается весь RepoDir).
func GetSafeScriptExecutor(cfg Config, scriptDir string) (ScriptExecutor, func(), error) {
	return getSandboxedScriptExecutor(cfg, scriptDir, nil, nil)
}

// GetSafeScriptExecutorAs запускает script executor от указанных uid/gid
// (демон выполняет сборку от uid вызвавшего) с заданным окружением
// (HOME/USER/LOGNAME вызвавшего — см. callerEnv в internal/dbus).
// groups — supplementary groups вызвавшего: Credential.Groups заменяет
// ВСЕ supplementary groups процесса, поэтому без них plugin теряет
// членство в группах (например alr для общего dl-кэша). Сюда передаются
// все группы из NSS, включая основную — ядро дедуплицирует её с Gid.
// groups действуют только в прямом и landlock режимах; в ns-режиме
// supplementary groups вызвавшего теряются (маппинг одного uid/gid) —
// известное ограничение до Task 9.4.
// cfg — конфиг с путями ВЫЗВАВШЕГО (callerJobConfig), из него строится
// макет песочницы.
func GetSafeScriptExecutorAs(uid, gid uint32, groups []uint32, env []string, cfg Config, scriptDir string) (ScriptExecutor, func(), error) {
	cred := &syscall.Credential{Uid: uid, Gid: gid, Groups: groups}
	return getSandboxedScriptExecutor(cfg, scriptDir, env, cred)
}

// sandboxAttempt — одна ступень каскада песочницы: обёртка команды
// запуска plugin'а и её имя для логов.
type sandboxAttempt struct {
	name string
	wrap func(cmd *exec.Cmd) error
}

// scriptSandboxAttempts — каскад обёрток script executor'а:
// namespaces (sandbox.Wrap) → landlock (sandbox.WrapLandlock).
// Последняя ступень — прямой запуск — обёртки не требует и в список
// не входит.
func scriptSandboxAttempts(layout *sandbox.Layout, uid, gid int) []sandboxAttempt {
	return []sandboxAttempt{
		{"namespaces", func(cmd *exec.Cmd) error {
			// В userns действует маппинг uid 0→вызывающий: Credential с
			// хостовым uid внутри namespace невалиден и заменяется маппингом.
			if cmd.SysProcAttr != nil {
				cmd.SysProcAttr.Credential = nil
			}
			return sandbox.Wrap(cmd, layout, uid, gid)
		}},
		{"landlock", func(cmd *exec.Cmd) error {
			return sandbox.WrapLandlock(cmd, layout)
		}},
	}
}

// getSandboxedScriptExecutor реализует каскад запуска script executor'а:
// namespaces → landlock → прямой запуск. Каждая попытка строит команду
// заново — обёртки мутируют cmd. Ошибка handshake plugin'а (в т.ч. exit
// code sandbox.ExitSetupFailure от init'а) считается недоступностью режима
// и переводит каскад на ступень ниже. ALR_SANDBOX=off — сразу прямой запуск.
func getSandboxedScriptExecutor(cfg Config, scriptDir string, env []string, cred *syscall.Credential) (ScriptExecutor, func(), error) {
	layout := ScriptSandboxLayout(cfg, scriptDir)

	uid, gid := os.Getuid(), os.Getgid()
	if cred != nil {
		uid, gid = int(cred.Uid), int(cred.Gid)
	}

	buildCmd := func() (*exec.Cmd, error) {
		return getExecutorCmd("_internal-safe-script-executor", env, cred)
	}

	if os.Getenv("ALR_SANDBOX") != "off" {
		attempts := scriptSandboxAttempts(layout, uid, gid)
		for i, attempt := range attempts {
			cmd, err := buildCmd()
			if err != nil {
				return nil, nil, err
			}
			if err := attempt.wrap(cmd); err != nil {
				slog.Warn("песочница: не удалось применить режим, фолбэк", "mode", attempt.name, "error", err)
				continue
			}
			executor, cleanup, err := dispenseExecutor[ScriptExecutor](cmd, "script-executor")
			if err == nil {
				return executor, cleanup, nil
			}
			next := "прямой запуск"
			if i+1 < len(attempts) {
				next = attempts[i+1].name
			}
			slog.Warn("песочница: режим недоступен, фолбэк", "mode", attempt.name, "next", next, "error", err)
		}
	}

	cmd, err := buildCmd()
	if err != nil {
		return nil, nil, err
	}
	return dispenseExecutor[ScriptExecutor](cmd, "script-executor")
}

// getExecutorCmd строит команду запуска plugin-процесса. При cred != nil
// процесс запускается от указанных uid/gid; при env != nil окружение
// берётся как есть (без дополнения окружением текущего процесса).
func getExecutorCmd(subCommand string, env []string, cred *syscall.Credential) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(executable, subCommand)
	if env != nil {
		cmd.Env = env
	} else {
		setCommonCmdEnv(cmd)
	}
	if cred != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
	}
	return cmd, nil
}

func GetSafeReposExecutor() (ReposExecutor, func(), error) {
	return getSafeExecutor[ReposExecutor]("_internal-repos", "repos")
}

func getSafeExecutor[T any](subCommand, pluginName string) (T, func(), error) {
	cmd, err := getExecutorCmd(subCommand, nil, nil)
	if err != nil {
		var zero T
		return zero, nil, err
	}
	return dispenseExecutor[T](cmd, pluginName)
}

func dispenseExecutor[T any](cmd *exec.Cmd, pluginName string) (T, func(), error) {
	var err error

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
