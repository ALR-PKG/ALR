# Optional Dependency Owners Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Показать в общем меню, к каким пакетам относится каждая опциональная зависимость.

**Architecture:** Дерево сохранит упорядоченных владельцев каждой зависимости без дублирования самой зависимости. Меню добавит локализованный суффикс `(для pkg-a, pkg-b)`; однопакетный путь без карты владельцев останется прежним.

**Tech Stack:** Go, gotext, существующий survey MultiSelect.

**Spec:** Выбранный в диалоге вариант: владельцы в каждой строке общего меню.

## Global Constraints

- Не менять выбор, фильтрацию, значения по умолчанию и установку пакетов.
- Общая зависимость нескольких пакетов показывается один раз со всеми владельцами.
- Не создавать коммит без отдельной просьбы.

## Review Focus

- Повторная зависимость добавляет владельца, но не строку меню.
- Зависимость без владельцев не получает пустой суффикс.
- Описание после `: ` сохраняется перед суффиксом.

---

### Task 1: Владельцы в дереве и меню

**Files:**
- Modify: `internal/build/dependency_tree.go`
- Modify: `internal/build/build.go`
- Test: `internal/build/dependency_tree_test.go` или существующий профильный тестовый файл

**Interfaces:**
- Produces: `UnifiedDependencyTree.OptDepOwners map[string][]string`; `installOptDeps` получает карту владельцев.

- [ ] Написать failing-тесты на уникальность строки, список владельцев и формат `dep: description (for pkg-a, pkg-b)`.
- [ ] Запустить `go test ./internal/build -run OptDep -v` и увидеть FAIL.
- [ ] Добавить заполнение карты для обычных пакетов и подпакетов, форматирование суффикса через `gotext.Get(" (for %s)")` и передачу карты из дерева.
- [ ] Обновить переводы через `make i18n`.
- [ ] Запустить профильные тесты, `go test ./...`, `go build ./...` и линтер изменённых пакетов.
