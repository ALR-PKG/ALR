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

// Lock — handle удерживаемой блокировки. Close уменьшает счётчик ссылок;
// реальное снятие flock происходит при Close последнего handle на путь
type Lock struct {
	path   string
	closed bool
}

// Close освобождает блокировку. Повторный вызов — no-op.
func (l *Lock) Close() error {
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

// TryLock создаёт (при необходимости) файл path и берёт exclusive
// non-blocking flock(2). Возвращённый Lock снимает блокировку при Close.
// В пределах процесса вызов реентерабелен: повторный TryLock того же пути
// возвращает новый handle на тот же flock.
// Если блокировку уже держит другой процесс — ErrLocked.
func TryLock(path string) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o2775); err != nil {
		return nil, fmt.Errorf("не удалось создать каталог для файла блокировки %s: %w", path, err)
	}

	locksMu.Lock()
	defer locksMu.Unlock()

	if entry, ok := heldLocks[path]; ok {
		entry.refs++
		return &Lock{path: path}, nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o664)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл блокировки %s: %w", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("не удалось заблокировать %s: %w", path, err)
	}

	heldLocks[path] = &lockEntry{file: f, refs: 1}
	return &Lock{path: path}, nil
}
