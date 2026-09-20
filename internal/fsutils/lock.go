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

package fsutils

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

var ErrLocked = errors.New("resource is locked by another process")

type lockEntry struct {
	file *os.File
	refs int
}

// heldLocks — реестр локов, удерживаемых этим процессом: flock привязан
// к open file description, поэтому повторный open+flock того же файла в
// том же процессе вернул бы EWOULDBLOCK. Реестр делает TryLock
// реентерабельным в пределах процесса (например, при рекурсивной сборке
// сабпакетов одного base)
var (
	locksMu   sync.Mutex
	heldLocks = map[string]*lockEntry{}
)

// LockHandle — handle удерживаемой блокировки. Close уменьшает счётчик
// ссылок; реальное снятие flock происходит при Close последнего handle на путь
type LockHandle struct {
	path   string
	closed bool
}

// Close освобождает блокировку. Повторный вызов — no-op.
func (l *LockHandle) Close() error {
	locksMu.Lock()
	defer locksMu.Unlock()
	if l.closed {
		return nil
	}
	l.closed = true

	entry, ok := heldLocks[l.path]
	if !ok {
		return nil
	}
	entry.refs--
	if entry.refs > 0 {
		return nil
	}
	err := entry.file.Close()
	delete(heldLocks, l.path)
	return err
}

// Lock создаёт (при необходимости) файл path и берёт exclusive блокирующий
// flock(2): если блокировку держит другой процесс, вызов ждёт её
// освобождения. Возвращённый LockHandle снимает блокировку при Close.
// В пределах процесса вызов реентерабелен: повторный Lock того же пути
// возвращает новый handle на тот же flock.
func Lock(path string) (*LockHandle, error) {
	return flockPath(path, 0)
}

// TryLock создаёт (при необходимости) файл path и берёт exclusive
// non-blocking flock(2). Возвращённый LockHandle снимает блокировку при Close.
// В пределах процесса вызов реентерабелен: повторный TryLock того же пути
// возвращает новый handle на тот же flock.
// Если блокировку уже держит другой процесс — ErrLocked.
func TryLock(path string) (*LockHandle, error) {
	l, err := flockPath(path, unix.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, ErrLocked
	}
	return l, err
}

// flockPath — общая часть Lock и TryLock: flags добавляются к LOCK_EX
// (unix.LOCK_NB для non-blocking варианта).
func flockPath(path string, flags int) (*LockHandle, error) {
	dir := filepath.Dir(path)
	_, statErr := os.Stat(dir)
	dirCreated := os.IsNotExist(statErr)
	if err := os.MkdirAll(dir, 0o2775); err != nil {
		return nil, fmt.Errorf("не удалось создать каталог для файла блокировки %s: %w", path, err)
	}

	locksMu.Lock()
	defer locksMu.Unlock()

	if entry, ok := heldLocks[path]; ok {
		entry.refs++
		return &LockHandle{path: path}, nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o664)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл блокировки %s: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|flags); err != nil {
		f.Close()
		return nil, fmt.Errorf("не удалось заблокировать %s: %w", path, err)
	}

	normalizeLockAccess(path, dir, dirCreated)

	heldLocks[path] = &lockEntry{file: f, refs: 1}
	return &LockHandle{path: path}, nil
}

// normalizeLockAccess при запуске от root выставляет лок-файлу целевую
// группу общего кэша (CacheShareGroup) и режим 0660, чтобы члены группы
// могли брать блокировку после pull/сборки от root (RepoDir — общий каталог
// группы). Родительский каталог получает группу только если его только что
// создал захват блокировки — существующие каталоги не трогаем. Ошибки
// нормализации не фатальны: на системах без целевой группы root продолжает
// работать. Реентерабельный путь (refs++) сюда не попадает — файл уже
// нормализован при первом захвате.
func normalizeLockAccess(path, dir string, dirCreated bool) {
	if os.Geteuid() != 0 {
		return
	}

	gid, err := privilegedGID()
	if err != nil {
		slog.Debug("Lock: не удалось определить целевую группу кэша", "error", err)
		return
	}

	if err := os.Chown(path, 0, gid); err != nil {
		slog.Debug("Lock: не удалось сменить группу лок-файла", "path", path, "error", err)
	}
	if err := os.Chmod(path, 0o660); err != nil {
		slog.Debug("Lock: не удалось сменить права лок-файла", "path", path, "error", err)
	}

	if dirCreated {
		if err := os.Chown(dir, 0, gid); err != nil {
			slog.Debug("Lock: не удалось сменить группу каталога блокировки", "dir", dir, "error", err)
		}
	}
}
