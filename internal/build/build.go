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

package build

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/leonelquinteros/gotext"

	"git.alr-pkg.ru/ALR/ALR/internal/cliutils"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
	"git.alr-pkg.ru/ALR/ALR/internal/manager"
	"git.alr-pkg.ru/ALR/ALR/internal/stats"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/distro"
	"git.alr-pkg.ru/ALR/ALR/pkg/types"
)

type BuildInput struct {
	opts             *types.BuildOpts
	info             *distro.OSRelease
	pkgFormat        string
	script           string
	repository       string
	packages         []string
	skipDepsBuilding bool // Пропустить сборку зависимостей (используется при вызове из BuildALRDeps)
	skipBuildDeps    bool // Пропустить установку build_deps (используется при единой установке)
}

func (bi *BuildInput) GobEncode() ([]byte, error) {
	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)

	if err := encoder.Encode(bi.opts); err != nil {
		return nil, err
	}
	if err := encoder.Encode(bi.info); err != nil {
		return nil, err
	}
	if err := encoder.Encode(bi.pkgFormat); err != nil {
		return nil, err
	}
	if err := encoder.Encode(bi.script); err != nil {
		return nil, err
	}
	if err := encoder.Encode(bi.repository); err != nil {
		return nil, err
	}
	if err := encoder.Encode(bi.packages); err != nil {
		return nil, err
	}

	return w.Bytes(), nil
}

func (bi *BuildInput) GobDecode(data []byte) error {
	r := bytes.NewBuffer(data)
	decoder := gob.NewDecoder(r)

	if err := decoder.Decode(&bi.opts); err != nil {
		return err
	}
	if err := decoder.Decode(&bi.info); err != nil {
		return err
	}
	if err := decoder.Decode(&bi.pkgFormat); err != nil {
		return err
	}
	if err := decoder.Decode(&bi.script); err != nil {
		return err
	}
	if err := decoder.Decode(&bi.repository); err != nil {
		return err
	}
	if err := decoder.Decode(&bi.packages); err != nil {
		return err
	}

	return nil
}

func (b *BuildInput) Repository() string {
	return b.repository
}

func (b *BuildInput) BuildOpts() *types.BuildOpts {
	return b.opts
}

func (b *BuildInput) OSRelease() *distro.OSRelease {
	return b.info
}

func (b *BuildInput) PkgFormat() string {
	return b.pkgFormat
}

type BuildOptsProvider interface {
	BuildOpts() *types.BuildOpts
}

type OsInfoProvider interface {
	OSRelease() *distro.OSRelease
}

type PkgFormatProvider interface {
	PkgFormat() string
}

type RepositoryProvider interface {
	Repository() string
}

// ================================================

type BuiltDep struct {
	Name string
	Path string
}

func Map[T, R any](items []T, f func(T) R) []R {
	res := make([]R, len(items))
	for i, item := range items {
		res[i] = f(item)
	}
	return res
}

func GetBuiltPaths(deps []*BuiltDep) []string {
	return Map(deps, func(dep *BuiltDep) string {
		return dep.Path
	})
}

func GetBuiltName(deps []*BuiltDep) []string {
	return Map(deps, func(dep *BuiltDep) string {
		return dep.Name
	})
}

type PackageFinder interface {
	FindPkgs(ctx context.Context, pkgs []string) (map[string][]alrsh.Package, []string, error)
}

type Config interface {
	GetPaths() *config.Paths
	PagerStyle() string
	PreferALRDeps() bool
	Site() types.Site
}

type FunctionsOutput struct {
	Contents *[]string
}

// EXECUTORS

type ScriptResolverExecutor interface {
	ResolveScript(ctx context.Context, pkg *alrsh.Package) *ScriptInfo
}

type CacheExecutor interface {
	CheckForBuiltPackage(ctx context.Context, input *BuildInput, vars *alrsh.Package, funcsKey string) (string, bool, error)
}

type ScriptViewerExecutor interface {
	ViewScript(ctx context.Context, input *BuildInput, sf *alrsh.ScriptFile, basePkg string) error
}

type CheckerExecutor interface {
	PerformChecks(
		ctx context.Context,
		input *BuildInput,
		vars *alrsh.Package,
	) (bool, error)
}

type SourcesInput struct {
	Sources   []string
	Checksums []string
}

type SourceDownloaderExecutor interface {
	DownloadSources(
		ctx context.Context,
		input *BuildInput,
		basePkg string,
		si SourcesInput,
	) error
}

//

func NewBuilder(
	scriptResolver ScriptResolverExecutor,
	scriptExecutor ScriptExecutor,
	cacheExecutor CacheExecutor,
	scriptViewerExecutor ScriptViewerExecutor,
	checkerExecutor CheckerExecutor,
	installerExecutor InstallerExecutor,
	sourceExecutor SourceDownloaderExecutor,
) *Builder {
	return &Builder{
		scriptResolver:       scriptResolver,
		scriptExecutor:       scriptExecutor,
		cacheExecutor:        cacheExecutor,
		scriptViewerExecutor: scriptViewerExecutor,
		checkerExecutor:      checkerExecutor,
		installerExecutor:    installerExecutor,
		sourceExecutor:       sourceExecutor,
	}
}

type Builder struct {
	scriptResolver       ScriptResolverExecutor
	scriptExecutor       ScriptExecutor
	cacheExecutor        CacheExecutor
	scriptViewerExecutor ScriptViewerExecutor
	checkerExecutor      CheckerExecutor
	installerExecutor    InstallerExecutor
	sourceExecutor       SourceDownloaderExecutor
	repos                PackageFinder
	mgr                  manager.Manager
	cfg                  Config
	trustedUIDs          []uint32
}

// SetTrustedUIDs задаёт дополнительные доверенные uid владельцев кэша
// (uid вызвавшего при сборке от демона) и пробрасывает их в Cache.
func (b *Builder) SetTrustedUIDs(uids ...uint32) {
	b.trustedUIDs = uids
	if c, ok := b.cacheExecutor.(*Cache); ok {
		c.trustedUIDs = uids
	}
}

type BuildArgs struct {
	Opts             *types.BuildOpts
	Info             *distro.OSRelease
	PkgFormat_       string
	SkipDepsBuilding bool // Пропустить сборку зависимостей (используется при вызове из BuildALRDeps)
	SkipViewScript   bool // Пропустить просмотр скрипта (используется когда скрипты уже показаны)
	SkipBuildDeps    bool // Пропустить установку build_deps (используется при единой установке)
}

func (b *BuildArgs) BuildOpts() *types.BuildOpts {
	return b.Opts
}

func (b *BuildArgs) OSRelease() *distro.OSRelease {
	return b.Info
}

func (b *BuildArgs) PkgFormat() string {
	return b.PkgFormat_
}

type BuildPackageFromDbArgs struct {
	BuildArgs
	Package  *alrsh.Package
	Packages []string
}

type BuildPackageFromScriptArgs struct {
	BuildArgs
	Script   string
	Packages []string
}

func (b *Builder) BuildPackageFromDb(
	ctx context.Context,
	args *BuildPackageFromDbArgs,
) ([]*BuiltDep, error) {
	scriptInfo := b.scriptResolver.ResolveScript(ctx, args.Package)

	return b.BuildPackage(ctx, &BuildInput{
		script:           scriptInfo.Script,
		repository:       scriptInfo.Repository,
		packages:         args.Packages,
		pkgFormat:        args.PkgFormat(),
		opts:             args.Opts,
		info:             args.Info,
		skipDepsBuilding: args.SkipDepsBuilding,
		skipBuildDeps:    args.SkipBuildDeps,
	})
}

func (b *Builder) BuildPackageFromScript(
	ctx context.Context,
	args *BuildPackageFromScriptArgs,
) ([]*BuiltDep, error) {
	return b.BuildPackage(ctx, &BuildInput{
		script:     args.Script,
		repository: ExtractRepoNameFromPath(args.Script),
		packages:   args.Packages,
		pkgFormat:  args.PkgFormat(),
		opts:       args.Opts,
		info:       args.Info,
	})
}

func (b *Builder) BuildPackage(
	ctx context.Context,
	input *BuildInput,
) ([]*BuiltDep, error) {
	scriptPath := input.script

	slog.Debug("ReadScript")
	sf, err := b.scriptExecutor.ReadScript(ctx, scriptPath)
	if err != nil {
		return nil, fmt.Errorf("failed reading script: %w", err)
	}

	slog.Debug("ExecuteFirstPass")
	basePkg, varsOfPackages, err := b.scriptExecutor.ExecuteFirstPass(ctx, input, sf)
	if err != nil {
		return nil, fmt.Errorf("failed ExecuteFirstPass: %w", err)
	}

	lockPath := getBaseDir(b.cfg, basePkg) + ".lock"
	lock, err := fsutils.TryLock(lockPath)
	if errors.Is(err, fsutils.ErrLocked) {
		return nil, fmt.Errorf("%s: %w", gotext.Get("Package is already being built by another process"), fsutils.ErrLocked)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock %s: %w", lockPath, err)
	}
	defer lock.Close()

	var builtDeps []*BuiltDep
	var remainingVars []*alrsh.Package

	if !input.opts.Clean {
		funcsKey := ScriptFuncsKey(sf)
		for _, vars := range varsOfPackages {
			builtPkgPath, ok, err := b.cacheExecutor.CheckForBuiltPackage(ctx, input, vars, funcsKey)
			if err != nil {
				return nil, err
			}
			if ok {
				builtDeps = append(builtDeps, &BuiltDep{
					Path: builtPkgPath,
					Name: vars.Name,
				})
			} else {
				remainingVars = append(remainingVars, vars)
			}
		}

		if len(remainingVars) == 0 {
			slog.Info(gotext.Get("Using cached package"), "name", basePkg)
			return builtDeps, nil
		}

		// Обновляем varsOfPackages только теми пакетами, которые нужно собрать
		varsOfPackages = remainingVars

		// Очищаем build-директории для пакетов, которые будем пересобирать
		// (например, при изменении release в рамках той же версии)
		for _, vars := range varsOfPackages {
			pkgDirName := "pkg"
			if vars.Name != "" && vars.Name != basePkg {
				pkgDirName = "pkg_" + vars.Name
			}
			pkgDir := filepath.Join(getBaseDir(b.cfg, basePkg), pkgDirName)
			if err := os.RemoveAll(pkgDir); err != nil {
				slog.Warn(gotext.Get("Failed to clean build directory"), "dir", pkgDir, "error", err)
			} else {
				slog.Debug(gotext.Get("Cleaned build directory for rebuild"), "dir", pkgDir)
			}
		}
	}

	// Примечание: вывод "Building package" перенесен в InstallPkgs для единообразия

	for _, vars := range varsOfPackages {
		cont, err := b.checkerExecutor.PerformChecks(ctx, input, vars)
		if err != nil {
			return nil, err
		}
		if !cont {
			return nil, errors.New("exit...")
		}
	}

	buildDepends := []string{}
	optDepends := []string{}
	depends := []string{}
	sources := []string{}
	checksums := []string{}
	for _, vars := range varsOfPackages {
		buildDepends = append(buildDepends, vars.BuildDepends.Resolved()...)
		optDepends = append(optDepends, vars.OptDepends.Resolved()...)
		depends = append(depends, vars.Depends.Resolved()...)
		sources = append(sources, vars.Sources.Resolved()...)
		checksums = append(checksums, vars.Checksums.Resolved()...)
	}
	buildDepends = removeDuplicates(buildDepends)
	optDepends = removeDuplicates(optDepends)
	depends = removeDuplicates(depends)

	if len(sources) != len(checksums) {
		slog.Error(gotext.Get("The checksums array must be the same length as sources"))
		return nil, errors.New("exit...")
	}
	sources, checksums = removeDuplicatesSources(sources, checksums)

	var alrBuildDeps []*BuiltDep

	// Устанавливаем build_deps только если не в режиме единой установки
	if !input.skipBuildDeps {
		slog.Debug("installBuildDeps")
		alrBuildDeps, _, err = b.installBuildDeps(ctx, input, buildDepends)
		if err != nil {
			return nil, err
		}

		slog.Debug("installOptDeps")
		_, err = b.installOptDeps(ctx, input, optDepends)
		if err != nil {
			return nil, err
		}
	}

	depNames := make(map[string]struct{})
	for _, dep := range alrBuildDeps {
		depNames[dep.Name] = struct{}{}
	}

	// We filter so as not to re-build what has already been built at the `installBuildDeps` stage.
	var filteredDepends []string

	// Создаем набор подпакетов текущего мультипакета для исключения циклических зависимостей
	// Используем имена из varsOfPackages, так как input.packages может быть пустым
	currentPackageNames := make(map[string]struct{})
	for _, vars := range varsOfPackages {
		currentPackageNames[vars.Name] = struct{}{}
	}

	for _, d := range depends {
		if _, found := depNames[d]; !found {
			// Исключаем зависимости, которые являются подпакетами текущего мультипакета
			if _, isCurrentPackage := currentPackageNames[d]; !isCurrentPackage {
				filteredDepends = append(filteredDepends, d)
			}
		}
	}

	var newBuiltDeps []*BuiltDep
	var repoDeps []string

	// Пропускаем сборку зависимостей если флаг установлен (вызов из BuildALRDeps)
	if !input.skipDepsBuilding {
		slog.Debug("BuildALRDeps")
		newBuiltDeps, repoDeps, err = b.BuildALRDeps(ctx, input, filteredDepends)
		if err != nil {
			return nil, err
		}
	}

	slog.Debug("PrepareDirs")
	err = b.scriptExecutor.PrepareDirs(ctx, input, basePkg)
	if err != nil {
		return nil, err
	}

	slog.Info(gotext.Get("Downloading sources"))
	slog.Debug("DownloadSources")
	err = b.sourceExecutor.DownloadSources(
		ctx,
		input,
		basePkg,
		SourcesInput{
			Sources:   sources,
			Checksums: checksums,
		},
	)
	if err != nil {
		return nil, err
	}

	builtDeps = removeDuplicates(append(builtDeps, newBuiltDeps...))

	// Для git-пакетов с функцией version() обновляем версию после скачивания исходников,
	// когда srcdir уже содержит git-репозиторий.
	dirs, err := getDirs(b.cfg, input.script, basePkg)
	if err != nil {
		return nil, err
	}
	for i, vars := range varsOfPackages {
		v, err := sf.ResolveVersion(ctx, input.info, dirs.SrcDir)
		if err != nil {
			slog.Warn("Failed to resolve git version", "package", vars.Name, "error", err)
			continue
		}
		if v != "" {
			varsOfPackages[i].Version = v
		}
	}

	slog.Debug("ExecuteSecondPass")
	res, err := b.scriptExecutor.ExecuteSecondPass(
		ctx,
		input,
		sf,
		varsOfPackages,
		repoDeps,
		builtDeps,
		basePkg,
	)
	if err != nil {
		return nil, err
	}

	builtDeps = removeDuplicates(append(builtDeps, res...))

	// Примечание: удаление build_deps теперь происходит один раз в конце InstallPkgs
	// чтобы избежать дублирования промптов

	return builtDeps, nil
}

type InstallPkgsArgs struct {
	BuildArgs
	AlrPkgs    []alrsh.Package
	NativePkgs []string
}

func (b *Builder) InstallALRPackages(
	ctx context.Context,
	input interface {
		OsInfoProvider
		BuildOptsProvider
		PkgFormatProvider
	},
	alrPkgs []alrsh.Package,
) error {
	if len(alrPkgs) == 0 {
		return nil
	}

	// Получаем имена всех пакетов для установки
	pkgNames := make([]string, len(alrPkgs))
	for i, pkg := range alrPkgs {
		pkgNames[i] = pkg.Name
	}

	// Используем тот же алгоритм что и InstallPkgs
	_, err := b.InstallPkgs(ctx, input, pkgNames)
	return err
}

func (b *Builder) BuildALRDeps(
	ctx context.Context,
	input interface {
		OsInfoProvider
		BuildOptsProvider
		PkgFormatProvider
	},
	depends []string,
) (buildDeps []*BuiltDep, repoDeps []string, err error) {
	if len(depends) == 0 {
		return nil, nil, nil
	}

	// Используем единое дерево зависимостей
	tree, err := b.ResolveUnifiedDependencyTree(ctx, input, depends)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to resolve dependency tree: %w", err)
	}

	repoDeps = tree.AllSystemDeps

	// Собираем все ALR пакеты из дерева
	var allBuiltDeps []*BuiltDep

	for _, pkgName := range tree.AllALRPackages {
		node, ok := tree.Nodes[pkgName]
		if !ok || node == nil || node.Package == nil {
			continue
		}

		// Проверяем нужна ли сборка
		needBuildPkgs, err := b.installerExecutor.FilterPackagesByVersion(ctx, []alrsh.Package{*node.Package}, input.OSRelease())
		if err != nil {
			return nil, nil, fmt.Errorf("failed to filter package %s: %w", pkgName, err)
		}

		if len(needBuildPkgs) == 0 {
			continue
		}

		// Проверяем кеш
		scriptInfo := b.scriptResolver.ResolveScript(ctx, node.Package)
		buildInput := &BuildInput{
			script:     scriptInfo.Script,
			repository: scriptInfo.Repository,
			packages:   []string{pkgName},
			pkgFormat:  input.PkgFormat(),
			opts:       input.BuildOpts(),
			info:       input.OSRelease(),
		}

		cachedDeps, allInCache, err := b.checkCacheForAllSubpackages(ctx, buildInput, node.BasePkgName, []string{pkgName})
		if err != nil {
			return nil, nil, err
		}

		if allInCache {
			allBuiltDeps = append(allBuiltDeps, cachedDeps...)
			continue
		}

		// Собираем пакет
		res, err := b.BuildPackageFromDb(
			ctx,
			&BuildPackageFromDbArgs{
				Package:  node.Package,
				Packages: []string{pkgName},
				BuildArgs: BuildArgs{
					Opts:             input.BuildOpts(),
					Info:             input.OSRelease(),
					PkgFormat_:       input.PkgFormat(),
					SkipDepsBuilding: true,
				},
			},
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to build %s: %w", pkgName, err)
		}

		allBuiltDeps = append(allBuiltDeps, res...)
	}

	return allBuiltDeps, repoDeps, nil
}

// findAllSubpackages находит все подпакеты для базового пакета
func (b *Builder) findAllSubpackages(ctx context.Context, basePkgName, repository string) ([]string, error) {
	// Запрашиваем все пакеты с этим basepkg_name
	pkgs, _, err := b.repos.FindPkgs(ctx, []string{basePkgName})
	if err != nil {
		return nil, err
	}

	var subpkgs []string
	seen := make(map[string]bool)

	for _, pkgList := range pkgs {
		for _, pkg := range pkgList {
			// Проверяем, что это пакет из нужного репозитория
			if pkg.Repository == repository {
				pkgBase := pkg.BasePkgName
				if pkgBase == "" {
					pkgBase = pkg.Name
				}

				// Добавляем только если это пакет с нужным BasePkgName
				if pkgBase == basePkgName && !seen[pkg.Name] {
					subpkgs = append(subpkgs, pkg.Name)
					seen[pkg.Name] = true
				}
			}
		}
	}

	return subpkgs, nil
}

// checkCacheForAllSubpackages проверяет кеш для всех подпакетов
func (b *Builder) checkCacheForAllSubpackages(
	ctx context.Context,
	buildInput *BuildInput,
	basePkgName string,
	subpkgs []string,
) ([]*BuiltDep, bool, error) {
	var cachedDeps []*BuiltDep
	allInCache := true

	// funcsKey — ключ функций скрипта, общий для всех подпакетов
	// одного basePkgName; вычисляется лениво при первой проверке
	var funcsKey string
	var funcsKeySet bool

	// Получаем информацию обо всех подпакетах
	pkgsInfo, _, err := b.repos.FindPkgs(ctx, subpkgs)
	if err != nil {
		return nil, false, fmt.Errorf("failed to find subpackages info: %w", err)
	}

	for _, pkgName := range subpkgs {
		var pkgForCheck *alrsh.Package

		// Находим Package для подпакета
		if pkgList, ok := pkgsInfo[pkgName]; ok && len(pkgList) > 0 {
			pkgForCheck = &pkgList[0]
		}

		if pkgForCheck != nil {
			// funcsKey считаем лениво один раз на вызов: все подпакеты
			// одного basePkgName используют один и тот же скрипт
			if !funcsKeySet {
				funcsKey, funcsKeySet = b.scriptFuncsKey(ctx, pkgForCheck), true
			}

			pkgPath, found, err := b.cacheExecutor.CheckForBuiltPackage(ctx, buildInput, pkgForCheck, funcsKey)
			if err != nil {
				return nil, false, fmt.Errorf("failed to check cache: %w", err)
			}

			if found {
				slog.Info(gotext.Get("Using cached package"), "name", pkgName, "path", pkgPath)
				cachedDeps = append(cachedDeps, &BuiltDep{
					Name: pkgName,
					Path: pkgPath,
				})
			} else {
				allInCache = false
				break
			}
		}
	}

	return cachedDeps, allInCache && len(cachedDeps) > 0, nil
}

// scriptFuncsKey резолвит и читает скрипт пакета и возвращает ключ его
// сборочных функций. Любая ошибка резолва или чтения — безопасная сторона:
// пустой ключ не совпадёт с записанной метой, и пакет пересоберётся.
func (b *Builder) scriptFuncsKey(ctx context.Context, pkg *alrsh.Package) string {
	scriptInfo := b.scriptResolver.ResolveScript(ctx, pkg)

	sf, err := b.scriptExecutor.ReadScript(ctx, scriptInfo.Script)
	if err != nil {
		slog.Debug("Не удалось прочитать скрипт для проверки кэша, пересборка", "pkg", pkg.Name, "error", err)
		return ""
	}

	return ScriptFuncsKey(sf)
}

func (i *Builder) installBuildDeps(
	ctx context.Context,
	input interface {
		OsInfoProvider
		BuildOptsProvider
		PkgFormatProvider
	},
	pkgs []string,
) ([]*BuiltDep, []string, error) {
	var builtDeps []*BuiltDep
	var deps []string
	var err error
	if len(pkgs) > 0 {
		deps, err = i.installerExecutor.RemoveAlreadyInstalled(ctx, pkgs)
		if err != nil {
			return nil, nil, err
		}

		builtDeps, err = i.InstallPkgs(ctx, input, deps) // Устанавливаем выбранные пакеты
		if err != nil {
			return nil, nil, err
		}
	}
	return builtDeps, deps, nil
}

func (i *Builder) installOptDeps(
	ctx context.Context,
	input interface {
		OsInfoProvider
		BuildOptsProvider
		PkgFormatProvider
	},
	pkgs []string,
) ([]*BuiltDep, error) {
	var builtDeps []*BuiltDep
	optDeps, err := i.installerExecutor.RemoveAlreadyInstalled(ctx, pkgs)
	if err != nil {
		return nil, err
	}
	if len(optDeps) > 0 {
		optDeps, err := cliutils.ChooseOptDepends(
			ctx,
			optDeps,
			"install",
			input.BuildOpts().Interactive,
		) // Пользователя просят выбрать опциональные зависимости
		if err != nil {
			return nil, err
		}

		if len(optDeps) == 0 {
			return builtDeps, nil
		}

		builtDeps, err = i.InstallPkgs(ctx, input, optDeps) // Устанавливаем выбранные пакеты
		if err != nil {
			return nil, err
		}
	}
	return builtDeps, nil
}

// printDependencyTree рекурсивно выводит дерево зависимостей
// path используется для отслеживания текущего пути (для обнаружения циклов)
// printed используется для отслеживания уже выведенных пакетов (дедупликация)
func (i *Builder) printDependencyTree(
	w *os.File,
	node *DependencyNode,
	tree *UnifiedDependencyTree,
	prefix string,
	isLast bool,
	path map[string]bool,
	printed map[string]bool,
	redColor, yellowColor, blueColor, grayColor lipgloss.Color,
	versionStyle, repoStyle lipgloss.Style,
) {
	// Проверяем цикл на текущем пути
	if path[node.PkgName] {
		// Цикл обнаружен, показываем [circular]
		connectorStyle := lipgloss.NewStyle().Foreground(grayColor)
		circularStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
		var connector string
		if isLast {
			connector = "└── "
		} else {
			connector = "├── "
		}
		fmt.Fprintf(w, "%s%s%s\n", prefix, connectorStyle.Render(connector), circularStyle.Render(node.PkgName+" [circular]"))
		return
	}

	// Проверяем, выводился ли уже этот пакет (дедупликация)
	if printed[node.PkgName] {
		// Пакет уже выводился, показываем только имя без зависимостей
		connectorStyle := lipgloss.NewStyle().Foreground(grayColor)
		dupStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
		var connector string
		if isLast {
			connector = "└── "
		} else {
			connector = "├── "
		}
		fmt.Fprintf(w, "%s%s%s\n", prefix, connectorStyle.Render(connector), dupStyle.Render(node.PkgName+" [already listed]"))
		return
	}

	// Отмечаем пакет как выведенный
	printed[node.PkgName] = true

	// Добавляем текущий узел в путь
	path[node.PkgName] = true
	defer delete(path, node.PkgName) // Удаляем при выходе из функции

	// Определяем символы для дерева
	var connector string
	if isLast {
		connector = "└── "
	} else {
		connector = "├── "
	}

	// Выбираем цвет в зависимости от типа пакета
	var pkgColor lipgloss.Color
	if node.IsTarget {
		pkgColor = redColor // Целевые пакеты - красные
	} else {
		pkgColor = yellowColor // Зависимости - желтые
	}

	pkgStyle := lipgloss.NewStyle().Foreground(pkgColor)
	connectorStyle := lipgloss.NewStyle().Foreground(grayColor)

	// Формируем строку с пакетом
	var pkgLine string
	if node.Package != nil {
		version := versionStyle.Render(node.Package.Version)
		repo := repoStyle.Render("(" + node.Package.Repository + ")")
		pkgLine = fmt.Sprintf("%s %s %s", pkgStyle.Render(node.PkgName), version, repo)
	} else {
		pkgLine = pkgStyle.Render(node.PkgName)
	}

	fmt.Fprintf(w, "%s%s%s\n", prefix, connectorStyle.Render(connector), pkgLine)

	// Рекурсивно выводим зависимости
	childPrefix := prefix
	if isLast {
		childPrefix += "    "
	} else {
		childPrefix += "│   "
	}

	// Собираем всех детей (runtime зависимости + системные зависимости)
	children := node.Dependencies
	sysDeps := node.SystemDeps

	// Сначала ALR зависимости
	for idx, childName := range children {
		if childNode, ok := tree.Nodes[childName]; ok {
			isLastChild := idx == len(children)-1 && len(sysDeps) == 0
			i.printDependencyTree(w, childNode, tree, childPrefix, isLastChild, path, printed,
				redColor, yellowColor, blueColor, grayColor, versionStyle, repoStyle)
		} else {
			// Проверяем, является ли зависимость метапакетом
			var subpkgs []*DependencyNode
			for _, node := range tree.Nodes {
				if node.BasePkgName == childName {
					subpkgs = append(subpkgs, node)
				}
			}
			if len(subpkgs) > 0 {
				isLastChild := idx == len(children)-1 && len(sysDeps) == 0
				i.printMetaPackageTree(w, childName, subpkgs, tree, childPrefix, isLastChild, printed,
					redColor, yellowColor, blueColor, grayColor, versionStyle, repoStyle)
			}
		}
	}

	// Затем системные зависимости (синим цветом)
	if len(sysDeps) > 0 {
		sysStyle := lipgloss.NewStyle().Foreground(blueColor)
		for idx, sysPkg := range sysDeps {
			isLastSys := idx == len(sysDeps)-1
			var sysConnector string
			if isLastSys {
				sysConnector = "└── "
			} else {
				sysConnector = "├── "
			}
			fmt.Fprintf(w, "%s%s%s\n", childPrefix, connectorStyle.Render(sysConnector), sysStyle.Render(sysPkg))
		}
	}
}

// printMetaPackageTree выводит метапакет и его подпакеты
func (i *Builder) printMetaPackageTree(
	w *os.File,
	metaPkgName string,
	subpkgs []*DependencyNode,
	tree *UnifiedDependencyTree,
	prefix string,
	isLast bool,
	printed map[string]bool,
	redColor, yellowColor, blueColor, grayColor lipgloss.Color,
	versionStyle, repoStyle lipgloss.Style,
) {
	// Определяем символы для дерева
	var connector string
	if isLast {
		connector = "└── "
	} else {
		connector = "├── "
	}

	connectorStyle := lipgloss.NewStyle().Foreground(grayColor)
	metaStyle := lipgloss.NewStyle().Foreground(redColor).Bold(true)

	// Выводим имя метапакета
	fmt.Fprintf(w, "%s%s%s\n", prefix, connectorStyle.Render(connector), metaStyle.Render(metaPkgName))

	// Префикс для дочерних элементов
	var childPrefix string
	if isLast {
		childPrefix = prefix + "    "
	} else {
		childPrefix = prefix + "│   "
	}

	// Выводим подпакеты
	for idx, subpkg := range subpkgs {
		isLastSub := idx == len(subpkgs)-1
		path := make(map[string]bool)
		i.printDependencyTree(w, subpkg, tree, childPrefix, isLastSub, path, printed,
			redColor, yellowColor, blueColor, grayColor, versionStyle, repoStyle)
	}
}

func (i *Builder) InstallPkgs(
	ctx context.Context,
	input interface {
		OsInfoProvider
		BuildOptsProvider
		PkgFormatProvider
	},
	pkgs []string,
) ([]*BuiltDep, error) {
	if len(pkgs) == 0 {
		return nil, nil
	}

	slog.Debug(fmt.Sprintf("[TIME: %s] InstallPkgs: starting", time.Now().Format("15:04:05.000")), "packages", len(pkgs))

	// Шаг 1: Построить единое дерево зависимостей
	slog.Debug(fmt.Sprintf("[TIME: %s] InstallPkgs: resolving dependency tree...", time.Now().Format("15:04:05.000")))
	tree, err := i.ResolveUnifiedDependencyTree(ctx, input, pkgs)
	slog.Debug(fmt.Sprintf("[TIME: %s] InstallPkgs: dependency tree resolved", time.Now().Format("15:04:05.000")))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve dependency tree: %w", err)
	}

	// Формируем список целевых пакетов с версиями
	var pkgList []string
	for _, pkgName := range pkgs {
		if node, ok := tree.Nodes[pkgName]; ok && node.Package != nil {
			pkgList = append(pkgList, fmt.Sprintf("%s-%s", pkgName, node.Package.Version))
		} else {
			pkgList = append(pkgList, pkgName)
		}
	}
	slog.Info(gotext.Get("Resolving dependencies for packages"), "packages", pkgList)

	slog.Debug(gotext.Get("Dependency tree resolved"),
		"alr_packages", len(tree.AllALRPackages),
		"system_deps", len(tree.AllSystemDeps),
		"opt_deps", len(tree.AllOptDeps),
		"build_deps", len(tree.AllBuildDeps))

	// Помечаем целевые пакеты
	targetSet := make(map[string]bool)
	for _, pkgName := range pkgs {
		targetSet[pkgName] = true
		if node, ok := tree.Nodes[pkgName]; ok {
			node.IsTarget = true
		} else {
			// Пакет не найден напрямую, возможно это basepkg_name мультипакета
			// Ищем все подпакеты с этим basepkg_name и помечаем их как целевые
			for _, node := range tree.Nodes {
				if node.BasePkgName == pkgName {
					node.IsTarget = true
				}
			}
		}
	}

	// Формируем полный список пакетов для сборки (зависимости + целевые)
	var allPackages []string
	allPackages = append(allPackages, tree.AllALRPackages...)
	for _, pkgName := range pkgs {
		// Добавляем целевые пакеты если их ещё нет в списке
		found := false
		for _, existing := range allPackages {
			if existing == pkgName {
				found = true
				break
			}
		}
		if !found {
			allPackages = append(allPackages, pkgName)
		}
	}

	// Показываем сводку и запрашиваем подтверждение (один раз)
	userConfirmed := false
	if input.BuildOpts().Interactive && (len(tree.AllSystemDeps) > 0 || len(allPackages) > 0) {
		// Цвета из прогресс-бара: красный -> желтый -> синий
		redColor := lipgloss.Color("#ef4444")    // Целевые пакеты
		yellowColor := lipgloss.Color("#eab308") // Зависимости ALR
		blueColor := lipgloss.Color("#3b82f6")   // Системные пакеты
		grayColor := lipgloss.Color("#64748B")   // Разделители дерева

		titleStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#6C5DD3"))
		versionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#94A3B8"))

		repoStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#475569"))

		fmt.Fprintln(os.Stderr, titleStyle.Render("📦 "+gotext.Get("Installation summary")))

		// Рисуем дерево зависимостей (path отслеживает текущий путь для обнаружения циклов)
		// printed отслеживает уже выведенные пакеты для дедупликации
		printed := make(map[string]bool)
		for _, pkgName := range pkgs {
			if node, ok := tree.Nodes[pkgName]; ok {
				path := make(map[string]bool)
				i.printDependencyTree(os.Stderr, node, tree, "", true, path, printed,
					redColor, yellowColor, blueColor, grayColor, versionStyle, repoStyle)
			} else {
				// Проверяем, является ли пакет метапакетом (есть подпакеты с таким BasePkgName)
				var subpkgs []*DependencyNode
				for _, node := range tree.Nodes {
					if node.BasePkgName == pkgName {
						subpkgs = append(subpkgs, node)
					}
				}
				if len(subpkgs) > 0 {
					// Выводим метапакет и его подпакеты
					i.printMetaPackageTree(os.Stderr, pkgName, subpkgs, tree, "", true, printed,
						redColor, yellowColor, blueColor, grayColor, versionStyle, repoStyle)
				}
			}
		}

		// Запрашиваем подтверждение через survey (корректно обрабатывает Ctrl+C)
		cont, err := cliutils.YesNoPrompt(ctx, gotext.Get("Proceed with installation?"), true, true)
		if err != nil {
			// Пользователь нажал Ctrl+C или произошла ошибка ввода
			fmt.Fprintln(os.Stderr)
			return nil, errors.New("installation cancelled by user")
		}
		if !cont {
			return nil, errors.New("installation cancelled by user")
		}

		// После подтверждения устанавливаем всё без дополнительных запросов
		userConfirmed = true
	}

	var allBuiltDeps []*BuiltDep
	var targetDeps []*BuiltDep
	var installedBuildDeps []string

	// Шаг 2: Устанавливаем ВСЕ системные зависимости одним вызовом
	if len(tree.AllSystemDeps) > 0 {
		slog.Info(gotext.Get("Installing system dependencies"), "count", len(tree.AllSystemDeps))
		err = i.installerExecutor.Install(ctx, tree.AllSystemDeps, &manager.Opts{
			NoConfirm: userConfirmed || !input.BuildOpts().Interactive,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to install system dependencies: %w", err)
		}
		_ = i.installerExecutor.CheckVersionsAfterInstall(ctx, tree.AllSystemDeps)
	}

	// Шаг 3: Обрабатываем опциональные зависимости (если есть)
	if len(tree.AllOptDeps) > 0 && input.BuildOpts().Interactive {
		slog.Info(gotext.Get("Processing optional dependencies"))
		_, err := i.installOptDeps(ctx, input, tree.AllOptDeps)
		if err != nil {
			return nil, fmt.Errorf("failed to install optional dependencies: %w", err)
		}
	}

	// Шаг 4: Показываем скрипты всех пакетов перед сборкой
	if input.BuildOpts().Interactive {
		var scripts []cliutils.ScriptInfo
		for _, pkgName := range allPackages {
			if node, ok := tree.Nodes[pkgName]; ok && node.Package != nil {
				scriptInfo := i.scriptResolver.ResolveScript(ctx, node.Package)
				scripts = append(scripts, cliutils.ScriptInfo{
					Name:       pkgName,
					ScriptPath: scriptInfo.Script,
				})
			}
		}

		if len(scripts) > 0 {
			cont, err := cliutils.PromptViewMultipleScripts(ctx, scripts, i.cfg.PagerStyle(), input.BuildOpts().Interactive)
			if err != nil {
				return nil, err
			}
			if !cont {
				return nil, errors.New("user cancelled")
			}
		}
	}

	// Шаг 5: Собираем и устанавливаем все ALR пакеты в правильном порядке
	slog.Info(gotext.Get("Building %d packages", len(allPackages)))
	slog.Debug("Package build order", "order", allPackages)

	for _, pkgName := range allPackages {
		node, ok := tree.Nodes[pkgName]
		if !ok || node == nil || node.Package == nil {
			slog.Debug(gotext.Get("Package %s not found in tree, skipping", pkgName))
			continue
		}

		pkg := node.Package
		basePkgName := node.BasePkgName

		// Очищаем имя пакета от суффикса репозитория (+repo) перед сборкой,
		// так как в alr.sh функции meta_*, package_*, files_* не содержат суффикса.
		cleanPkgName := StripRepoSuffix(pkgName)

		// Проверяем нужна ли сборка
		needBuildPkgs, err := i.installerExecutor.FilterPackagesByVersion(ctx, []alrsh.Package{*pkg}, input.OSRelease())
		if err != nil {
			return nil, fmt.Errorf("failed to filter package %s: %w", pkgName, err)
		}

		if len(needBuildPkgs) == 0 && !node.IsTarget {
			slog.Debug(gotext.Get("Package %s already installed, skipping", pkgName))
			continue
		}

		// Проверяем кеш
		scriptInfo := i.scriptResolver.ResolveScript(ctx, pkg)
		buildInput := &BuildInput{
			script:     scriptInfo.Script,
			repository: scriptInfo.Repository,
			packages:   []string{cleanPkgName},
			pkgFormat:  input.PkgFormat(),
			opts:       input.BuildOpts(),
			info:       input.OSRelease(),
		}

		cachedDeps, allInCache, err := i.checkCacheForAllSubpackages(ctx, buildInput, basePkgName, []string{cleanPkgName})
		if err != nil {
			return nil, err
		}

		if allInCache {
			slog.Info(gotext.Get("Using cached package"), "name", pkgName)
			// Целевые пакеты откладываем для финальной установки
			if node.IsTarget {
				targetDeps = append(targetDeps, cachedDeps...)
			} else {
				allBuiltDeps = append(allBuiltDeps, cachedDeps...)
				// Устанавливаем кешированный пакет сразу
				if len(cachedDeps) > 0 {
					err = i.installerExecutor.InstallLocal(ctx, GetBuiltPaths(cachedDeps), &manager.Opts{
						NoConfirm: userConfirmed || !input.BuildOpts().Interactive,
					})
					if err != nil {
						return nil, fmt.Errorf("failed to install cached %s: %w", pkgName, err)
					}
				}
			}
			continue
		}

		// Собираем пакет
		if node.IsTarget {
			slog.Info(gotext.Get("Building package %s-%s", pkgName, pkg.Version))
		} else {
			slog.Info(gotext.Get("Building dependency %s-%s", pkgName, pkg.Version))
		}

		res, err := i.BuildPackageFromDb(
			ctx,
			&BuildPackageFromDbArgs{
				Package:  pkg,
				Packages: []string{cleanPkgName},
				BuildArgs: BuildArgs{
					Opts:             input.BuildOpts(),
					Info:             input.OSRelease(),
					PkgFormat_:       input.PkgFormat(),
					SkipDepsBuilding: true, // Все зависимости уже установлены
					SkipBuildDeps:    true, // build_deps уже установлены в InstallPkgs
				},
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to build %s: %w", pkgName, err)
		}

		// Для целевых пакетов откладываем установку в финальный шаг,
		// для зависимостей устанавливаем сразу
		if node.IsTarget {
			targetDeps = append(targetDeps, res...)
		} else {
			allBuiltDeps = append(allBuiltDeps, res...)
		}

		// Устанавливаем собранный пакет сразу, чтобы он был доступен для следующих
		if len(res) > 0 && !node.IsTarget {
			err = i.installerExecutor.InstallLocal(ctx, GetBuiltPaths(res), &manager.Opts{
				NoConfirm: userConfirmed || !input.BuildOpts().Interactive,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to install %s: %w", pkgName, err)
			}
		}

		// Собираем установленные build deps для удаления в конце
		installedBuildDeps = append(installedBuildDeps, node.BuildDeps...)
	}

	// Шаг 6: Устанавливаем целевые пакеты
	if len(targetDeps) > 0 {
		slog.Info(gotext.Get("Installing target packages"))

		// Перепроверяем владельца кэшированных пакетов: между попаданием
		// в кэш и установкой файл в общем кэше мог быть подменён
		if err := verifyTrustedPaths(GetBuiltPaths(targetDeps), i.trustedUIDs...); err != nil {
			return nil, err
		}

		err = i.installerExecutor.InstallLocal(ctx, GetBuiltPaths(targetDeps), &manager.Opts{
			NoConfirm: userConfirmed || !input.BuildOpts().Interactive,
		})
		if err != nil {
			return nil, err
		}

		// Отслеживание установки
		for _, dep := range targetDeps {
			if stats.ShouldTrackPackage(dep.Name) {
				stats.TrackInstallation(ctx, dep.Name, "install")
			}
		}
	}

	// Шаг 7: Один финальный промпт на удаление всех build зависимостей
	installedBuildDeps = removeDuplicates(installedBuildDeps)
	if len(installedBuildDeps) > 0 {
		remove, err := cliutils.YesNoPrompt(ctx, gotext.Get("Would you like to remove all build dependencies?"), input.BuildOpts().Interactive, false)
		if err != nil {
			return nil, err
		}

		if remove {
			err = i.installerExecutor.Remove(ctx, installedBuildDeps, &manager.Opts{
				NoConfirm: !input.BuildOpts().Interactive,
			})
			if err != nil {
				slog.Warn(gotext.Get("Failed to remove build dependencies: %v", err))
			}
		}
	}

	// Объединяем зависимости и целевые пакеты для возврата
	return append(allBuiltDeps, targetDeps...), nil
}
