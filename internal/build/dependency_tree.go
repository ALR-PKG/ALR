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
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/leonelquinteros/gotext"

	"git.alr-pkg.ru/ALR/ALR/internal/cliutils"
	"git.alr-pkg.ru/ALR/ALR/internal/overrides"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
	"git.alr-pkg.ru/ALR/ALR/pkg/depver"
)

// DependencyNode представляет узел в дереве зависимостей
type DependencyNode struct {
	Package      *alrsh.Package
	BasePkgName  string
	PkgName      string   // Имя конкретного подпакета (может отличаться от BasePkgName)
	Dependencies []string // Имена runtime зависимостей (depends)
	BuildDeps    []string // Имена зависимостей для сборки (build_deps)
	SystemDeps   []string // Системные зависимости (не найденные в ALR)
	IsTarget     bool     // true если это целевой пакет (запрошен пользователем)
}

// UnifiedDependencyTree представляет единое дерево зависимостей для однопроходной установки
type UnifiedDependencyTree struct {
	Nodes          map[string]*DependencyNode
	AllALRPackages []string // Все ALR пакеты в порядке топологической сортировки (от листьев)
	AllSystemDeps  []string // Все системные зависимости (build + runtime)
	AllOptDeps     []string // Все опциональные зависимости
	AllBuildDeps   []string // Все build зависимости для отслеживания
}

// ResolveUnifiedDependencyTree строит единое дерево зависимостей
// и возвращает все пакеты в порядке топологической сортировки
func (b *Builder) ResolveUnifiedDependencyTree(
	ctx context.Context,
	input interface {
		OsInfoProvider
		BuildOptsProvider
		PkgFormatProvider
	},
	initialPkgs []string,
) (*UnifiedDependencyTree, error) {
	tree := &UnifiedDependencyTree{
		Nodes:          make(map[string]*DependencyNode),
		AllALRPackages: []string{},
		AllSystemDeps:  []string{},
		AllOptDeps:     []string{},
		AllBuildDeps:   []string{},
	}

	visited := make(map[string]bool)
	systemVisited := make(map[string]bool)
	optVisited := make(map[string]bool)
	buildDepVisited := make(map[string]bool)

	overrideNames, err := overrides.Resolve(input.OSRelease(), overrides.DefaultOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve overrides: %w", err)
	}

	// Временное хранение порядка (реверсивное - от корня к листьям)
	var order []string

	// resolve рекурсивно разрешает зависимости
	// parentPkg - имя пакета, который запрашивает эти зависимости (для привязки системных зависимостей)
	var resolve func(pkgNames []string, isBuildDep bool, parentPkg string) error
	resolveCallCount := 0
	totalProcessed := 0

	// Счётчик обработанных пакетов для отладки

	resolve = func(pkgNames []string, isBuildDep bool, parentPkg string) error {
		resolveCallCount++
		if len(pkgNames) == 0 {
			return nil
		}

		slog.Debug(fmt.Sprintf("[TIME: %s] Resolving dependencies", time.Now().Format("15:04:05.000")), "call", resolveCallCount, "packages", len(pkgNames), "parent", parentPkg)

		// Находим пакеты
		found, notFound, err := b.repos.FindPkgs(ctx, pkgNames)
		slog.Debug(fmt.Sprintf("[TIME: %s] FindPkgs completed", time.Now().Format("15:04:05.000")), "call", resolveCallCount, "found", len(found), "notFound", len(notFound))
		if err != nil {
			return fmt.Errorf("failed to find packages: %w", err)
		}

		// Проверяем preferALRDeps: системный пакет предпочитается ALR-пакету.
		// Применяем только к зависимостям (parentPkg != ""): целевые пакеты,
		// которые пользователь явно просит установить/обновить через ALR,
		// системной замене не подлежат — иначе, например, upgrade уходит
		// в системный менеджер и фактически не выполняется
		if parentPkg != "" && b.cfg != nil && !b.cfg.PreferALRDeps() && b.mgr != nil {
			for pkgName := range found {
				available, err := b.mgr.ListAvailable(pkgName)
				if err != nil {
					continue
				}
				for _, av := range available {
					if av == pkgName {
						slog.Debug("Preferring system package over ALR", "pkg", pkgName)
						delete(found, pkgName)
						notFound = append(notFound, pkgName)
						break
					}
				}
			}
		}

		// Обрабатываем системные зависимости - привязываем к родительскому пакету
		for _, pkgName := range notFound {
			if !systemVisited[pkgName] {
				systemVisited[pkgName] = true
				tree.AllSystemDeps = append(tree.AllSystemDeps, pkgName)
			}
			// Привязываем системную зависимость к родительскому пакету
			if parentPkg != "" {
				if node, ok := tree.Nodes[parentPkg]; ok {
					// Проверяем, не добавлена ли уже
					alreadyAdded := false
					for _, sd := range node.SystemDeps {
						if sd == pkgName {
							alreadyAdded = true
							break
						}
					}
					if !alreadyAdded {
						node.SystemDeps = append(node.SystemDeps, pkgName)
					}
				}
			}
			// Отслеживаем build зависимости для удаления
			if isBuildDep && !buildDepVisited[pkgName] {
				buildDepVisited[pkgName] = true
				tree.AllBuildDeps = append(tree.AllBuildDeps, pkgName)
			}
		}

		// Обрабатываем найденные ALR пакеты
		pkgCounter := 0
		for pkgName, pkgList := range found {
			pkgCounter++
			totalProcessed++

			if len(pkgList) == 0 {
				continue
			}

			// Несколько РАЗНЫХ пакетов под одно имя (например name, name-git,
			// name-bin, найденные через provides) — это не мультипакет.
			// Если же в списке есть подпакеты мультипакета с basepkg_name,
			// совпадающим с запросом, — это запрос мультипакета: сторонние
			// provides-совпадения отбрасываем
			if len(pkgList) > 1 && !cliutils.IsMultiPackage(pkgList) {
				if subset := multiPackageSubset(pkgList, variantQueryName(pkgName)); len(subset) > 1 {
					pkgList = subset
				} else {
					// Выбираем один вариант: для целевых пакетов в интерактивном
					// режиме показываем меню, иначе предпочитаем точное
					// совпадение имени, а при его отсутствии — первый вариант
					pkgList = selectPackageVariant(ctx, pkgName, pkgList, parentPkg, input.BuildOpts().Interactive)
				}
			}

			// Проверяем, найден ли пакет по basepkg_name (мультипакет)
			// В этом случае pkgName - это basepkg_name, а pkgList содержит все подпакеты
			// Если найдено несколько пакетов - это метапакет
			isMetaPackage := len(pkgList) > 1

			if isMetaPackage {
				// Обрабатываем все подпакеты мультипакета
				for _, pkg := range pkgList {
					subPkgName := pkg.Name
					if visited[subPkgName] {
						continue
					}
					visited[subPkgName] = true

					if pkgCounter%5 == 0 || pkgCounter == 1 {
						slog.Debug(fmt.Sprintf("[TIME: %s] Processing package", time.Now().Format("15:04:05.000")), "call", resolveCallCount, "pkg", pkgCounter, "name", subPkgName, "total", totalProcessed)
					}

					alrsh.ResolvePackage(&pkg, overrideNames)

					baseName := pkg.BasePkgName
					if baseName == "" {
						baseName = pkg.Name
					}

					deps := pkg.Depends.Resolved()
					buildDeps := pkg.BuildDepends.Resolved()
					optDeps := pkg.OptDepends.Resolved()

					// Добавляем узел с реальным именем подпакета
					tree.Nodes[subPkgName] = &DependencyNode{
						Package:      &pkg,
						BasePkgName:  baseName,
						PkgName:      subPkgName,
						Dependencies: deps,
						BuildDeps:    buildDeps,
					}

					// Собираем опциональные зависимости
					for _, optDep := range optDeps {
						if !optVisited[optDep] {
							optVisited[optDep] = true
							tree.AllOptDeps = append(tree.AllOptDeps, optDep)
						}
					}

					// Отслеживаем build зависимости для удаления
					slog.Debug(fmt.Sprintf("[TIME: %s] Checking build deps", time.Now().Format("15:04:05.000")), "pkg", subPkgName, "count", len(buildDeps))
					for i, bd := range buildDeps {
						if b.mgr != nil {
							if i%5 == 0 {
								slog.Debug(fmt.Sprintf("[TIME: %s] IsAvailable", time.Now().Format("15:04:05.000")), "pkg", subPkgName, "dep", bd, "idx", i)
							}
							isAvail, _ := b.mgr.IsAvailable(bd)
							if isAvail && !buildDepVisited[bd] {
								buildDepVisited[bd] = true
								tree.AllBuildDeps = append(tree.AllBuildDeps, bd)
							}
						}
					}

					// Рекурсивно обрабатываем зависимости
					if len(buildDeps) > 0 {
						if err := resolve(buildDeps, true, subPkgName); err != nil {
							return err
						}
					}
					if len(deps) > 0 {
						if err := resolve(deps, false, subPkgName); err != nil {
							return err
						}
					}

					// Добавляем пакет в порядок
					order = append(order, subPkgName)
				}
			} else {
				// Обычный пакет (не мультипакет)
				if visited[pkgName] {
					continue
				}
				visited[pkgName] = true

				if pkgCounter%5 == 0 || pkgCounter == 1 {
					slog.Debug(fmt.Sprintf("[TIME: %s] Processing package", time.Now().Format("15:04:05.000")), "call", resolveCallCount, "pkg", pkgCounter, "name", pkgName, "total", totalProcessed)
				}

				pkg := pkgList[0]
				alrsh.ResolvePackage(&pkg, overrideNames)

				baseName := pkg.BasePkgName
				if baseName == "" {
					baseName = pkg.Name
				}

				deps := pkg.Depends.Resolved()
				buildDeps := pkg.BuildDepends.Resolved()
				optDeps := pkg.OptDepends.Resolved()

				// Добавляем узел
				tree.Nodes[pkgName] = &DependencyNode{
					Package:      &pkg,
					BasePkgName:  baseName,
					PkgName:      pkgName,
					Dependencies: deps,
					BuildDeps:    buildDeps,
				}

				// Собираем опциональные зависимости
				for _, optDep := range optDeps {
					if !optVisited[optDep] {
						optVisited[optDep] = true
						tree.AllOptDeps = append(tree.AllOptDeps, optDep)
					}
				}

				// Отслеживаем build зависимости для удаления
				slog.Debug(fmt.Sprintf("[TIME: %s] Checking build deps", time.Now().Format("15:04:05.000")), "pkg", pkgName, "count", len(buildDeps))
				for i, bd := range buildDeps {
					if b.mgr != nil {
						if i%5 == 0 {
							slog.Debug(fmt.Sprintf("[TIME: %s] IsAvailable", time.Now().Format("15:04:05.000")), "pkg", pkgName, "dep", bd, "idx", i)
						}
						isAvail, _ := b.mgr.IsAvailable(bd)
						if isAvail && !buildDepVisited[bd] {
							buildDepVisited[bd] = true
							tree.AllBuildDeps = append(tree.AllBuildDeps, bd)
						}
					}
				}

				// Рекурсивно обрабатываем зависимости
				if len(buildDeps) > 0 {
					if err := resolve(buildDeps, true, pkgName); err != nil {
						return err
					}
				}
				if len(deps) > 0 {
					if err := resolve(deps, false, pkgName); err != nil {
						return err
					}
				}

				// Добавляем пакет в порядок
				order = append(order, pkgName)
			}
		}

		return nil
	}

	// Создаем множество целевых пакетов для исключения из AllALRPackages
	targetSet := make(map[string]bool)
	for _, pkgName := range initialPkgs {
		targetSet[pkgName] = true
	}

	// Начинаем разрешение с начальных пакетов (без родителя)
	if err := resolve(initialPkgs, false, ""); err != nil {
		return nil, err
	}

	// Порядок уже от листьев к корню (зависимости добавлены после рекурсии)
	// Исключаем целевые пакеты - они будут добавлены в конец в InstallPkgs
	slog.Debug("Dependency resolution order", "order", order)
	for _, pkgName := range order {
		if !targetSet[pkgName] {
			tree.AllALRPackages = append(tree.AllALRPackages, pkgName)
		}
	}
	slog.Debug("AllALRPackages after filtering", "packages", tree.AllALRPackages)

	return tree, nil
}

// TopologicalSort выполняет топологическую сортировку пакетов по зависимостям
// Возвращает список имен подпакетов в порядке сборки (от корней к листьям)
// Учитывает как runtime depends, так и build_deps
func TopologicalSort(nodes map[string]*DependencyNode) ([]string, error) {
	// Список для результата
	var result []string

	// Множество посещенных узлов
	visited := make(map[string]bool)

	// Множество узлов в текущем пути (для обнаружения циклов)
	inStack := make(map[string]bool)

	var visit func(pkgName string) error
	visit = func(pkgName string) error {
		if visited[pkgName] {
			return nil
		}

		if inStack[pkgName] {
			return fmt.Errorf("circular dependency detected: %s", pkgName)
		}

		node := nodes[pkgName]
		if node == nil {
			// Это системный пакет или пакет не в дереве, игнорируем
			return nil
		}

		inStack[pkgName] = true

		// Посещаем все зависимости (runtime + build)
		allDeps := append([]string{}, node.Dependencies...)
		allDeps = append(allDeps, node.BuildDeps...)

		for _, dep := range allDeps {
			// Используем имя зависимости напрямую (это имя подпакета)
			if err := visit(dep); err != nil {
				return err
			}
		}

		inStack[pkgName] = false
		visited[pkgName] = true
		result = append(result, pkgName)

		return nil
	}

	// Посещаем все узлы
	for pkgName := range nodes {
		if err := visit(pkgName); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// MarkTargetPackages помечает указанные пакеты как целевые (запрошенные пользователем)
func MarkTargetPackages(nodes map[string]*DependencyNode, targetPkgNames []string) {
	for _, name := range targetPkgNames {
		if node, ok := nodes[name]; ok {
			node.IsTarget = true
		}
	}
}

// GetAllDependencies возвращает полный список всех зависимостей (runtime + build)
// для всех пакетов в дереве, исключая целевые пакеты
func GetAllDependencies(nodes map[string]*DependencyNode) []string {
	depMap := make(map[string]bool)

	for _, node := range nodes {
		if node.IsTarget {
			// Для целевых пакетов не включаем их зависимости в общий список
			// они будут обработаны отдельно
			continue
		}

		// Добавляем все зависимости (runtime + build)
		for _, dep := range node.Dependencies {
			depMap[dep] = true
		}
		for _, dep := range node.BuildDeps {
			depMap[dep] = true
		}
	}

	var result []string
	for dep := range depMap {
		result = append(result, dep)
	}

	return result
}

// GetTargetPackages возвращает список целевых пакетов
func GetTargetPackages(nodes map[string]*DependencyNode) []*DependencyNode {
	var result []*DependencyNode
	for _, node := range nodes {
		if node.IsTarget {
			result = append(result, node)
		}
	}
	return result
}

// GetDependencyOnlyPackages возвращает список пакетов-зависимостей (не целевых)
func GetDependencyOnlyPackages(nodes map[string]*DependencyNode) []*DependencyNode {
	var result []*DependencyNode
	for _, node := range nodes {
		if !node.IsTarget {
			result = append(result, node)
		}
	}
	return result
}

// variantQueryName возвращает имя пакета из запроса без версионных
// ограничений и квалификатора репозитория (repo/pkg, pkg+repo)
func variantQueryName(query string) string {
	name := depver.Parse(query).Name
	if i := strings.Index(name, "/"); i > 0 {
		return name[i+1:]
	}
	if i := strings.Index(name, "+"); i > 0 {
		return name[:i]
	}
	return name
}

// multiPackageSubset возвращает подпакеты одного мультипакета,
// чей basepkg_name совпадает с запрошенным именем
func multiPackageSubset(pkgs []alrsh.Package, queryName string) []alrsh.Package {
	var subset []alrsh.Package
	repo := ""
	for _, p := range pkgs {
		if p.BasePkgName == "" || p.BasePkgName != queryName {
			continue
		}
		if repo == "" {
			repo = p.Repository
		}
		if p.Repository != repo {
			continue
		}
		subset = append(subset, p)
	}
	return subset
}

// selectPackageVariant выбирает один пакет из списка разных пакетов,
// найденных под одним именем (точное совпадение имени и provides-варианты).
// Список pkgs уже отсортирован FindPkgs: точные совпадения в начале
func selectPackageVariant(
	ctx context.Context,
	query string,
	pkgs []alrsh.Package,
	parentPkg string,
	interactive bool,
) []alrsh.Package {
	// Меню выбора — только для целевых пакетов в интерактивном режиме;
	// для зависимостей интерактивный выбор недопустим
	if parentPkg == "" && interactive {
		choice, err := cliutils.PkgPrompt(ctx, pkgs, "install", true)
		if err != nil {
			slog.Error(gotext.Get("Error prompting for choice of package"))
			os.Exit(1)
		}
		return []alrsh.Package{choice}
	}

	// Без интерактива предпочитаем точное совпадение имени
	queryName := variantQueryName(query)
	for _, p := range pkgs {
		if p.Name == queryName {
			return []alrsh.Package{p}
		}
	}
	return []alrsh.Package{pkgs[0]}
}
