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

package dbus

import (
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

// validDBusInterfaceName проверяет имя интерфейса по спецификации D-Bus:
// минимум два элемента через точку, элементы только [A-Za-z0-9_],
// не начинаются с цифры, не пустые.
func validDBusInterfaceName(name string) bool {
	elems := strings.Split(name, ".")
	if len(elems) < 2 {
		return false
	}
	for _, e := range elems {
		if e == "" {
			return false
		}
		for i, r := range e {
			ok := r == '_' ||
				(r >= 'a' && r <= 'z') ||
				(r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9' && i > 0)
			if !ok {
				return false
			}
		}
	}
	return true
}

func TestInterfaceNamesAreValid(t *testing.T) {
	for name, iface := range map[string]string{
		"ManagerInterfaceName": ManagerInterfaceName,
		"PackageInterfaceName": PackageInterfaceName,
		"JobInterfaceName":     JobInterfaceName,
	} {
		if !validDBusInterfaceName(iface) {
			t.Errorf("%s = %q is not a valid D-Bus interface name", name, iface)
		}
	}
}

func TestErrorNamesAreValid(t *testing.T) {
	// Имена ошибок подчиняются тем же правилам, что и имена интерфейсов.
	for _, errName := range []string{
		ErrorNotAuthorized,
		ErrorNotImplemented,
		ErrorNotInitialized,
		ErrorInternal,
		ErrorSearchFailed,
		ErrorPackageNotFound,
		ErrorJobAlreadyFinished,
	} {
		if !validDBusInterfaceName(errName) {
			t.Errorf("error name %q is not a valid D-Bus error name", errName)
		}
	}
}

func TestGetPackageObjectPathSanitizesInvalidChars(t *testing.T) {
	path := GetPackageObjectPath("alr-repo", "some-pkg")
	if !path.IsValid() {
		t.Errorf("GetPackageObjectPath produced invalid path: %q", path)
	}
	if strings.Contains(string(path), "-") {
		t.Errorf("path contains hyphen: %q", path)
	}
}

func TestGetJobObjectPathUsesDecimalDigits(t *testing.T) {
	path := GetJobObjectPath(1)
	if !path.IsValid() {
		t.Fatalf("GetJobObjectPath produced invalid path: %q", path)
	}
	expected := DBusObjectPath + "/jobs/1"
	if string(path) != expected {
		t.Errorf("GetJobObjectPath(1) = %q, want %q", path, expected)
	}

	path = GetJobObjectPath(12345)
	if !path.IsValid() {
		t.Fatalf("GetJobObjectPath produced invalid path: %q", path)
	}
	if !strings.HasSuffix(string(path), "/jobs/12345") {
		t.Errorf("GetJobObjectPath(12345) = %q, want suffix /jobs/12345", path)
	}
}

func TestJobObjectPathToID(t *testing.T) {
	for _, tc := range []struct {
		path string
		want uint32
	}{
		{"/ru/alr_pkg/ALR/jobs/123", 123},
		{"/ru/alr_pkg/ALR/jobs/1", 1},
		{string(GetJobObjectPath(424242)), 424242},
		{"/ru/alr_pkg/ALR/jobs/", 0},
		{"/ru/alr_pkg/ALR/jobs/abc", 0},
		{"", 0},
		{"/jobs", 0},
	} {
		if got := JobObjectPathToID(dbus.ObjectPath(tc.path)); got != tc.want {
			t.Errorf("JobObjectPathToID(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}
