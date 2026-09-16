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

package alrsh

import "encoding/json"

type PackageView struct {
	pkg Package

	Resolved bool

	// SiteURL — ссылка на страницу пакета на сайте ALR
	SiteURL string
}

func NewPackageView(v Package) PackageView {
	return PackageView{pkg: v}
}

func (p PackageView) MarshalJSON() ([]byte, error) {
	var raw []byte
	var err error
	if p.Resolved {
		raw, err = json.Marshal(PackageToResolved(&p.pkg))
	} else {
		raw, err = json.Marshal(p.pkg)
	}
	if err != nil || p.SiteURL == "" {
		return raw, err
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, nil
	}
	m["site_url"] = p.SiteURL
	return json.Marshal(m)
}
