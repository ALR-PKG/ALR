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
	"reflect"
	"testing"
)

func findMount(l *Layout, target string) *Mount {
	for i := range l.Mounts {
		if l.Mounts[i].Target == target {
			return &l.Mounts[i]
		}
	}
	return nil
}

func TestLayoutFromPathsBasic(t *testing.T) {
	l := LayoutFromPaths("/home/user/.cache/alr/build", "/home/user/.cache/alr/pkgs", "/home/user/proj")

	for _, rw := range []string{"/home/user/.cache/alr/build", "/home/user/.cache/alr/pkgs", "/tmp"} {
		m := findMount(l, rw)
		if m == nil {
			t.Errorf("rw-путь %s отсутствует в макете", rw)
		} else if !m.RW {
			t.Errorf("путь %s должен быть rw", rw)
		}
	}

	for _, ro := range []string{"/home/user/proj", "/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc"} {
		m := findMount(l, ro)
		if m == nil {
			t.Errorf("ro-путь %s отсутствует в макете", ro)
		} else if m.RW {
			t.Errorf("путь %s должен быть ro", ro)
		}
	}

	wantDev := []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"}
	if !reflect.DeepEqual(l.DevNodes, wantDev) {
		t.Errorf("DevNodes = %v, ожидалось %v", l.DevNodes, wantDev)
	}
	if !l.ProcFresh {
		t.Error("ProcFresh должен быть true")
	}
}

func TestLayoutFromPathsNestedRW(t *testing.T) {
	// BuildDir внутри PkgsDir: вложенный путь не дублируется
	l := LayoutFromPaths("/cache/pkgs/build", "/cache/pkgs", "/proj")
	if findMount(l, "/cache/pkgs/build") != nil {
		t.Error("вложенный BuildDir не должен дублироваться, когда PkgsDir — его родитель")
	}
	m := findMount(l, "/cache/pkgs")
	if m == nil || !m.RW {
		t.Error("PkgsDir должен остаться rw")
	}

	// PkgsDir внутри BuildDir: наоборот
	l = LayoutFromPaths("/cache/build", "/cache/build/pkgs", "/proj")
	if findMount(l, "/cache/build/pkgs") != nil {
		t.Error("вложенный PkgsDir не должен дублироваться, когда BuildDir — его родитель")
	}
	m = findMount(l, "/cache/build")
	if m == nil || !m.RW {
		t.Error("BuildDir должен остаться rw")
	}
}

func TestLayoutFromPathsScriptDirOnRWPath(t *testing.T) {
	// ScriptDir совпадает с rw-путём → rw побеждает, ro-записи нет
	l := LayoutFromPaths("/home/user/proj", "/cache/pkgs", "/home/user/proj")
	m := findMount(l, "/home/user/proj")
	if m == nil {
		t.Fatal("путь ScriptDir отсутствует")
	}
	if !m.RW {
		t.Error("при совпадении ScriptDir с rw-путём rw должен победить")
	}
	count := 0
	for _, mnt := range l.Mounts {
		if mnt.Target == "/home/user/proj" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("ожидалась ровно одна запись для пути, получено %d", count)
	}
}

func TestLayoutFromPathsScriptDirNestedInRW(t *testing.T) {
	// ScriptDir внутри rw-дерева → отдельная ro-запись не нужна
	l := LayoutFromPaths("/home/user/proj", "/cache/pkgs", "/home/user/proj/subdir")
	if findMount(l, "/home/user/proj/subdir") != nil {
		t.Error("ScriptDir внутри rw-дерева не должен давать отдельную ro-запись")
	}
}

func TestLayoutJSONRoundTrip(t *testing.T) {
	l := LayoutFromPaths("/cache/build", "/cache/pkgs", "/proj")
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("маршалинг: %v", err)
	}
	var got Layout
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("унмаршалинг: %v", err)
	}
	if !reflect.DeepEqual(*l, got) {
		t.Errorf("после JSON round-trip макет изменился:\nбыло:  %+v\nстало: %+v", *l, got)
	}
}

func TestLayoutMountsParentsFirst(t *testing.T) {
	// rw внутри ro-родителя (BuildDir в /usr/local): родитель должен идти раньше
	l := LayoutFromPaths("/usr/local/build", "/cache/pkgs", "/proj")
	idx := map[string]int{}
	for i, m := range l.Mounts {
		idx[m.Target] = i
	}
	if _, ok := idx["/usr/local/build"]; !ok {
		t.Fatal("rw-путь внутри ro-родителя должен сохраниться")
	}
	if idx["/usr"] > idx["/usr/local/build"] {
		t.Error("родительский mount должен идти раньше вложенного")
	}
}
