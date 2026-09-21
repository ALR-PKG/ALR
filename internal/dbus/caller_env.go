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

package dbus

import (
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	nobodyUID       = 65534
	nobodyUsername  = "nobody"
	systemCacheRoot = "/var/cache/alr"
)

// lookupUser вынесено в переменную для подмены в тестах
var lookupUser = user.LookupId

// callerIdentity возвращает home и username вызвавшего по uid. nobody
// (и любой uid без passwd-записи) отображается на системный кэш:
// у nobody нет домашнего каталога, а его uid уже доверен в общем кэше.
func callerIdentity(uid uint32) (home, username string) {
	if uid != nobodyUID {
		u, err := lookupUser(strconv.Itoa(int(uid)))
		if err == nil {
			return u.HomeDir, u.Username
		}
		slog.Debug("no passwd entry for caller uid, falling back to nobody", "uid", uid, "error", err)
	}
	return systemCacheRoot, nobodyUsername
}

// callerEnv строит окружение plugin-процесса для сборки от имени
// вызвавшего (аналог setCommonCmdEnv, но не от текущего процесса).
func callerEnv(uid uint32) ([]string, error) {
	home, username := callerIdentity(uid)

	env := []string{
		"HOME=" + home,
		"LOGNAME=" + username,
		"USER=" + username,
		"PATH=/usr/bin:/bin:/usr/local/bin",
	}
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "LANG=") ||
			strings.HasPrefix(e, "LANGUAGE=") ||
			strings.HasPrefix(e, "LC_") ||
			strings.HasPrefix(e, "ALR_LOG_LEVEL=") {
			env = append(env, e)
		}
	}
	return env, nil
}

// callerCachePaths возвращает каталоги кэша сборки вызвавшего:
// <home>/.cache/alr/build|pkgs; для nobody — системный кэш.
func callerCachePaths(uid uint32) (buildDir, pkgsDir string) {
	home, _ := callerIdentity(uid)
	if home == systemCacheRoot {
		return filepath.Join(systemCacheRoot, "build"), filepath.Join(systemCacheRoot, "pkgs")
	}
	return filepath.Join(home, ".cache", "alr", "build"),
		filepath.Join(home, ".cache", "alr", "pkgs")
}
