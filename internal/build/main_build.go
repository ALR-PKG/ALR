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

package build

import (
	"git.alr-pkg.ru/ALR/ALR/internal/manager"
)

func NewMainBuilder(
	cfg Config,
	mgr manager.Manager,
	repos PackageFinder,
	scriptExecutor ScriptExecutor,
	installerExecutor InstallerExecutor,
) (*Builder, error) {
	builder := &Builder{
		scriptExecutor: scriptExecutor,
		cacheExecutor: &Cache{
			cfg,
		},
		scriptResolver: &ScriptResolver{
			cfg,
		},
		scriptViewerExecutor: &ScriptViewer{
			config: cfg,
		},
		checkerExecutor: &Checker{
			mgr,
		},
		installerExecutor: installerExecutor,
		sourceExecutor: &SourceDownloader{
			cfg,
		},
		repos: repos,
		mgr:   mgr,
		cfg:   cfg,
	}

	return builder, nil
}
