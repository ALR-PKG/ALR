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
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

// fakeAuthorizer - тестовый авторизатор с записью параметров вызова
type fakeAuthorizer struct {
	allowed bool

	gotActionID string
	gotUID      uint32
	calls       int
}

func (f *fakeAuthorizer) CheckAuthorization(actionID string, uid uint32, details map[string]string, allowUserInteraction bool) (bool, error) {
	f.calls++
	f.gotActionID = actionID
	f.gotUID = uid
	return f.allowed, nil
}

// newTestService создает Service с подменёнными authorizer и резолвером uid
func newTestService(t *testing.T, auth Authorizer) *Service {
	t.Helper()

	oldResolver := getConnectionUnixUser
	getConnectionUnixUser = func(conn *dbus.Conn, sender dbus.Sender) (uint32, error) {
		if sender == "bad-sender" {
			return 0, errors.New("unknown sender")
		}
		return 1000, nil
	}
	t.Cleanup(func() { getConnectionUnixUser = oldResolver })

	svc := NewService()
	svc.authorizer = auth
	return svc
}

func newTestPackage(svc *Service) *DBusPackage {
	return &DBusPackage{
		service:    svc,
		name:       "testpkg",
		repository: "testrepo",
	}
}

func TestInstallNotAuthorized(t *testing.T) {
	auth := &fakeAuthorizer{allowed: false}
	svc := newTestService(t, auth)
	pkg := newTestPackage(svc)

	path, derr := pkg.Install("1.23", map[string]dbus.Variant{})
	if derr == nil {
		t.Fatal("expected NotAuthorized error, got nil")
	}
	if derr.Name != ErrorNotAuthorized {
		t.Errorf("unexpected error name: %s", derr.Name)
	}
	if path != "" {
		t.Errorf("expected empty job path, got %q", path)
	}
	if len(svc.jobs) != 0 {
		t.Errorf("job must not be created on auth failure, got %d jobs", len(svc.jobs))
	}
	if auth.calls != 1 {
		t.Errorf("expected 1 authorizer call, got %d", auth.calls)
	}
	if auth.gotUID != 1000 {
		t.Errorf("authorizer got uid %d, want 1000", auth.gotUID)
	}
	if auth.gotActionID != ActionInstall {
		t.Errorf("authorizer got action %q, want %q", auth.gotActionID, ActionInstall)
	}
}

func TestInstallAuthorized(t *testing.T) {
	auth := &fakeAuthorizer{allowed: true}
	svc := newTestService(t, auth)
	pkg := newTestPackage(svc)

	path, derr := pkg.Install("1.23", map[string]dbus.Variant{})
	if derr != nil {
		t.Fatalf("expected success, got error: %v", derr)
	}
	if path == "" {
		t.Error("expected non-empty job path")
	}
	if len(svc.jobs) != 1 {
		t.Errorf("expected 1 job, got %d", len(svc.jobs))
	}
	if auth.gotUID != 1000 {
		t.Errorf("authorizer got uid %d, want 1000", auth.gotUID)
	}
	if auth.gotActionID != ActionInstall {
		t.Errorf("authorizer got action %q, want %q", auth.gotActionID, ActionInstall)
	}
}

func TestInstallSenderResolveFailure(t *testing.T) {
	auth := &fakeAuthorizer{allowed: true}
	svc := newTestService(t, auth)
	pkg := newTestPackage(svc)

	_, derr := pkg.Install("bad-sender", map[string]dbus.Variant{})
	if derr == nil {
		t.Fatal("expected NotAuthorized error, got nil")
	}
	if derr.Name != ErrorNotAuthorized {
		t.Errorf("unexpected error name: %s", derr.Name)
	}
	if auth.calls != 0 {
		t.Errorf("authorizer must not be called when sender resolution fails, got %d calls", auth.calls)
	}
	if len(svc.jobs) != 0 {
		t.Errorf("job must not be created on resolve failure, got %d jobs", len(svc.jobs))
	}
}

func TestRemoveNotAuthorized(t *testing.T) {
	auth := &fakeAuthorizer{allowed: false}
	svc := newTestService(t, auth)
	pkg := newTestPackage(svc)

	_, derr := pkg.Remove("1.23", map[string]dbus.Variant{})
	if derr == nil {
		t.Fatal("expected NotAuthorized error, got nil")
	}
	if derr.Name != ErrorNotAuthorized {
		t.Errorf("unexpected error name: %s", derr.Name)
	}
	if auth.gotActionID != ActionRemove {
		t.Errorf("authorizer got action %q, want %q", auth.gotActionID, ActionRemove)
	}
	if len(svc.jobs) != 0 {
		t.Errorf("job must not be created on auth failure, got %d jobs", len(svc.jobs))
	}
}

func TestRefreshRepositoriesNotAuthorized(t *testing.T) {
	auth := &fakeAuthorizer{allowed: false}
	svc := newTestService(t, auth)
	mgr := &DBusManager{service: svc}

	_, derr := mgr.RefreshRepositories("1.23")
	if derr == nil {
		t.Fatal("expected NotAuthorized error, got nil")
	}
	if derr.Name != ErrorNotAuthorized {
		t.Errorf("unexpected error name: %s", derr.Name)
	}
	if auth.gotActionID != ActionRefresh {
		t.Errorf("authorizer got action %q, want %q", auth.gotActionID, ActionRefresh)
	}
	if auth.gotUID != 1000 {
		t.Errorf("authorizer got uid %d, want 1000", auth.gotUID)
	}
	if len(svc.jobs) != 0 {
		t.Errorf("job must not be created on auth failure, got %d jobs", len(svc.jobs))
	}
}

func TestRefreshRepositoriesAuthorized(t *testing.T) {
	auth := &fakeAuthorizer{allowed: true}
	svc := newTestService(t, auth)
	mgr := &DBusManager{service: svc}

	path, derr := mgr.RefreshRepositories("1.23")
	// Сервис без deps: после успешной авторизации ожидаем NotInitialized,
	// но не NotAuthorized
	if derr != nil && derr.Name == ErrorNotAuthorized {
		t.Fatalf("expected auth to pass, got: %v", derr)
	}
	if derr == nil && path == "" {
		t.Error("expected non-empty job path")
	}
	if auth.gotActionID != ActionRefresh {
		t.Errorf("authorizer got action %q, want %q", auth.gotActionID, ActionRefresh)
	}
	if auth.gotUID != 1000 {
		t.Errorf("authorizer got uid %d, want 1000", auth.gotUID)
	}
}

func TestFallbackCheckOnlyRoot(t *testing.T) {
	p := &PolicyKitAuthorizer{}
	if !p.fallbackCheck(0) {
		t.Error("fallback must allow uid 0")
	}
	if p.fallbackCheck(1000) {
		t.Error("fallback must deny uid 1000")
	}
}
