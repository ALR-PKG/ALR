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

// lookupUser, lookupGroup и lookupGroupIds вынесены в переменные для
// подмены в тестах
var (
	lookupUser     = user.LookupId
	lookupGroup    = user.LookupGroup
	lookupGroupIds = func(u *user.User) ([]string, error) { return u.GroupIds() }
)

// callerInfo — идентичность вызвавшего: домашний каталог, имя, основная
// группа и supplementary groups (для Credential.Groups plugin-процесса).
type callerInfo struct {
	home     string
	username string
	gid      uint32
	// groups — все группы из NSS (GroupIds обычно включает и основную;
	// ядро дедуплицирует её с Credential.Gid). Пусто для nobody.
	groups []uint32
}

// nobodyGID резолвит gid группы nobody через NSS ("nobody", затем
// "nogroup" — имя группы различается между дистрибутивами); если группа
// не найдена, возвращает общеупотребительный 65534.
func nobodyGID() uint32 {
	for _, name := range []string{"nobody", "nogroup"} {
		g, err := lookupGroup(name)
		if err == nil {
			if gid, err := strconv.ParseUint(g.Gid, 10, 32); err == nil {
				return uint32(gid)
			}
		}
	}
	return nobodyUID
}

// callerGroups возвращает supplementary groups пользователя из NSS;
// при ошибке lookup — пустой список (лучше собрать без доп. групп,
// чем упасть целиком).
func callerGroups(u *user.User) []uint32 {
	ids, err := lookupGroupIds(u)
	if err != nil {
		slog.Debug("cannot resolve caller group ids, continuing without supplementary groups",
			"username", u.Username, "error", err)
		return nil
	}
	groups := make([]uint32, 0, len(ids))
	for _, id := range ids {
		if g, err := strconv.ParseUint(id, 10, 32); err == nil {
			groups = append(groups, uint32(g))
		}
	}
	return groups
}

// callerIdentity возвращает идентичность вызвавшего по uid. nobody
// (и любой uid без passwd-записи либо с пустым домашним каталогом)
// отображается на системный кэш: у nobody нет домашнего каталога,
// а его uid уже доверен в общем кэше.
func callerIdentity(uid uint32) callerInfo {
	if uid != nobodyUID {
		u, err := lookupUser(strconv.Itoa(int(uid)))
		if err == nil && u.HomeDir != "" {
			g, err := strconv.ParseUint(u.Gid, 10, 32)
			if err == nil {
				return callerInfo{
					home:     u.HomeDir,
					username: u.Username,
					gid:      uint32(g),
					groups:   callerGroups(u),
				}
			}
			slog.Debug("cannot parse caller gid, falling back to nobody", "uid", uid, "gid", u.Gid)
		} else if err == nil {
			slog.Debug("empty home dir for caller uid, falling back to nobody", "uid", uid)
		} else {
			slog.Debug("no passwd entry for caller uid, falling back to nobody", "uid", uid, "error", err)
		}
	}
	return callerInfo{home: systemCacheRoot, username: nobodyUsername, gid: nobodyGID()}
}

// callerEnv строит окружение plugin-процесса для сборки от имени
// вызвавшего (аналог setCommonCmdEnv, но не от текущего процесса).
func callerEnv(info callerInfo) ([]string, error) {
	env := []string{
		"HOME=" + info.home,
		"LOGNAME=" + info.username,
		"USER=" + info.username,
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
	home := callerIdentity(uid).home
	if home == systemCacheRoot {
		return filepath.Join(systemCacheRoot, "build"), filepath.Join(systemCacheRoot, "pkgs")
	}
	return filepath.Join(home, ".cache", "alr", "build"),
		filepath.Join(home, ".cache", "alr", "pkgs")
}
