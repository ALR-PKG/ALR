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

// Package sandbox реализует встроенную песочницу ALR: макет ФС (Layout),
// обёртку команд в user+mount+pid namespaces (Wrap) и точку входа
// re-exec init'а (RunInit).
package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// ExitSetupFailure — зарезервированный код выхода init'а песочницы:
// «песочница не завелась». Позволяет каскаду деградации отличить сбой
// настройки от падения команды внутри песочницы.
const ExitSetupFailure = 70

// Mount — одна запись bind-mount'а в песочнице.
type Mount struct {
	Source string `json:"source"`
	Target string `json:"target"`
	RW     bool   `json:"rw"`
}

// Layout — макет ФС песочницы, передаётся init'у JSON'ом через fd 3.
type Layout struct {
	Mounts    []Mount  `json:"mounts"`
	DevNodes  []string `json:"dev_nodes"`
	ProcFresh bool     `json:"proc_fresh"`
}

// LayoutFromPaths вычисляет макет из путей конфига: rw — дерево сборки,
// кэш пакетов и /tmp; ro — каталог скрипта и системные каталоги.
// Дедупликация: вложенный путь, уже покрытый mount'ом с не меньшими
// правами, опускается («наиболее общий покрывающий путь, rw сильнее»);
// запись rw внутри ro-родителя сохраняется и идёт после него.
func LayoutFromPaths(buildDir, pkgsDir, scriptDir string) *Layout {
	rw := []string{buildDir, pkgsDir, "/tmp"}
	ro := []string{scriptDir, "/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc"}

	var mounts []Mount
	for _, p := range rw {
		p = filepath.Clean(p)
		mounts = append(mounts, Mount{Source: p, Target: p, RW: true})
	}
	for _, p := range ro {
		p = filepath.Clean(p)
		mounts = append(mounts, Mount{Source: p, Target: p})
	}

	mounts = dedupMounts(mounts)
	sort.SliceStable(mounts, func(i, j int) bool {
		// родители раньше детей — порядок применения mount'ов
		return mounts[i].Target < mounts[j].Target
	})

	return &Layout{
		Mounts:    mounts,
		DevNodes:  []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"},
		ProcFresh: true,
	}
}

// pathCovers сообщает, покрывает ли parent путь child.
func pathCovers(parent, child string) bool {
	return parent == child || strings.HasPrefix(child, parent+"/")
}

// dedupMounts удаляет записи, чей target покрыт другой записью с не
// меньшими правами (rw покрывает и rw, и ro; ro покрывает только ro).
func dedupMounts(mounts []Mount) []Mount {
	out := make([]Mount, 0, len(mounts))
	for i, m := range mounts {
		redundant := false
		for j, p := range mounts {
			if i == j || !pathCovers(p.Target, m.Target) {
				continue
			}
			if p.Target == m.Target && p.RW == m.RW && j > i {
				continue // одинаковая запись позже — удалим её, не эту
			}
			if p.RW || !m.RW {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, m)
		}
	}
	return out
}

// Wrap превращает cmd в запуск той же команды внутри песочницы:
// процесс перевыполняет собственный бинарник с подкомандой
// `_internal-sandbox-init -- <исходные args>`, получает JSON-макет на fd 3
// (ExtraFiles) и стартует в user+mount+pid namespaces с маппингом
// uid/gid 0→вызывающий.
//
// Wrap должен вызываться до добавления собственных ExtraFiles: макет
// занимает fd 3. Родительской стороне pipe (cmd.ExtraFiles[0]) можно
// закрыть после cmd.Start(). Недоступность userns проявится как ошибка
// запуска (clone EPERM) либо exit code ExitSetupFailure — это сигнал
// каскаду деградации.
func Wrap(cmd *exec.Cmd, layout *Layout, uid, gid int) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("sandbox: путь к собственному бинарнику: %w", err)
	}

	data, err := json.Marshal(layout)
	if err != nil {
		return fmt.Errorf("sandbox: маршалинг макета: %w", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("sandbox: pipe для макета: %w", err)
	}
	// Макет мал и целиком помещается в буфер pipe — писать можно до Start.
	if _, err := w.Write(data); err != nil {
		r.Close()
		w.Close()
		return fmt.Errorf("sandbox: запись макета: %w", err)
	}
	w.Close()

	args := append([]string{self, "_internal-sandbox-init", "--"}, cmd.Args...)
	cmd.Path = self
	cmd.Args = args
	cmd.ExtraFiles = append(cmd.ExtraFiles, r)

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Cloneflags |= unix.CLONE_NEWUSER | unix.CLONE_NEWNS | unix.CLONE_NEWPID
	cmd.SysProcAttr.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: uid, Size: 1}}
	cmd.SysProcAttr.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: gid, Size: 1}}

	return nil
}
