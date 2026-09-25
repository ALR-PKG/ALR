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

package cmd

import (
	"regexp"
	"strings"

	"github.com/leonelquinteros/gotext"
)

var (
	// Форматы ошибок cobra/pflag, которые нужно локализовать
	unknownCommandRe   = regexp.MustCompile(`^unknown command "([^"]*)" for "([^"]*)"`)
	unknownFlagRe      = regexp.MustCompile(`^unknown flag: (--\S+)`)
	unknownShorthandRe = regexp.MustCompile(`^unknown shorthand flag: '(.)' in (-\S+)`)
)

// LocalizeExecuteError переводит известные сообщения об ошибках cobra/pflag,
// неизвестные возвращает как есть
func LocalizeExecuteError(err error) string {
	msg := err.Error()

	if m := unknownCommandRe.FindStringSubmatch(msg); m != nil {
		rest := unknownCommandRe.ReplaceAllString(msg, "")
		rest = strings.ReplaceAll(rest, "Did you mean this?", gotext.Get("Did you mean this?"))
		return gotext.Get("unknown command \"%s\" for \"%s\"", m[1], m[2]) + rest
	}

	if m := unknownFlagRe.FindStringSubmatch(msg); m != nil {
		return gotext.Get("unknown flag: %s", m[1])
	}

	if m := unknownShorthandRe.FindStringSubmatch(msg); m != nil {
		return gotext.Get("unknown shorthand flag: '%s' in %s", m[1], m[2])
	}

	return msg
}
