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
	"fmt"
	"log/slog"

	"github.com/godbus/dbus/v5"
)

const (
	// PolicyKitInterface - PolicyKit authority interface
	PolicyKitInterface = "org.freedesktop.PolicyKit1"
	// PolicyKitPath - object path для PolicyKit
	PolicyKitPath = "/org/freedesktop/PolicyKit1/Authority"
)

// PolicyKitActions
const (
	ActionInstall = "ru.alr-pkg.install"
	ActionRemove  = "ru.alr-pkg.remove"
	ActionBuild   = "ru.alr-pkg.build"
	ActionRefresh = "ru.alr-pkg.refresh"
	ActionUpgrade = "ru.alr-pkg.upgrade"
)

// PolicyKitAuthorizer предоставляет интеграцию с PolicyKit
type PolicyKitAuthorizer struct {
	conn *dbus.Conn
}

// NewPolicyKitAuthorizer создает новый авторизатор
func NewPolicyKitAuthorizer(conn *dbus.Conn) *PolicyKitAuthorizer {
	return &PolicyKitAuthorizer{conn: conn}
}

// polkitSubject - subject вызова polkit CheckAuthorization: ОДНА структура
// (kind, details) — на шине маршалится как (sa{sv})
type polkitSubject struct {
	Kind    string
	Details map[string]dbus.Variant
}

// newPolkitSubject строит subject kind "unix-user" для указанного uid
func newPolkitSubject(uid uint32) polkitSubject {
	return polkitSubject{
		Kind: "unix-user",
		Details: map[string]dbus.Variant{
			"uid": dbus.MakeVariant(uid),
		},
	}
}

// CheckAuthorization проверяет авторизацию через PolicyKit для указанного
// uid (uid ВЫЗЫВАВШЕГО, а не демона). При недоступном или ошибочном polkit —
// fail-closed: разрешено только uid 0.
func (p *PolicyKitAuthorizer) CheckAuthorization(actionID string, uid uint32, details map[string]string, allowUserInteraction bool) (bool, error) {
	if p.conn == nil {
		return false, fmt.Errorf("no D-Bus connection")
	}

	// Проверяем доступность PolicyKit
	if !p.isPolicyKitAvailable() {
		slog.Debug("PolicyKit not available, falling back to root check")
		return p.fallbackCheck(uid), nil
	}

	// Детали авторизации
	actionDetails := map[string]string{}
	if details != nil {
		actionDetails = details
	}

	// Флаги
	flags := uint32(0)
	if allowUserInteraction {
		flags = 1 // CheckAuthorizationFlagsAllowUserInteraction
	}

	// Вызываем PolicyKit:
	// CheckAuthorization((sa{sv}) subject, s action_id, a{ss} details,
	// u flags, s cancellation_id) -> ((bba{ss}) result)
	obj := p.conn.Object(PolicyKitInterface, dbus.ObjectPath(PolicyKitPath))
	call := obj.Call(PolicyKitInterface+".Authority.CheckAuthorization", 0,
		newPolkitSubject(uid),
		actionID,
		actionDetails,
		flags,
		"", // cancellation ID
	)

	if call.Err != nil {
		slog.Error("PolicyKit check failed", "action", actionID, "uid", uid, "err", call.Err)
		return p.fallbackCheck(uid), nil
	}

	// Результат: (is_authorized, is_challenge, details)
	var result struct {
		IsAuthorized bool
		IsChallenge  bool
		Details      map[string]dbus.Variant
	}

	if err := call.Store(&result.IsAuthorized, &result.IsChallenge, &result.Details); err != nil {
		slog.Error("Failed to parse PolicyKit response", "err", err)
		return p.fallbackCheck(uid), nil
	}

	return result.IsAuthorized, nil
}

// isPolicyKitAvailable проверяет доступность PolicyKit
func (p *PolicyKitAuthorizer) isPolicyKitAvailable() bool {
	if p.conn == nil {
		return false
	}

	var names []string
	if err := p.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return false
	}

	for _, name := range names {
		if name == PolicyKitInterface {
			return true
		}
	}

	return false
}

// fallbackCheck - fallback проверка: без polkit разрешено только root (uid 0)
func (p *PolicyKitAuthorizer) fallbackCheck(uid uint32) bool {
	return uid == 0
}

// Authorizer интерфейс для авторизации
type Authorizer interface {
	CheckAuthorization(actionID string, uid uint32, details map[string]string, allowUserInteraction bool) (bool, error)
}

// DefaultAuthorizer возвращает авторизатор по умолчанию
func DefaultAuthorizer(conn *dbus.Conn) Authorizer {
	return NewPolicyKitAuthorizer(conn)
}
