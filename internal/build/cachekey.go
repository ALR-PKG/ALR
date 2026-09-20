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

package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"

	"mvdan.cc/sh/v3/syntax"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

// buildScriptFuncs — функции alr.sh, влияющие на содержимое пакета.
// version() влияет на кэш через поле Version, meta_*() — через поля Package,
// поэтому в ключ не входят. Помимо перечисленных, в ключ входят функции
// подпакетов package_<name> — их тела формируют содержимое подпакетов.
var buildScriptFuncs = []string{"prepare", "build", "package"}

type scriptFuncText struct {
	name string
	text string
}

// ScriptFuncsKey возвращает sha256 от канонических текстов функций
// prepare/build/package в скрипте. Принтер не сохраняет комментарии и
// нормализует форматирование (отступы, «;» против перевода строки), поэтому
// косметические правки скрипта ключ не меняют. Формы `f() { ...; }` и
// `f() ( ... )` печатаются по-разному — сознательно: subshell изолирует
// переменные и рабочий каталог, это семантически другая функция.
// Функции хешируются в отсортированном по имени порядке, чтобы ключ не
// зависел от порядка обхода AST; повторные объявления одной функции
// сохраняются в порядке объявления (stable sort).
func ScriptFuncsKey(sf *alrsh.ScriptFile) string {
	printer := syntax.NewPrinter()
	var funcs []scriptFuncText

	syntax.Walk(sf.File(), func(node syntax.Node) bool {
		fd, ok := node.(*syntax.FuncDecl)
		if !ok {
			return true
		}
		if !slices.Contains(buildScriptFuncs, fd.Name.Value) &&
			!strings.HasPrefix(fd.Name.Value, "package_") {
			return true
		}
		var sb strings.Builder
		// Print по *syntax.FuncDecl не может вернуть ошибку на bytes-подобном writer
		_ = printer.Print(&sb, fd)
		funcs = append(funcs, scriptFuncText{name: fd.Name.Value, text: sb.String()})
		return true
	})

	slices.SortStableFunc(funcs, func(a, b scriptFuncText) int {
		return strings.Compare(a.name, b.name)
	})

	h := sha256.New()
	for _, fn := range funcs {
		h.Write([]byte(fn.name))
		h.Write([]byte{0})
		h.Write([]byte(fn.text))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// semanticCachePayload — сборочно-значимые поля пакета. Поля-метаданные
// (Summary, Description, Group, Homepage, Maintainer, Licenses) сюда не
// входят: их правка не влияет на содержимое пакета. Идентификаторы
// (Repository, Name, BasePkgName) и служебные поля (AlrShURL, AlrShSHA256,
// CachedAt) тоже не входят: имя и репозиторий уже закодированы в имени файла
// пакета. Порядок элементов в списках значим — перестановка зависимостей
// инвалидирует кэш (безопасная избыточная инвалидация).
type semanticCachePayload struct {
	Version          string            `json:"version"`
	Release          int               `json:"release"`
	Epoch            uint              `json:"epoch"`
	Architectures    []string          `json:"architectures"`
	Provides         []string          `json:"provides"`
	Conflicts        []string          `json:"conflicts"`
	Replaces         []string          `json:"replaces"`
	Depends          []string          `json:"depends"`
	BuildDepends     []string          `json:"build_depends"`
	OptDepends       []string          `json:"opt_depends"`
	Sources          []string          `json:"sources"`
	Checksums        []string          `json:"checksums"`
	Backup           []string          `json:"backup"`
	Scripts          alrsh.Scripts     `json:"scripts"`
	AutoReq          []string          `json:"auto_req"`
	AutoProv         []string          `json:"auto_prov"`
	AutoReqSkipList  []string          `json:"auto_req_skiplist"`
	AutoProvSkipList []string          `json:"auto_prov_skiplist"`
	FireJailed       bool              `json:"firejailed"`
	FireJailProfiles map[string]string `json:"firejail_profiles"`
	ExtraFiles       []alrsh.ExtraFile `json:"extra_files"`
	FuncsKey         string            `json:"funcs_key"`
}

// SemanticCacheKey возвращает sha256 от JSON-представления сборочно-значимых
// полей пакета (resolved-значения OverridableField) и ключа функций скрипта.
func SemanticCacheKey(vars *alrsh.Package, funcsKey string) string {
	payload := semanticCachePayload{
		Version:          vars.Version,
		Release:          vars.Release,
		Epoch:            vars.Epoch,
		Architectures:    vars.Architectures,
		Provides:         vars.Provides,
		Conflicts:        vars.Conflicts,
		Replaces:         vars.Replaces,
		Depends:          vars.Depends.Resolved(),
		BuildDepends:     vars.BuildDepends.Resolved(),
		OptDepends:       vars.OptDepends.Resolved(),
		Sources:          vars.Sources.Resolved(),
		Checksums:        vars.Checksums.Resolved(),
		Backup:           vars.Backup.Resolved(),
		Scripts:          vars.Scripts.Resolved(),
		AutoReq:          vars.AutoReq.Resolved(),
		AutoProv:         vars.AutoProv.Resolved(),
		AutoReqSkipList:  vars.AutoReqSkipList.Resolved(),
		AutoProvSkipList: vars.AutoProvSkipList.Resolved(),
		FireJailed:       vars.FireJailed.Resolved(),
		FireJailProfiles: vars.FireJailProfiles.Resolved(),
		ExtraFiles:       vars.ExtraFiles,
		FuncsKey:         funcsKey,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		// Не должно происходить: все поля payload JSON-сериализуемы.
		// Фолбэк на ключ функций — безопасная сторона (мета не совпадёт с
		// записанной при той же ошибке и пакет пересоберётся).
		slog.Warn("Не удалось сериализовать семантический ключ кэша", "error", err)
		data = []byte(funcsKey)
	}

	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
