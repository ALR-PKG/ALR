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

package constants

const (
	SystemConfigPath = "/etc/alr/alr.toml"
	UserConfigPath   = "~/.config/alr/alr.toml"
	SystemCachePath  = "/var/cache/alr"
	TempDir          = "/tmp/alr"
	// DefaultSiteURL — адрес сайта ALR по умолчанию
	DefaultSiteURL = "https://alr-pkg.ru"
	// PrivilegedGroup - устарело, используйте GetPrivilegedGroup()
	PrivilegedGroup = "wheel" // оставлено для обратной совместимости
)
