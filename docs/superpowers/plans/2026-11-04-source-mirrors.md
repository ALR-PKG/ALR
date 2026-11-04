# Source Mirrors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Добавить в `alr.sh` наборы `sources_mirrorN` с последовательным fallback и общим кэшем источников.

**Architecture:** Динамический декодер заполняет упорядоченные `OverridableField` в модели пакета. Сборка передаёт связанные записи `Source`, а `pkg/dl` перебирает адреса и кэширует результат по каноническому URL.

**Tech Stack:** Go, `mvdan.cc/sh`, `go-git`, `httptest`, `testify`, существующие генераторы ALR.

**Spec:** `docs/superpowers/specs/2026-11-04-source-mirrors-design.md`

## Global Constraints

- Существующие `sources`, `checksums`, overrides и ключи кэша обратно совместимы.
- Порядок: `sources_mirror1`, `sources_mirror2`, …, основной `sources`.
- Checksum обязателен для каждого успешного кандидата, если он не `SKIP`.
- Новых внешних зависимостей не добавлять.
- Сообщения и пользовательская документация — на русском, идентификаторы — на языке проекта.

## Review Focus

- `sources_mirror1` не должен превращаться в override `sources`; тест Task 1.
- Дубликат источника в подпакетах не должен терять зеркала; тест Task 3.
- Ошибка обновления cached Git не должна блокировать чистую загрузку со следующего зеркала; тест Task 2.
- `~name` и `~archive` не должны меняться при смене адреса; тесты Task 2.
- Старые JSON/БД-записи без `source_mirrors` должны декодироваться; тест Task 4.

---

### Task 1: Разбор и валидация `sources_mirrorN`

**Files:**
- Create: `pkg/alrsh/source_mirrors.go`
- Create: `pkg/alrsh/source_mirrors_test.go`
- Modify: `pkg/alrsh/package.go:44-95`
- Modify: `internal/shutils/decoder/decoder.go:352-375`
- Modify: `internal/shutils/decoder/decoder_test.go`

**Interfaces:**
- Produces: `Package.SourceMirrors []OverridableField[[]string]`.
- Produces: `DecodeSourceMirrors(dec *decoder.Decoder) ([]OverridableField[[]string], error)` либо эквивалентный метод декодера без циклического импорта.
- Produces: `ValidateSourceMirrors(sources []string, mirrors [][]string) error`.

- [ ] Написать failing tests на два набора, пустые элементы, numeric ordering (`mirror10` после `mirror2`), distro override, пропуск номера, нулевой номер, неверную длину и отсутствие зеркал.
- [ ] Запустить `go test ./pkg/alrsh ./internal/shutils/decoder` и убедиться в ожидаемом FAIL.
- [ ] Добавить модель и динамический разбор по `^sources_mirror([1-9][0-9]*)(?:_(.+))?$`; вызывать его из обоих путей создания `Package` в `pkg/alrsh/alrsh.go`.
- [ ] Исключить `sources_mirrorN...` из `getVarsByPrefix("sources")`, оставив остальные prefix-overrides без изменений.
- [ ] Запустить те же пакетные тесты, затем `go test ./pkg/alrsh ./internal/shutils/decoder`; ожидается PASS.
- [ ] Закоммитить разбор и тесты.

### Task 2: Fallback и канонический кэш в `pkg/dl`

**Files:**
- Create: `pkg/dl/mirrors.go`
- Create: `pkg/dl/mirrors_test.go`
- Modify: `pkg/dl/dl.go:88-100,149-241`
- Modify: `pkg/dl/git.go`
- Modify: `pkg/dl/file.go`

**Interfaces:**
- Consumes: спецификацию порядка и наследования параметров.
- Produces: `Options.CacheID string` и `Options.FallbackURLs []string`; пустой `CacheID` означает `Options.URL`.
- Produces: агрегированную ошибку всех неудачных адресов.

- [ ] Написать `httptest`-тесты: первое зеркало успешно; 404 и checksum mismatch переключают на следующий адрес; upstream является последним; cancellation прекращает перебор; все ошибки видны в итоговой ошибке.
- [ ] Написать тесты единого кэша при разных успешных URL, наследования `~name`/`~archive`, запрета расходящихся `~name`/`~rev`, local source без сетевого fallback.
- [ ] Запустить `go test ./pkg/dl` и убедиться в FAIL.
- [ ] Реализовать построение кандидатов и канонический `CacheID`; не менять публичный `Download(ctx, opts) error`.
- [ ] Очищать неполный кэш после неудачной новой загрузки; для ошибки cached Git update выполнить чистую попытку со следующим кандидатом.
- [ ] Запустить `go test ./pkg/dl`; ожидается PASS.
- [ ] Закоммитить загрузку и тесты.

### Task 3: Связанные источники в сборке

**Files:**
- Modify: `internal/build/build.go:211-223,422-442,503-513`
- Modify: `internal/build/source_downloader.go`
- Modify: `internal/build/source_downloader_test.go`
- Modify: `internal/build/utils.go:550-568`
- Modify: `internal/build/utils_internal_test.go`

**Interfaces:**
- Consumes: resolved `SourceMirrors`, `Options.CacheID`, `Options.FallbackURLs`.
- Produces: `Source{URL string, Mirrors []string, Checksum string}` и `SourcesInput{Sources []Source}`.

- [ ] Написать failing tests на объединение зеркал дубликатов, приоритет не-`SKIP`, ошибку разных не-`SKIP` checksum, пустые зеркала и передачу канонического URL в `dl`.
- [ ] Запустить `go test ./internal/build` и убедиться в FAIL.
- [ ] Собирать `[]Source` для каждого подпакета, валидировать длины и заменить параллельные срезы в `DownloadSources`.
- [ ] Запустить `go test ./internal/build`; ожидается PASS.
- [ ] Закоммитить интеграцию сборки.

### Task 4: Resolved JSON, БД и семантический кэш

**Files:**
- Modify: `generators/alrsh-package/main.go`
- Modify: `pkg/alrsh/package_gen.go` через `go generate ./pkg/alrsh`
- Modify: `pkg/alrsh/view.go` при необходимости
- Modify: `pkg/alrsh/source_mirrors_test.go`
- Modify: `internal/build/cachekey_test.go`

**Interfaces:**
- Consumes: `Package.SourceMirrors`.
- Produces: resolved JSON `source_mirrors`, сохранение overrides и разрешение каждого набора.

- [ ] Написать tests на JSON round-trip, старый JSON без поля, resolved overrides и неизменность `SemanticCacheKey` при изменении только зеркал.
- [ ] Запустить `go test ./pkg/alrsh ./internal/build ./internal/db` и убедиться в FAIL новых assertions.
- [ ] Расширить генератор для `[]OverridableField[[]string]`, выполнить `go generate ./pkg/alrsh` и стандартное обновление license headers.
- [ ] Запустить те же тесты; ожидается PASS.
- [ ] Закоммитить сериализацию и генератор.

### Task 5: Сквозная проверка

**Files:**
- Modify: существующая документация синтаксиса `alr.sh`, если она находится в репозитории; новых отдельных руководств не создавать.

- [ ] Добавить e2e-сценарий в существующем стиле: локальный HTTP mirror отдаёт архив, основной URL недоступен, сборка успешна; второй сценарий проверяет fallback на upstream.
- [ ] Запустить focused unit/e2e tests и убедиться в PASS.
- [ ] Запустить `go test ./...`, `go vet ./...` и стандартную проверку форматирования проекта.
- [ ] Проверить `git status --short`, удалить только созданные проверками ненужные артефакты и просмотреть итоговый diff.
- [ ] Закоммитить финальные исправления; ветка `feature/source-mirrors` готова к review.
