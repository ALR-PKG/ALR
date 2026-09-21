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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// layoutFD — дескриптор, через который родитель передаёт JSON-макет
// (первый элемент exec.Cmd.ExtraFiles).
const layoutFD = 3

// RunInit — тело скрытой команды `_internal-sandbox-init -- <cmd> [args...]`.
// Читает макет с fd 3, строит newroot, делает pivot_root и exec'ит целевую
// команду с сохранением окружения. При успехе не возвращается; при любой
// ошибке настройки печатает причину в stderr и возвращает ExitSetupFailure.
func RunInit(args []string) int {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "sandbox-init: не указана целевая команда")
		return ExitSetupFailure
	}

	layout, err := readLayout(layoutFD)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-init: чтение макета: %v\n", err)
		return ExitSetupFailure
	}

	if err := setupRoot(layout); err != nil {
		fmt.Fprintf(os.Stderr, "sandbox-init: настройка ФС: %v\n", err)
		return ExitSetupFailure
	}

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

// lookPath ищет бинарник по PATH уже внутри песочницы.
func lookPath(name string) (string, error) {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p, nil
		}
	}
	return "", fmt.Errorf("команда %s не найдена в PATH", name)
}

func readLayout(fd int) (*Layout, error) {
	f := os.NewFile(uintptr(fd), "layout")
	if f == nil {
		return nil, fmt.Errorf("fd %d недоступен", fd)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var layout Layout
	if err := json.Unmarshal(data, &layout); err != nil {
		return nil, fmt.Errorf("разбор JSON: %w", err)
	}
	return &layout, nil
}

// setupRoot строит новое корневое дерево (bubblewrap-подобная схема):
//
//  1. приватизирует propagation mount'ов, чтобы ничего не утекло на хост;
//  2. newroot — свежая tmpfs в /tmp/alr-sandbox-<случайное>; хостовый
//     корень целиком НЕ bind'ится — только пути из макета (так tmpfs
//     не затеняется bind'ом и остаётся рабочим корнем для pivot_root);
//  3. mount'ы макета bind'ятся на свои же абсолютные пути (ro — с
//     перевмонтированием read-only), родители раньше детей;
//  4. Covers накрываются пустой ro tmpfs (содержимое $HOME недоступно
//     даже на чтение); вложенные в cover mount'ы применяются до того,
//     как cover переводится в ro;
//  5. /dev — свежая tmpfs, в неё ro-bind'ятся только ноды из макета;
//  6. /proc — свежий (новый pid namespace);
//  7. pivot_root + MNT_DETACH старого корня — хостовые пути исчезают
//     из дерева процесса.
//
// Граница безопасности — uid/gid mapping: даже если процесс внутри
// перевмонтирует ro-bind rw (CAP_SYS_ADMIN в своём userns это позволяет
// для флагов vfsmount), запись в чужие файлы блокируется inode-правами,
// так как реальный uid — непривилегированный.
func setupRoot(layout *Layout) error {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("приватизация propagation: %w", err)
	}

	newroot, err := os.MkdirTemp("/tmp", "alr-sandbox-")
	if err != nil {
		return fmt.Errorf("newroot: %w", err)
	}
	if err := unix.Mount("tmpfs", newroot, "tmpfs", 0, "mode=755"); err != nil {
		return fmt.Errorf("tmpfs newroot: %w", err)
	}

	steps := mergeSteps(layout)
	if err := applySteps(newroot, steps); err != nil {
		return err
	}

	if err := setupDev(newroot, layout.DevNodes); err != nil {
		return err
	}

	if layout.ProcFresh {
		proc := filepath.Join(newroot, "proc")
		if err := os.MkdirAll(proc, 0o555); err != nil {
			return fmt.Errorf("mkdir /proc: %w", err)
		}
		if err := unix.Mount("proc", proc, "proc", 0, ""); err != nil {
			return fmt.Errorf("mount свежего /proc: %w", err)
		}
	}

	putOld := filepath.Join(newroot, ".put_old")
	if err := os.Mkdir(putOld, 0o700); err != nil {
		return fmt.Errorf("put_old: %w", err)
	}
	cwd, _ := os.Getwd()
	if err := unix.PivotRoot(newroot, putOld); err != nil {
		return fmt.Errorf("pivot_root: %w", err)
	}
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("chdir /: %w", err)
	}
	if err := unix.Unmount("/.put_old", unix.MNT_DETACH); err != nil {
		return fmt.Errorf("umount put_old: %w", err)
	}
	os.Remove("/.put_old")
	// Каталог-точка монтирования newroot остался на хостовом /tmp;
	// рекурсивный bind /tmp скопировал сам mount newroot в песочницу —
	// отцепим эту копию и удалим каталог (best-effort).
	if base := filepath.Base(newroot); strings.HasPrefix(base, "alr-sandbox-") {
		p := filepath.Join("/tmp", base)
		unix.Unmount(p, unix.MNT_DETACH)
		os.Remove(p)
	}

	if cwd != "" && cwd != "/" {
		if err := os.Chdir(cwd); err != nil {
			os.Chdir("/")
		}
	}
	return nil
}

// fsStep — один шаг построения дерева: bind из макета либо cover
// пустой ro tmpfs.
type fsStep struct {
	target string
	mount  *Mount
	cover  bool
}

// mergeSteps объединяет mount'ы и cover'ы в один отсортированный список:
// родители раньше детей (лексикографический порядок абсолютных путей).
func mergeSteps(layout *Layout) []fsStep {
	steps := make([]fsStep, 0, len(layout.Mounts)+len(layout.Covers))
	for i := range layout.Mounts {
		steps = append(steps, fsStep{target: layout.Mounts[i].Target, mount: &layout.Mounts[i]})
	}
	for _, c := range layout.Covers {
		steps = append(steps, fsStep{target: c, cover: true})
	}
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].target < steps[j].target })
	return steps
}

// applySteps применяет шаги к newroot. Шаги, вложенные в cover,
// применяются внутри его tmpfs до перевода cover'а в ro.
func applySteps(newroot string, steps []fsStep) error {
	i := 0
	for i < len(steps) {
		s := steps[i]
		dst := filepath.Join(newroot, s.target)
		if !s.cover {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return fmt.Errorf("точка монтирования %s: %w", s.target, err)
			}
			if err := unix.Mount(s.mount.Source, dst, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
				return fmt.Errorf("bind %s: %w", s.mount.Source, err)
			}
			if !s.mount.RW {
				if err := makeMountRO(dst); err != nil {
					fmt.Fprintf(os.Stderr, "sandbox-init: предупреждение: %s остался rw: %v\n", s.target, err)
				}
			}
			i++
			continue
		}

		if err := os.MkdirAll(dst, 0o755); err != nil {
			return fmt.Errorf("точка cover'а %s: %w", s.target, err)
		}
		if err := unix.Mount("tmpfs", dst, "tmpfs", 0, "mode=755"); err != nil {
			return fmt.Errorf("cover %s: %w", s.target, err)
		}
		j := i + 1
		var nested []fsStep
		for j < len(steps) && strings.HasPrefix(steps[j].target, s.target+"/") {
			n := steps[j]
			n.target = strings.TrimPrefix(n.target, s.target)
			nested = append(nested, n)
			j++
		}
		if err := applySteps(dst, nested); err != nil {
			return err
		}
		if err := remountRO(dst, false); err != nil {
			return fmt.Errorf("cover %s ro: %w", s.target, err)
		}
		i = j
	}
	return nil
}

func remountRO(target string, recursive bool) error {
	flags := uintptr(unix.MS_REMOUNT | unix.MS_BIND | unix.MS_RDONLY)
	if recursive {
		flags |= unix.MS_REC
	}
	return unix.Mount("", target, "", flags, "")
}

// setupDev подменяет /dev свежей tmpfs и ro-bind'ит в неё только
// разрешённые ноды (bind устройств в userns работает, в отличие от mknod
// на неподдерживаемых ядрах).
func setupDev(newroot string, nodes []string) error {
	dev := filepath.Join(newroot, "dev")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		return fmt.Errorf("mkdir /dev: %w", err)
	}
	if err := unix.Mount("tmpfs", dev, "tmpfs", 0, "mode=755"); err != nil {
		return fmt.Errorf("tmpfs /dev: %w", err)
	}
	for _, node := range nodes {
		dst := filepath.Join(newroot, node)
		f, err := os.OpenFile(dst, os.O_CREATE, 0o644)
		if err != nil {
			return fmt.Errorf("заглушка %s: %w", node, err)
		}
		f.Close()
		if err := unix.Mount(node, dst, "", unix.MS_BIND, ""); err != nil {
			return fmt.Errorf("bind %s: %w", node, err)
		}
		// ro для ноды — желательно, но не критично: права на устройство
		// всё равно проверяются по реальным (не отображённым) кредам.
		if err := makeMountRO(dst); err != nil {
			fmt.Fprintf(os.Stderr, "sandbox-init: предупреждение: %s осталась rw: %v\n", node, err)
		}
	}
	return nil
}

// mountAttr — struct mount_attr ядра (новый mount API, 5.12+).
type mountAttr struct {
	attrSet     uint64
	attrClr     uint64
	propagation uint64
	usernsFd    uint64
}

// makeMountRO делает bind-mount read-only. Классический
// MS_REMOUNT|MS_BIND|MS_RDONLY на bind'е с чужого superblock (devtmpfs)
// в userns даёт EPERM, поэтому сначала пробуется mount_setattr(2),
// работающий внутри userns; на старых ядрах — фолбэк на remount.
func makeMountRO(path string) error {
	if err := mountSetattrRO(path); err == nil {
		return nil
	}
	return remountRO(path, false)
}

func mountSetattrRO(path string) error {
	p, err := unix.BytePtrFromString(path)
	if err != nil {
		return err
	}
	attr := mountAttr{
		attrSet: unix.MOUNT_ATTR_RDONLY | unix.MOUNT_ATTR_NOSUID,
	}
	dirfd := unix.AT_FDCWD
	_, _, errno := syscall.Syscall6(
		unix.SYS_MOUNT_SETATTR,
		uintptr(int64(dirfd)),
		uintptr(unsafe.Pointer(p)),
		0,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
