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

	"github.com/godbus/dbus/v5"
)

// getConnectionUnixUser - шов для подмены в тестах; возвращает uid владельца
// D-Bus соединения по его unique name (sender).
// ВНИМАНИЕ: тесты, подменяющие эту переменную, НЕ должны использовать
// t.Parallel() — подмена глобальна для процесса.
var getConnectionUnixUser = func(conn *dbus.Conn, sender dbus.Sender) (uint32, error) {
	if conn == nil {
		return 0, fmt.Errorf("no D-Bus connection")
	}
	var uid uint32
	err := conn.BusObject().Call(
		"org.freedesktop.DBus.GetConnectionUnixUser", 0, string(sender),
	).Store(&uid)
	if err != nil {
		return 0, fmt.Errorf("GetConnectionUnixUser failed for %q: %w", sender, err)
	}
	return uid, nil
}

// resolveCallerUID определяет unix uid вызвавшего по sender сообщения
func resolveCallerUID(conn *dbus.Conn, sender dbus.Sender) (uint32, error) {
	if sender == "" {
		return 0, fmt.Errorf("empty message sender")
	}
	return getConnectionUnixUser(conn, sender)
}
