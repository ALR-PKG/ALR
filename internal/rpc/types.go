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

package rpc

import "git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"

// Package представляет пакет, возвращаемый RPC API
type Package struct {
	Name          string      `json:"name"`
	BasePkgName   string      `json:"basepkg_name"`
	Version       string      `json:"version"`
	Release       int         `json:"release"`
	Epoch         int         `json:"epoch"`
	Repo          string      `json:"repo"`
	Desc          string      `json:"desc"`
	DescRu        string      `json:"desc_ru"`
	Architectures []string    `json:"architectures"`
	Maintainer    string      `json:"maintainer"`
	License       []string    `json:"license"`
	Provides      []string    `json:"provides"`
	Conflicts     []string    `json:"conflicts"`
	Depends       []string    `json:"depends"`
	BuildDepends  []string    `json:"build_depends"`
	OptDepends    []string    `json:"opt_depends"`
	Path          string      `json:"path"`
	AlrShURL      string      `json:"alr_sh_url"`
	AlrShSHA256   string      `json:"alr_sh_sha256"`
	LastModified  int64       `json:"last_modified"`
	ExtraFiles    []ExtraFile `json:"extra_files"`
}

// ExtraFile — дополнительный файл из директории пакета
type ExtraFile struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
}

// ReposResponse — ответ от /rpc/v1/repos
type ReposResponse struct {
	Version      int              `json:"version"`
	Repositories []RepositoryInfo `json:"repositories"`
}

// RepositoryInfo — информация о репозитории в RPC
type RepositoryInfo struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// InfoResponse — ответ от /rpc/v1/info
type InfoResponse struct {
	Version     int       `json:"version"`
	ResultCount int       `json:"resultcount"`
	Results     []Package `json:"results"`
}

// SearchResponse — ответ от /rpc/v1/search
type SearchResponse struct {
	Version     int       `json:"version"`
	ResultCount int       `json:"resultcount"`
	Results     []Package `json:"results"`
}

// ListResponse — ответ от /rpc/v1/list
type ListResponse struct {
	Version     int       `json:"version"`
	ResultCount int       `json:"resultcount"`
	Total       int       `json:"total"`
	Page        int       `json:"page"`
	Limit       int       `json:"limit"`
	Results     []Package `json:"results"`
}

// CountResponse — ответ от /rpc/v1/count
type CountResponse struct {
	Version int `json:"version"`
	Count   int `json:"count"`
}

// InstalledPackage — установленный пакет в запросе обновлений
type InstalledPackage struct {
	Name    string `json:"name"`
	Repo    string `json:"repo"`
	Version string `json:"version"`
}

// DistroInfo — информация о дистрибутиве в запросе обновлений
type DistroInfo struct {
	ID        string `json:"id"`
	VersionID string `json:"version_id,omitempty"`
}

// UpdatesRequest — тело запроса для /rpc/v1/updates
type UpdatesRequest struct {
	Packages []InstalledPackage `json:"packages"`
	Distro   *DistroInfo        `json:"distro,omitempty"`
}

// UpdateInfo — информация о доступном обновлении
type UpdateInfo struct {
	Name        string `json:"name"`
	Repo        string `json:"repo"`
	FromVersion string `json:"from_version"`
	ToVersion   string `json:"to_version"`
}

// UpdatesResponse — ответ от /rpc/v1/updates
type UpdatesResponse struct {
	Version int          `json:"version"`
	Updates []UpdateInfo `json:"updates"`
}

// ToALRShPackage конвертирует RPC Package в alrsh.Package
func (p *Package) ToALRShPackage() *alrsh.Package {
	pkg := &alrsh.Package{
		Name:          p.Name,
		BasePkgName:   p.BasePkgName,
		Version:       p.Version,
		Release:       p.Release,
		Epoch:         uint(p.Epoch),
		Repository:    p.Repo,
		Architectures: p.Architectures,
		Licenses:      p.License,
		Provides:      p.Provides,
		Conflicts:     p.Conflicts,
	}

	pkg.Description.Set("", p.Desc)
	pkg.Description.SetResolved(p.Desc)
	pkg.Maintainer.Set("", p.Maintainer)
	pkg.Maintainer.SetResolved(p.Maintainer)

	if len(p.Depends) > 0 {
		deps := make([]string, len(p.Depends))
		copy(deps, p.Depends)
		pkg.Depends.Set("", deps)
		pkg.Depends.SetResolved(deps)
	}

	if len(p.BuildDepends) > 0 {
		buildDeps := make([]string, len(p.BuildDepends))
		copy(buildDeps, p.BuildDepends)
		pkg.BuildDepends.Set("", buildDeps)
		pkg.BuildDepends.SetResolved(buildDeps)
	}

	if len(p.OptDepends) > 0 {
		optDeps := make([]string, len(p.OptDepends))
		copy(optDeps, p.OptDepends)
		pkg.OptDepends.Set("", optDeps)
		pkg.OptDepends.SetResolved(optDeps)
	}

	pkg.AlrShURL = p.AlrShURL
	pkg.AlrShSHA256 = p.AlrShSHA256

	// Копируем дополнительные файлы
	if len(p.ExtraFiles) > 0 {
		pkg.ExtraFiles = make([]alrsh.ExtraFile, len(p.ExtraFiles))
		for i, ef := range p.ExtraFiles {
			pkg.ExtraFiles[i] = alrsh.ExtraFile{
				Name:        ef.Name,
				Path:        ef.Path,
				DownloadURL: ef.DownloadURL,
				SHA256:      ef.SHA256,
			}
		}
	}

	return pkg
}
