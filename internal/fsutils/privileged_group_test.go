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
	"os"
	"os/user"
	"strconv"
	"testing"
)

// Тесты подменяют lookupGroup и не параллельны: глобальное состояние
// восстанавливается через t.Cleanup.

func stubLookupGroup(t *testing.T, fn func(name string) (*user.Group, error)) {
	t.Helper()
	orig := lookupGroup
	lookupGroup = fn
	t.Cleanup(func() { lookupGroup = orig })
}

func TestCacheShareGroupPrefersAlr(t *testing.T) {
	stubLookupGroup(t, func(name string) (*user.Group, error) {
		if name == "alr" {
			return &user.Group{Gid: "999", Name: "alr"}, nil
		}
		return nil, errors.New("группа не найдена")
	})

	if got := CacheShareGroup(); got != "alr" {
		t.Errorf("CacheShareGroup() = %q, ожидалось %q", got, "alr")
	}
}

func TestCacheShareGroupFallbackToPrivileged(t *testing.T) {
	stubLookupGroup(t, func(name string) (*user.Group, error) {
		if name == "alr" {
			return nil, errors.New("группа не найдена")
		}
		return user.LookupGroup(name)
	})

	want := GetPrivilegedGroup()
	if got := CacheShareGroup(); got != want {
		t.Errorf("CacheShareGroup() = %q, ожидался fallback %q", got, want)
	}
}

func TestPrivilegedGIDUsesCacheShareGroup(t *testing.T) {
	stubLookupGroup(t, func(name string) (*user.Group, error) {
		if name == "alr" {
			return &user.Group{Gid: "4242", Name: "alr"}, nil
		}
		return nil, errors.New("группа не найдена")
	})

	gid, err := privilegedGID()
	if err != nil {
		t.Fatalf("privilegedGID() вернул ошибку: %s", err)
	}
	if gid != 4242 {
		t.Errorf("privilegedGID() = %d, ожидался gid группы alr (4242)", gid)
	}
}

func TestCurrentUserGroupGIDs(t *testing.T) {
	gids, err := CurrentUserGroupGIDs()
	if err != nil {
		t.Fatalf("CurrentUserGroupGIDs() вернул ошибку: %s", err)
	}

	// Основная группа процесса обязана присутствовать
	primary := uint32(os.Getegid())
	if !gids[primary] {
		t.Errorf("основная gid %d отсутствует в множестве %v", primary, gids)
	}

	// Множество должно совпадать с тем, что сообщает ОС о текущем пользователе
	cur, err := user.Current()
	if err != nil {
		t.Skipf("user.Current() недоступен: %s", err)
	}
	list, err := cur.GroupIds()
	if err != nil {
		t.Skipf("GroupIds() недоступен: %s", err)
	}
	for _, s := range list {
		gid, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			continue
		}
		if !gids[uint32(gid)] {
			t.Errorf("gid %d из GroupIds отсутствует в множестве %v", gid, gids)
		}
	}
}

func TestDirSharedWith(t *testing.T) {
	myGID := uint32(os.Getgid())
	myGIDs := map[uint32]bool{myGID: true}

	makeDir := func(t *testing.T, mode os.FileMode) string {
		t.Helper()
		dir := t.TempDir()
		sub := dir + "/sub"
		if err := os.Mkdir(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		// chown в свою группу разрешён без root
		if err := os.Chown(sub, os.Getuid(), int(myGID)); err != nil {
			t.Fatalf("chown: %s", err)
		}
		if err := os.Chmod(sub, mode); err != nil {
			t.Fatalf("chmod: %s", err)
		}
		return sub
	}

	t.Run("корректный общий каталог", func(t *testing.T) {
		dir := makeDir(t, os.ModeSetgid|0o0775)
		if !dirSharedWith(dir, myGIDs) {
			t.Error("ожидалось true для каталога 2775 в моей группе")
		}
	})

	t.Run("без setgid", func(t *testing.T) {
		dir := makeDir(t, 0o0775)
		if dirSharedWith(dir, myGIDs) {
			t.Error("ожидалось false без setgid-бита")
		}
	})

	t.Run("группа без полных прав", func(t *testing.T) {
		dir := makeDir(t, os.ModeSetgid|0o0755)
		if dirSharedWith(dir, myGIDs) {
			t.Error("ожидалось false без group rwx")
		}
	})

	t.Run("чужая группа", func(t *testing.T) {
		dir := makeDir(t, os.ModeSetgid|0o0775)
		otherGIDs := map[uint32]bool{myGID + 100000: true}
		if dirSharedWith(dir, otherGIDs) {
			t.Error("ожидалось false для gid не из множества")
		}
	})

	t.Run("не каталог", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "file")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		if dirSharedWith(f.Name(), myGIDs) {
			t.Error("ожидалось false для обычного файла")
		}
	})

	t.Run("несуществующий путь", func(t *testing.T) {
		if dirSharedWith("/nonexistent/path", myGIDs) {
			t.Error("ожидалось false для несуществующего пути")
		}
	})
}
