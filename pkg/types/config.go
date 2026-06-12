// This file was originally part of the project "LURE - Linux User REpository", created by Elara Musayelyan.
// It has been modified as part of "ALR - Any Linux Repository" by the ALR Authors.
//
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

package types

// Config represents the ALR configuration file
type Config struct {
	RootCmd               string   `json:"rootCmd" koanf:"rootCmd"`
	UseRootCmd            bool     `json:"useRootCmd" koanf:"useRootCmd"`
	PagerStyle            string   `json:"pagerStyle" koanf:"pagerStyle"`
	IgnorePkgUpdates      []string `json:"ignorePkgUpdates" koanf:"ignorePkgUpdates"`
	Repos                 []Repo   `json:"repo" koanf:"repo"`
	AutoPull              bool     `json:"autoPull" koanf:"autoPull"`
	LogLevel              string   `json:"logLevel" koanf:"logLevel"`
	UpdateSystemOnUpgrade bool     `json:"updateSystemOnUpgrade" koanf:"updateSystemOnUpgrade"`
	PreferALRDeps         bool     `json:"preferALRDeps" koanf:"preferALRDeps"`
	Site                  Site     `json:"site" koanf:"site"`
}

// Site represents the ALR site RPC configuration
type Site struct {
	URL        string   `json:"url" koanf:"url"`
	Mirrors    []string `json:"mirrors" koanf:"mirrors"`
	Timeout    int      `json:"timeout" koanf:"timeout"`
	DisableRPC bool     `json:"disableRPC" koanf:"disableRPC"`
	PreferRPC  bool     `json:"preferRPC" koanf:"preferRPC"`
}

// Repo represents a ALR repo within a configuration file
type Repo struct {
	Name    string   `json:"name" koanf:"name"`
	URL     string   `json:"url" koanf:"url"`
	Ref     string   `json:"ref" koanf:"ref"`
	Mirrors []string `json:"mirrors" koanf:"mirrors"`
}
