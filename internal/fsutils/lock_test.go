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

package fsutils_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
)

// flockHeld проверяет, удерживается ли flock на path каким-либо
// open file description (в т.ч. другим fd этого же процесса)
func flockHeld(t *testing.T, path string) bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open %s: %s", path, err)
	}
	defer f.Close()
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err == nil {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		return false
	}
	return errors.Is(err, syscall.EWOULDBLOCK)
}

func TestTryLockCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected lock file to exist, got %s", err)
	}
}

func TestTryLockAlreadyLocked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()

	// В пределах процесса TryLock реентерабелен, поэтому ErrLocked
	// проверяем из отдельного процесса
	cmd := exec.Command(os.Args[0], "-test.run=TestTryLockHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_TRYLOCK_HELPER=1", "TRYLOCK_PATH="+path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("Expected ErrLocked in helper process, got: %s (%s)", out, err)
	}
}

// TestTryLockHelperProcess — не тест, а subprocess для TestTryLockAlreadyLocked
func TestTryLockHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_TRYLOCK_HELPER") != "1" {
		return
	}
	_, err := fsutils.TryLock(os.Getenv("TRYLOCK_PATH"))
	if errors.Is(err, fsutils.ErrLocked) {
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "expected ErrLocked, got %v", err)
	os.Exit(1)
}

func TestTryLockAfterClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Failed to close lock: %s", err)
	}
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after Close")
	}

	l2, err := fsutils.TryLock(path)
	if err != nil {
		t.Errorf("Expected no error after Close, got %s", err)
	} else {
		l2.Close()
	}
}

func TestTryLockReentrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l1, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}

	l2, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected reentrant TryLock to succeed, got %s", err)
	}

	if err := l1.Close(); err != nil {
		t.Fatalf("Failed to close first lock: %s", err)
	}
	if !flockHeld(t, path) {
		t.Error("Expected flock to still be held after first Close")
	}

	if err := l2.Close(); err != nil {
		t.Fatalf("Failed to close second lock: %s", err)
	}
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after last Close")
	}

	l3, err := fsutils.TryLock(path)
	if err != nil {
		t.Errorf("Expected no error after all locks closed, got %s", err)
	} else {
		l3.Close()
	}
}

func TestTryLockDoubleClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Failed to close lock: %s", err)
	}
	if err := l.Close(); err != nil {
		t.Errorf("Expected double Close to be a no-op, got %s", err)
	}
}

func TestTryLockConcurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				l, err := fsutils.TryLock(path)
				if err != nil {
					t.Errorf("Expected no error, got %s", err)
					return
				}
				if err := l.Close(); err != nil {
					t.Errorf("Failed to close lock: %s", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after all goroutines finished")
	}
}

func TestTryLockCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "test.lock")
	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Expected lock file to exist, got %s", err)
	}
}

// TestTryLockRootNormalizesPermissions проверяет, что при запуске от root
// лок-файл получает целевую группу общего кэша и режим 0660, а только что
// созданный родительский каталог — ту же группу. Иначе после
// pull от root пользователь из группы не сможет открыть лок-файл (EACCES)
func TestTryLockRootNormalizesPermissions(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("тест требует root: проверяется нормализация прав лок-файла")
	}

	grp, err := user.LookupGroup(fsutils.CacheShareGroup())
	if err != nil {
		t.Skipf("целевая группа кэша %s не найдена: %s", fsutils.CacheShareGroup(), err)
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		t.Fatalf("некорректный gid группы %s: %s", grp.Name, err)
	}

	dir := filepath.Join(t.TempDir(), "repo")
	path := filepath.Join(dir, "test.lock")

	l, err := fsutils.TryLock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	defer l.Close()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Failed to stat %s: %s", path, err)
	}
	if info.Mode().Perm() != 0o660 {
		t.Errorf("Expected lock file mode 0660, got %04o", info.Mode().Perm())
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Gid) != gid {
		t.Errorf("Expected lock file gid %d (%s), got %+v", gid, grp.Name, info.Sys())
	}

	dinfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Failed to stat %s: %s", dir, err)
	}
	if stat, ok := dinfo.Sys().(*syscall.Stat_t); !ok || int(stat.Gid) != gid {
		t.Errorf("Expected parent dir gid %d (%s), got %+v", gid, grp.Name, dinfo.Sys())
	}
}

// holdFlockViaProbe берёт flock на path через отдельный open file
// description (без реестра fsutils), имитируя блокировку другим процессом.
// Возвращает функцию снятия блокировки.
func holdFlockViaProbe(t *testing.T, path string) func() {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o664)
	if err != nil {
		t.Fatalf("Failed to open probe lock file %s: %s", path, err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		t.Fatalf("Failed to flock probe file %s: %s", path, err)
	}
	return func() {
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		f.Close()
	}
}

func TestLockBlocksUntilReleased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	release := holdFlockViaProbe(t, path)

	type result struct {
		l   *fsutils.LockHandle
		err error
	}
	done := make(chan result, 1)
	go func() {
		l, err := fsutils.Lock(path)
		done <- result{l, err}
	}()

	// Пока probe держит flock, Lock обязан ждать
	select {
	case r := <-done:
		r.l.Close()
		t.Fatal("Expected Lock to block while probe holds the flock, but it returned")
	case <-time.After(200 * time.Millisecond):
	}

	release()

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("Expected no error after release, got %s", r.err)
		}
		r.l.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("Expected Lock to acquire the flock after release")
	}

	if flockHeld(t, path) {
		t.Error("Expected flock to be released after Close")
	}
}

// TestLockDoesNotBlockTryLockOtherPath проверяет, что блокирующий Lock,
// ожидающий чужой flock, не останавливает операции реестра по другим путям:
// TryLock(pathB) обязан вернуться сразу, пока Lock(pathA) ждёт
func TestLockDoesNotBlockTryLockOtherPath(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.lock")
	pathB := filepath.Join(dir, "b.lock")
	release := holdFlockViaProbe(t, pathA)

	type result struct {
		l   *fsutils.LockHandle
		err error
	}
	lockDone := make(chan result, 1)
	go func() {
		l, err := fsutils.Lock(pathA)
		lockDone <- result{l, err}
	}()

	// Даём горутине дойти до блокирующего ожидания flock на pathA
	time.Sleep(200 * time.Millisecond)

	tryDone := make(chan result, 1)
	go func() {
		l, err := fsutils.TryLock(pathB)
		tryDone <- result{l, err}
	}()

	select {
	case r := <-tryDone:
		if r.err != nil {
			t.Fatalf("Expected no error from TryLock on other path, got %s", r.err)
		}
		r.l.Close()
	case <-time.After(2 * time.Second):
		// Освобождаем probe до Fatal, чтобы зависший Lock не
		// блокировал реестр для последующих тестов
		release()
		<-lockDone
		t.Fatal("TryLock on pathB blocked while Lock(pathA) was waiting: registry is stalled by blocking flock")
	}

	release()

	select {
	case r := <-lockDone:
		if r.err != nil {
			t.Fatalf("Expected no error from Lock after release, got %s", r.err)
		}
		r.l.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("Expected Lock to acquire the flock after release")
	}
}

// TestLockConcurrentSamePath проверяет, что конкурентные Lock одного пути
// делят один flock: refs==2, после Close первого handle блокировка держится,
// после Close второго — снята
func TestLockConcurrentSamePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	release := holdFlockViaProbe(t, path)

	start := make(chan struct{})
	type result struct {
		l   *fsutils.LockHandle
		err error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			l, err := fsutils.Lock(path)
			results <- result{l, err}
		}()
	}
	close(start)

	// Обе горутины должны упереться в flock probe-файла
	time.Sleep(200 * time.Millisecond)
	release()

	r1 := <-results
	r2 := <-results
	for _, r := range []result{r1, r2} {
		if r.err != nil {
			t.Fatalf("Expected no error, got %s", r.err)
		}
	}

	if err := r1.l.Close(); err != nil {
		t.Fatalf("Failed to close first lock: %s", err)
	}
	if !flockHeld(t, path) {
		t.Error("Expected flock to still be held after first Close (refs==2)")
	}

	if err := r2.l.Close(); err != nil {
		t.Fatalf("Failed to close second lock: %s", err)
	}
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after last Close")
	}
}

// TestTryLockDuringPendingSamePath проверяет, что TryLock по пути, который
// процесс уже захватывает (Lock ждёт чужой flock), не возвращает ложный
// ErrLocked: TryLock ждёт завершения захвата и присоединяется к нему
func TestTryLockDuringPendingSamePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	release := holdFlockViaProbe(t, path)

	type result struct {
		l   *fsutils.LockHandle
		err error
	}
	lockDone := make(chan result, 1)
	go func() {
		l, err := fsutils.Lock(path)
		lockDone <- result{l, err}
	}()

	// Даём Lock дойти до ожидания flock (захват в статусе pending)
	time.Sleep(200 * time.Millisecond)

	tryDone := make(chan result, 1)
	go func() {
		l, err := fsutils.TryLock(path)
		tryDone <- result{l, err}
	}()

	// Пока probe держит flock, TryLock обязан ждать, а не возвращать ErrLocked
	select {
	case r := <-tryDone:
		if r.l != nil {
			r.l.Close()
		}
		// Освобождаем probe до Fatal, чтобы зависший Lock не
		// блокировал реестр для последующих тестов
		release()
		<-lockDone
		t.Fatalf("Expected TryLock to wait for the pending acquisition, got immediate result (err=%v)", r.err)
	case <-time.After(200 * time.Millisecond):
	}

	release()

	select {
	case r := <-tryDone:
		if r.err != nil {
			t.Fatalf("Expected TryLock to join the acquired lock, got %s", r.err)
		}
		r.l.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("Expected TryLock to return after the pending acquisition completed")
	}

	select {
	case r := <-lockDone:
		if r.err != nil {
			t.Fatalf("Expected no error from Lock, got %s", r.err)
		}
		r.l.Close()
	case <-time.After(5 * time.Second):
		t.Fatal("Expected Lock to acquire the flock after release")
	}

	if flockHeld(t, path) {
		t.Error("Expected flock to be released after all handles closed")
	}
}

// TestTryLockHeldByProbe проверяет маппинг ErrLocked без ожидания, когда
// flock держит другой open file description и pending-захвата нет
func TestTryLockHeldByProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	release := holdFlockViaProbe(t, path)
	defer release()

	done := make(chan error, 1)
	go func() {
		l, err := fsutils.TryLock(path)
		if err == nil {
			l.Close()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, fsutils.ErrLocked) {
			t.Errorf("Expected ErrLocked, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Expected TryLock to return ErrLocked immediately")
	}
}

func TestLockReentrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.lock")
	l1, err := fsutils.Lock(path)
	if err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}

	l2, err := fsutils.Lock(path)
	if err != nil {
		t.Fatalf("Expected reentrant Lock to succeed, got %s", err)
	}

	if err := l1.Close(); err != nil {
		t.Fatalf("Failed to close first lock: %s", err)
	}
	if !flockHeld(t, path) {
		t.Error("Expected flock to still be held after first Close")
	}

	if err := l2.Close(); err != nil {
		t.Fatalf("Failed to close second lock: %s", err)
	}
	if flockHeld(t, path) {
		t.Error("Expected flock to be released after last Close")
	}

	l3, err := fsutils.Lock(path)
	if err != nil {
		t.Errorf("Expected no error after all locks closed, got %s", err)
	} else {
		l3.Close()
	}
}
