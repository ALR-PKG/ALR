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

package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"git.alr-pkg.ru/xpamych/vercmp"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v2"
	"golang.org/x/exp/maps"

	"git.alr-pkg.ru/Plemya-x/ALR/internal/build"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils"
	appbuilder "git.alr-pkg.ru/Plemya-x/ALR/internal/cliutils/app_builder"
	database "git.alr-pkg.ru/Plemya-x/ALR/internal/db"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/manager"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/overrides"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/search"
	"git.alr-pkg.ru/Plemya-x/ALR/internal/utils"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

func UpgradeCmd() *cli.Command {
	return &cli.Command{
		Name:    "upgrade",
		Usage:   gotext.Get("Upgrade all installed packages"),
		Aliases: []string{"up"},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "clean",
				Aliases: []string{"c"},
				Usage:   gotext.Get("Build package from scratch even if there's an already built package available"),
			},
		},
		Action: utils.RootNeededAction(func(c *cli.Context) error {
			installer, installerClose, err := build.GetSafeInstaller()
			if err != nil {
				return err
			}
			defer installerClose()

			scripter, scripterClose, err := build.GetSafeScriptExecutor()
			if err != nil {
				return err
			}
			defer scripterClose()

			ctx := c.Context

			deps, err := appbuilder.
				New(ctx).
				WithConfig().
				WithDB().
				WithRepos().
				WithDistroInfo().
				WithManager().
				Build()
			if err != nil {
				return err
			}
			defer deps.Defer()

			// Обновляем систему, если это включено в конфигурации
			if deps.Cfg.UpdateSystemOnUpgrade() {
				slog.Info(gotext.Get("Updating system packages..."))
				err = deps.Manager.UpgradeAll(&manager.Opts{
					NoConfirm: !c.Bool("interactive"),
					Args:      manager.Args,
				})
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("Error updating system packages"), err)
				}
				slog.Info(gotext.Get("System packages updated successfully"))
			}

			slog.Debug(fmt.Sprintf("[TIME: %s] Starting upgrade process", time.Now().Format("15:04:05.000")))

			builder, err := build.NewMainBuilder(
				deps.Cfg,
				deps.Manager,
				deps.Repos,
				scripter,
				installer,
			)
			if err != nil {
				return err
			}

			slog.Debug(fmt.Sprintf("[TIME: %s] Starting checkForUpdates", time.Now().Format("15:04:05.000")))
			updates, err := checkForUpdates(ctx, deps.Manager, deps.DB, deps.Info, deps.Cfg)
			slog.Debug(fmt.Sprintf("[TIME: %s] Finished checkForUpdates", time.Now().Format("15:04:05.000")), "updates_count", len(updates))
			if err != nil {
				return cliutils.FormatCliExit(gotext.Get("Error checking for updates"), err)
			}

			if len(updates) > 0 {
				slog.Debug(fmt.Sprintf("[TIME: %s] Starting InstallPkgs", time.Now().Format("15:04:05.000")), "packages", len(updates))
				_, err = builder.InstallPkgs(
					ctx,
					&build.BuildArgs{
						Opts: &types.BuildOpts{
							Clean:       c.Bool("clean"),
							Interactive: c.Bool("interactive"),
						},
						Info:       deps.Info,
						PkgFormat_: build.GetPkgFormat(deps.Manager),
					},
					mapUpdatesToPackageNames(updates),
				)
				if err != nil {
					return cliutils.FormatCliExit(gotext.Get("Error checking for updates"), err)
				}
			} else {
				slog.Info(gotext.Get("There is nothing to do."))
			}

			return nil
		}),
	}
}

func mapUpdatesToPackageNames(updates []UpdateInfo) []string {
	seen := make(map[string]bool)
	var pkgNames []string

	for _, info := range updates {
		fullName := fmt.Sprintf("%s+%s", info.Package.Name, info.Package.Repository)
		if !seen[fullName] {
			seen[fullName] = true
			pkgNames = append(pkgNames, fullName)
			slog.Debug("mapUpdatesToPackageNames: added", "fullName", fullName, "version", info.Package.Version, "release", info.Package.Release)
		}
	}

	slog.Debug("mapUpdatesToPackageNames: returning", "pkgNames", pkgNames)
	return pkgNames
}

type UpdateInfo struct {
	Package *alrsh.Package

	FromVersion string
	ToVersion   string
}

func checkForUpdates(
	ctx context.Context,
	mgr manager.Manager,
	db *database.Database,
	info *distro.OSRelease,
	cfg build.Config,
) ([]UpdateInfo, error) {
	slog.Debug("checkForUpdates: starting", "time", time.Now().Format("15:04:05.000"))

	installed, err := mgr.ListInstalled(nil)
	slog.Debug("checkForUpdates: ListInstalled done", "time", time.Now().Format("15:04:05.000"), "count", len(installed))
	if err != nil {
		return nil, err
	}

	pkgNames := maps.Keys(installed)

	s := search.New(db)

	// Предварительно получаем индексы групп захвата для производительности
	pkgIdx := build.RegexpALRPackageName.SubexpIndex("package")
	repoIdx := build.RegexpALRPackageName.SubexpIndex("repo")

	slog.Info(gotext.Get("Checking for ALR package updates..."), "count", len(pkgNames))

	// RGB градиент для прогресс-бара (красный → желтый → синий)
	gradientColor := func(pos float64) string {
		var r, g, b int
		if pos < 0.5 {
			// Красный → жёлтый
			t := pos * 2
			r = int(239 + t*(234-239))
			g = int(68 + t*(179-68))
			b = int(68 + t*(8-68))
		} else {
			// Жёлтый → синий
			t := (pos - 0.5) * 2
			r = int(234 + t*(59-234))
			g = int(179 + t*(130-179))
			b = int(8 + t*(246-8))
		}
		return fmt.Sprintf("#%02x%02x%02x", r, g, b)
	}

	var out []UpdateInfo
	checked := 0
	total := len(pkgNames)

	for _, pkgName := range pkgNames {
		checked++

		matches := build.RegexpALRPackageName.FindStringSubmatch(pkgName)
		if matches != nil {
			packageName := matches[pkgIdx]
			repoName := matches[repoIdx]

			pkgs, err := s.Search(
				ctx,
				search.NewSearchOptions().
					WithName(packageName).
					WithRepository(repoName).
					Build(),
			)
			if err != nil {
				return nil, err
			}

			if len(pkgs) == 0 {
				continue
			}

			pkg := pkgs[0]

			repoVer := pkg.Version
			releaseStr := overrides.ReleasePlatformSpecific(pkg.Release, info)

			if pkg.Release != 0 && pkg.Epoch == 0 {
				repoVer = fmt.Sprintf("%s-%s", pkg.Version, releaseStr)
			} else if pkg.Release != 0 && pkg.Epoch != 0 {
				repoVer = fmt.Sprintf("%d:%s-%s", pkg.Epoch, pkg.Version, releaseStr)
			}

			// Формируем полное имя пакета как в installed (pkg.Name + репозиторий)
			fullPkgName := fmt.Sprintf("%s+%s", pkg.Name, repoName)

			// Получаем установленную версию по полному имени из БД
			installedVer, exists := installed[fullPkgName]
			if !exists {
				// Fallback: пробуем оригинальное имя из списка установленных
				installedVer = installed[pkgName]
			}

			// Нормализуем версии перед сравнением (убираем dist суффиксы типа .red80, .el9, .fc41)
			normalizedInstalled := normalizeVersion(installedVer)
			normalizedRepo := normalizeVersion(repoVer)

			c := vercmp.Compare(normalizedRepo, normalizedInstalled)

			if c == 1 {
				slog.Debug("checkForUpdates: adding update",
					"pkgName", pkgName,
					"fullPkgName", fullPkgName,
					"exists", exists,
					"repoVer", repoVer,
					"installedVer", installedVer)
				// Создаём копию pkg, т.к. переменная цикла переиспользуется
				pkgCopy := pkg
				out = append(out, UpdateInfo{
					Package:     &pkgCopy,
					FromVersion: installedVer,
					ToVersion:   repoVer,
				})
			} else {
				// Для git-пакетов с функцией version() проверяем актуальную версию
				// на скачанных исходниках, так как статическая версия в БД может отставать.
				sources := pkg.Sources.Resolved()
				if len(sources) > 0 {
					for _, src := range sources {
						if strings.HasPrefix(src, "git+") {
							basePkg := pkg.BasePkgName
							if basePkg == "" {
								basePkg = pkg.Name
							}
							srcDir := build.GetSrcDir(cfg, basePkg)
							gitRepoPath, err := findGitRepoInSrcDir(srcDir)
							if err == nil && gitRepoPath != "" {
								gitVer, err := gitVersionFromPath(gitRepoPath)
								if err == nil && gitVer != "" {
									cgit := vercmp.Compare(gitVer, installedVer)
									if cgit == 1 {
										slog.Debug("checkForUpdates: adding git update",
											"pkgName", pkgName,
											"installedVer", installedVer,
											"gitVer", gitVer)
										pkgCopy := pkg
										out = append(out, UpdateInfo{
											Package:     &pkgCopy,
											FromVersion: installedVer,
											ToVersion:   gitVer,
										})
									}
								}
							}
							break
						}
					}
				}
			}
		}

		// Рисуем прогресс-бар из точек с RGB-градиентом
		progress := float64(checked) / float64(total)
		barWidth := 40
		filled := int(progress * float64(barWidth))

		var bar strings.Builder
		for i := 0; i < barWidth; i++ {
			pos := float64(i) / float64(barWidth-1)
			color := lipgloss.Color(gradientColor(pos))
			if i < filled {
				bar.WriteString(lipgloss.NewStyle().Foreground(color).Render("●"))
			} else {
				bar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("○"))
			}
		}

		fmt.Fprintf(os.Stderr, "\r%s %3.0f%% (%d/%d)", bar.String(), math.Floor(progress*100), checked, total)
	}

	fmt.Fprintln(os.Stderr) // новая строка после завершения

	slog.Debug("checkForUpdates: finished", "time", time.Now().Format("15:04:05.000"), "updates_available", len(out))
	slog.Info(gotext.Get("Finished checking for updates"), "updates_available", len(out))

	return out, nil
}

// distSuffixRegex находит dist суффиксы в версии (.red80, .el9, .fc41, .alt1 и т.д.)
// Формат: [epoch:]version-release.dist
// Примеры: "1:26.0.4-10.red80", "2024.4-4.red80", "15.1.0-3.fc41"
var distSuffixRegex = regexp.MustCompile(`^(\d+:)?([\d\.]+-\d+)(\.[a-z]+\d*)$`)

// normalizeVersion убирает dist суффикс из версии пакета
// Например: "1:26.0.4-10.red80" → "1:26.0.4-10"
//           "2024.4-4.red80" → "2024.4-4"
//           "15.1.0-3.fc41" → "15.1.0-3"
func normalizeVersion(version string) string {
	// Пробуем найти и удалить dist суффикс
	matches := distSuffixRegex.FindStringSubmatch(version)
	if len(matches) >= 3 {
		// matches[1] = epoch (может быть пустым)
		// matches[2] = version-release
		return matches[1] + matches[2]
	}
	return version
}

// findGitRepoInSrcDir ищет git-репозиторий внутри директории исходников.
// Возвращает путь к найденному репозиторию или пустую строку.
func findGitRepoInSrcDir(srcDir string) (string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			gitDir := filepath.Join(srcDir, entry.Name(), ".git")
			if _, err := os.Stat(gitDir); err == nil {
				return filepath.Join(srcDir, entry.Name()), nil
			}
		}
	}
	return "", nil
}

// gitVersionFromPath получает версию в формате git-version (revNum.hash) из локального git-репозитория.
func gitVersionFromPath(repoPath string) (string, error) {
	r, err := git.PlainOpen(repoPath)
	if err != nil {
		return "", err
	}
	revNum := 0
	commits, err := r.Log(&git.LogOptions{})
	if err != nil {
		return "", err
	}
	err = commits.ForEach(func(*object.Commit) error { revNum++; return nil })
	if err != nil {
		return "", err
	}
	HEAD, err := r.Head()
	if err != nil {
		return "", err
	}
	hash := HEAD.Hash().String()
	return fmt.Sprintf("%d.%s", revNum, hash[:7]), nil
}
