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
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakePropSetter - тестовый шов для DBusJob.properties
type fakePropSetter struct {
	iface string
	name  string
	value dbus.Variant
	calls int
}

func (f *fakePropSetter) Set(iface, name string, value dbus.Variant) *dbus.Error {
	f.calls++
	f.iface = iface
	f.name = name
	f.value = value
	return nil
}

func TestJobSetStatusUpdatesProperties(t *testing.T) {
	svc := NewService()
	job := NewDBusJob(svc, 1, JobTypeInstall, "testpkg", "testrepo")
	fp := &fakePropSetter{}
	job.properties = fp

	job.SetStatus(JobStatusRunning)

	if fp.calls == 0 {
		t.Fatal("expected properties.Set to be called")
	}
	if fp.iface != JobInterfaceName {
		t.Errorf("Set got iface %q, want %q", fp.iface, JobInterfaceName)
	}
	if fp.name != "Status" {
		t.Errorf("Set got name %q, want %q", fp.name, "Status")
	}
	if got, ok := fp.value.Value().(string); !ok || got != string(JobStatusRunning) {
		t.Errorf("Set got value %v, want %q", fp.value.Value(), JobStatusRunning)
	}
}

func TestJobSetProgressUpdatesProperties(t *testing.T) {
	svc := NewService()
	job := NewDBusJob(svc, 1, JobTypeInstall, "testpkg", "testrepo")
	fp := &fakePropSetter{}
	job.properties = fp

	job.SetProgress(0.5, "working")

	if fp.calls != 2 {
		t.Fatalf("expected 2 Set calls (Progress, Message), got %d", fp.calls)
	}
	if fp.name != "Message" {
		t.Errorf("last Set got name %q, want %q", fp.name, "Message")
	}
	if got, ok := fp.value.Value().(string); !ok || got != "working" {
		t.Errorf("Set got value %v, want %q", fp.value.Value(), "working")
	}
}

func TestPolkitSubjectMarshalsAsStruct(t *testing.T) {
	subject := newPolkitSubject(":1.42")

	// polkit ждет subject как ОДНУ структуру (s a{sv}), а не два аргумента
	sig := dbus.SignatureOf(subject)
	if sig.String() != "(sa{sv})" {
		t.Errorf("subject signature = %q, want %q", sig.String(), "(sa{sv})")
	}
	if subject.Kind != "system-bus-name" {
		t.Errorf("subject kind = %q, want %q", subject.Kind, "system-bus-name")
	}
	nameVar, ok := subject.Details["name"]
	if !ok {
		t.Fatal("subject details missing name")
	}
	if got, ok := nameVar.Value().(string); !ok || got != ":1.42" {
		t.Errorf("subject name = %v, want :1.42", nameVar.Value())
	}
}
