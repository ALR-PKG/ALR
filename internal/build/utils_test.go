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
	"slices"
	"strings"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/cpu"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
)

type mockInput struct {
	repo      string
	osInfo    *distro.OSRelease
	pkgFormat string
}

func (m *mockInput) Repository() string {
	return m.repo
}

func (m *mockInput) OSRelease() *distro.OSRelease {
	return m.osInfo
}

func (m *mockInput) PkgFormat() string {
	return m.pkgFormat
}

func TestGetBasePkgInfo(t *testing.T) {
	tests := []struct {
		name         string
		packageName  string
		repoName     string
		expectedName string
	}{
		{
			name:         "обычный репозиторий",
			packageName:  "test-package",
			repoName:     "default",
			expectedName: "test-package+default",
		},
		{
			name:         "репозиторий с alr- префиксом",
			packageName:  "test-package",
			repoName:     "alr-default",
			expectedName: "test-package+alr-default",
		},
		{
			name:         "репозиторий с двойным alr- префиксом",
			packageName:  "test-package",
			repoName:     "alr-alr-repo",
			expectedName: "test-package+alr-alr-repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := &alrsh.Package{
				Name:    tt.packageName,
				Version: "1.0.0",
				Release: 1,
			}

			input := &mockInput{
				repo: tt.repoName,
				osInfo: &distro.OSRelease{
					ID: "test",
				},
			}

			info := getBasePkgInfo(pkg, input)

			if info.Name != tt.expectedName {
				t.Errorf("getBasePkgInfo() имя пакета = %v, ожидается %v", info.Name, tt.expectedName)
			}
		})
	}
}

func TestGetBasePkgInfoArch(t *testing.T) {
	tests := []struct {
		name          string
		architectures []string
		expectedArch  string
	}{
		{
			name:          "architectures=all даёт arch all",
			architectures: []string{"all"},
			expectedArch:  "all",
		},
		{
			name:          "all среди прочих архитектур даёт arch all",
			architectures: []string{"amd64", "all"},
			expectedArch:  "all",
		},
		{
			name:          "обычная архитектура берётся из cpu.Arch()",
			architectures: []string{"amd64"},
			expectedArch:  cpu.Arch(),
		},
		{
			name:          "пустые architectures берутся из cpu.Arch()",
			architectures: nil,
			expectedArch:  cpu.Arch(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := &alrsh.Package{
				Name:          "test-package",
				Version:       "1.0.0",
				Release:       1,
				Architectures: tt.architectures,
			}

			input := &mockInput{
				repo:   "default",
				osInfo: &distro.OSRelease{ID: "test"},
			}

			info := getBasePkgInfo(pkg, input)

			if info.Arch != tt.expectedArch {
				t.Errorf("getBasePkgInfo() arch = %v, ожидается %v", info.Arch, tt.expectedArch)
			}
		})
	}
}

// TestPkgFileNameArchAll проверяет, что имя файла для проверки кэша
// совпадает с именем, под которым пакет публикуется: при
// architectures=('all') nfpm для rpm подставляет noarch, для deb — all.
func TestPkgFileNameArchAll(t *testing.T) {
	tests := []struct {
		name          string
		pkgFormat     string
		architectures []string
		wantContains  string
	}{
		{
			name:          "rpm с architectures=all",
			pkgFormat:     "rpm",
			architectures: []string{"all"},
			wantContains:  ".noarch.rpm",
		},
		{
			name:          "deb с architectures=all",
			pkgFormat:     "deb",
			architectures: []string{"all"},
			wantContains:  "_all.deb",
		},
		{
			name:          "rpm с обычной архитектурой не noarch",
			pkgFormat:     "rpm",
			architectures: []string{cpu.Arch()},
			wantContains:  "+default-1.0.0-1.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkg := &alrsh.Package{
				Name:          "test-package",
				Version:       "1.0.0",
				Release:       1,
				Architectures: tt.architectures,
			}

			input := &mockInput{
				repo:      "default",
				osInfo:    &distro.OSRelease{ID: "test"},
				pkgFormat: tt.pkgFormat,
			}

			filename, err := pkgFileName(input, pkg)
			if err != nil {
				t.Fatalf("pkgFileName() ошибка: %v", err)
			}

			if !strings.Contains(filename, tt.wantContains) {
				t.Errorf("pkgFileName() = %q, ожидается подстрока %q", filename, tt.wantContains)
			}

			isAll := slices.Contains(tt.architectures, "all")
			if isAll && strings.Contains(filename, cpu.Arch()) {
				t.Errorf("pkgFileName() = %q содержит host-архитектуру %q при architectures=all", filename, cpu.Arch())
			}
		})
	}
}

func TestRegexpALRPackageName(t *testing.T) {
	tests := []struct {
		name         string
		packageName  string
		expectedPkg  string
		expectedRepo string
		shouldMatch  bool
	}{
		{
			name:         "новый формат - обычный репозиторий",
			packageName:  "test-package+default",
			expectedPkg:  "test-package",
			expectedRepo: "default",
			shouldMatch:  true,
		},
		{
			name:         "новый формат - alr-default репозиторий",
			packageName:  "test-package+alr-default",
			expectedPkg:  "test-package",
			expectedRepo: "alr-default",
			shouldMatch:  true,
		},
		{
			name:         "новый формат - двойной alr- префикс",
			packageName:  "test-package+alr-alr-repo",
			expectedPkg:  "test-package",
			expectedRepo: "alr-alr-repo",
			shouldMatch:  true,
		},
		{
			name:        "некорректный формат - без плюса",
			packageName: "test-package",
			shouldMatch: false,
		},
		{
			name:        "некорректный формат - пустое имя пакета",
			packageName: "+repo",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := RegexpALRPackageName.FindStringSubmatch(tt.packageName)

			if tt.shouldMatch {
				if matches == nil {
					t.Errorf("RegexpALRPackageName должен найти совпадение для %q", tt.packageName)
					return
				}

				packageName := matches[RegexpALRPackageName.SubexpIndex("package")]
				repoName := matches[RegexpALRPackageName.SubexpIndex("repo")]

				if packageName != tt.expectedPkg {
					t.Errorf("RegexpALRPackageName извлеченное имя пакета = %v, ожидается %v", packageName, tt.expectedPkg)
				}

				if repoName != tt.expectedRepo {
					t.Errorf("RegexpALRPackageName извлеченное имя репозитория = %v, ожидается %v", repoName, tt.expectedRepo)
				}
			} else {
				if matches != nil {
					t.Errorf("RegexpALRPackageName не должен найти совпадение для %q", tt.packageName)
				}
			}
		})
	}
}

func TestGoArchToRPMISA(t *testing.T) {
	tests := []struct {
		goarch   string
		expected string
	}{
		{"amd64", "x86-64"},
		{"386", "x86-32"},
		{"arm64", "aarch-64"},
		{"arm", ""},
		{"mips", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.goarch, func(t *testing.T) {
			result := goArchToRPMISA(tt.goarch)
			if result != tt.expected {
				t.Errorf("goArchToRPMISA(%q) = %q, ожидается %q", tt.goarch, result, tt.expected)
			}
		})
	}
}

func TestEscapeGlobChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "квадратные скобки",
			input:    "/path/to/file[1].txt",
			expected: `/path/to/file\[1\].txt`,
		},
		{
			name:     "фигурные скобки",
			input:    "/path/to/{GUID}/file.txt",
			expected: `/path/to/\{GUID\}/file.txt`,
		},
		{
			name:     "оба типа скобок",
			input:    "/path[1]/{GUID}/file.txt",
			expected: `/path\[1\]/\{GUID\}/file.txt`,
		},
		{
			name:     "обычный путь",
			input:    "/usr/share/doc/README.md",
			expected: "/usr/share/doc/README.md",
		},
		{
			name:     "UUID в фигурных скобках",
			input:    "{07FD8DFA-DFE0-4089-AL24-0730933CC80A}/3rd-Party.txt",
			expected: `\{07FD8DFA-DFE0-4089-AL24-0730933CC80A\}/3rd-Party.txt`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeGlobChars(tt.input)
			if result != tt.expected {
				t.Errorf("escapeGlobChars(%q) = %q, ожидается %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractRepoNameFromPath(t *testing.T) {
	tests := []struct {
		name         string
		scriptPath   string
		expectedRepo string
	}{
		{
			name:         "относительный путь - стандартная структура",
			scriptPath:   "alr-default/alr-bin/alr.sh",
			expectedRepo: "alr-default",
		},
		{
			name:         "абсолютный путь",
			scriptPath:   "/home/user/repos/alr-default/alr-bin/alr.sh",
			expectedRepo: "alr-default",
		},
		{
			name:         "репозиторий без префикса alr-",
			scriptPath:   "my-repo/my-package/alr.sh",
			expectedRepo: "my-repo",
		},
		{
			name:         "только имя файла",
			scriptPath:   "alr.sh",
			expectedRepo: "default",
		},
		{
			name:         "один уровень директории",
			scriptPath:   "package/alr.sh",
			expectedRepo: "default",
		},
		{
			name:         "путь с точками",
			scriptPath:   "./alr-default/alr-bin/alr.sh",
			expectedRepo: "alr-default",
		},
		{
			name:         "путь с двойными точками",
			scriptPath:   "../alr-default/alr-bin/alr.sh",
			expectedRepo: "alr-default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractRepoNameFromPath(tt.scriptPath)
			if result != tt.expectedRepo {
				t.Errorf("ExtractRepoNameFromPath(%q) = %q, ожидается %q", tt.scriptPath, result, tt.expectedRepo)
			}
		})
	}
}
