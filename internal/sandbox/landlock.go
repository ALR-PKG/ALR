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

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ErrLandlockUnavailable — ядро не поддерживает landlock (< 5.13).
var ErrLandlockUnavailable = errors.New("landlock недоступен в ядре (< 5.13)")

// landlockABI возвращает версию landlock ABI ядра (0 — недоступен).
func landlockABI() int {
	v, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0
	}
	return int(v)
}

// landlockWriteFlags — набор write-операций, доступный в данном ABI.
// REFER появился в ABI 2, TRUNCATE — в ABI 3; более старшие флаги ядро
// отвергнет, поэтому набор фильтруется по детектированной версии.
func landlockWriteFlags(abi int) uint64 {
	flags := uint64(unix.LANDLOCK_ACCESS_FS_WRITE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
		unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM |
		unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_REMOVE_DIR)
	if abi >= 2 {
		flags |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		flags |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	return flags
}

// ApplyLandlock ограничивает ТЕКУЩИЙ процесс ruleset'ом landlock.
// Ограничение наследуется через exec, поэтому применяется обёрткой-init
// перед exec'ом целевой команды.
//
// Выбранная модель: handled_access_fs включает ТОЛЬКО write-операции.
// Для путей, не покрытых правилами, landlock запрещает handled-действия —
// значит, чтение и выполнение остаются доступными по всей ФС (системные
// каталоги, скрипты), а запись возможна лишь в rw-путях макета. Это
// соответствует цели режима (запрет записи вне дерева сборки) и не ломает
// выполнение скомпилированных бинарников и чтение скрипта. Covers макета
// (сокрытие $HOME) landlock выразить не может — деградация до
// «домашний каталог читаем, но неписабелен».
func ApplyLandlock(layout *Layout) error {
	abi := landlockABI()
	if abi < 1 {
		return ErrLandlockUnavailable
	}

	handled := landlockWriteFlags(abi)
	attr := unix.LandlockRulesetAttr{Access_fs: handled}
	fd, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	rulesetFd := int(fd)
	defer unix.Close(rulesetFd)

	for i := range layout.Mounts {
		m := &layout.Mounts[i]
		if !m.RW {
			continue
		}
		if err := landlockAllowPath(rulesetFd, m.Target, handled); err != nil {
			return fmt.Errorf("правило для %s: %w", m.Target, err)
		}
	}

	// no_new_privs обязателен перед restrict_self.
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("prctl(PR_SET_NO_NEW_PRIVS): %w", err)
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(rulesetFd), 0, 0); errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

// landlockAllowPath добавляет правило path_beneath, разрешающее allowed
// (write-набор) для дерева path.
func landlockAllowPath(rulesetFd int, path string, allowed uint64) error {
	f, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(f)

	rule := unix.LandlockPathBeneathAttr{
		Allowed_access: allowed,
		Parent_fd:      int32(f),
	}
	_, _, errno := unix.Syscall6(
		unix.SYS_LANDLOCK_ADD_RULE,
		uintptr(rulesetFd),
		unix.LANDLOCK_RULE_PATH_BENEATH,
		uintptr(unsafe.Pointer(&rule)),
		0, 0, 0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}

// WrapLandlock превращает cmd в запуск той же команды с landlock-
// ограничением: процесс перевыполняет собственный бинарник с подкомандой
// `_internal-sandbox-ll -- <исходные args>`, получает JSON-макет на fd 3.
// В отличие от Wrap, namespaces не создаются — фолбэк-режим для систем,
// где userns недоступен. Как и Wrap, должен вызываться до добавления
// собственных ExtraFiles (макет занимает fd 3).
func WrapLandlock(cmd *exec.Cmd, layout *Layout) error {
	return wrapReexec(cmd, layout, "_internal-sandbox-ll")
}

// RunLandlockInit — тело скрытой команды `_internal-sandbox-ll`:
// читает макет с fd 3, применяет ApplyLandlock к себе и exec'ит целевую
// команду. При успехе не возвращается; при ошибке — ExitSetupFailure.
func RunLandlockInit(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sandbox-ll: не указана целевая команда")
		return ExitSetupFailure
	}

	layout, err := readLayout(layoutFD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-ll: чтение макета: %v\n", err)
		return ExitSetupFailure
	}

	if err := ApplyLandlock(layout); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-ll: landlock: %v\n", err)
		return ExitSetupFailure
	}

	return execTarget(args)
}

// execTarget разрешает args[0] по PATH (если не абсолютный) и заменяет
// процесс целевой командой. Общий хвост RunInit и RunLandlockInit.
func execTarget(args []string) int {
	target := args[0]
	if !filepath.IsAbs(target) {
		resolved, err := lookPath(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sandbox-init: %v\n", err)
			return ExitSetupFailure
		}
		target = resolved
	}
	if err := syscall.Exec(target, args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-init: exec %s: %v\n", target, err)
		return ExitSetupFailure
	}
	return ExitSetupFailure // недостижимо
}
