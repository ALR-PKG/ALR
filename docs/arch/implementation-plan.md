# План реализации: каталог сборки, мультиюзер, привилегии

> **Для исполнителей:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development
> (рекомендуется) или superpowers:executing-plans. Шаги с чекбоксами (`- [ ]`).
> Спецификация: `docs/arch/build-dir-multiuser.md` (целевая модель — разделы 9-11).

**Цель:** убрать сборку из tmpfs (`/tmp/alr`) в per-user кэш на диске, ввести
безопасную мультиюзер-модель и модель привилегий «sudo только для установки».

**Архитектура:** per-user деревья сборки и кэш пакетов (`os.UserCacheDir()`),
общие read-mostly данные в `/var/cache/alr` (репо, БД, dlcache), доверие кэша
по владельцу файла, семантический ключ инвалидации, демон собирает
setuid-форком в вызывающего пользователя, встроенная песочница namespaces.

**Стек:** Go 1.2x, koanf (конфиг), mvdan/sh (скрипты), x/sys/unix (Statfs,
namespaces), SQLite (modernc), D-Bus (godbus), polkit.

**Этапы.** Детально расписаны Этапы 1-2 (закрывают исходную проблему tmpfs).
Этапы 3-10 — на уровне состава работ; каждый получает такой же детальный
TDD-план перед стартом (отдельным документом или дополнением этого).

| Этап | Содержание | Статус |
|------|-----------|--------|
| 1 | Перенос сборки и dlcache с `/tmp` на диск, per-user build dir | детальный план ниже |
| 2 | Проверка свободного места перед сборкой | детальный план ниже |
| 3 | flock на сборку и pull, busy-timeout SQLite | эскиз |
| 4 | Разделение build dir и кэша пакетов, проверка владельца в `CheckForBuiltPackage` | эскиз |
| 5 | Семантический ключ кэша (спек §11) | эскиз |
| 6 | Группа `alr`, dlcache без sudo-спама | эскиз |
| 7 | `alr build` без привилегий; проверка wheel перед фазой установки | эскиз |
| 8 | D-Bus: polkit (subject вызывающего), setuid-сборка | эскиз |
| 9 | Песочница namespaces → landlock → none+warning | эскиз |
| 10 | `alr fix` чистит только своё | эскиз |

---

## Этап 1: per-user build dir, dlcache в /var/cache/alr/dl

Результат: ни один байт сборки не попадает в tmpfs. Дерево сборки и готовые
пакеты — `<userCacheDir>/alr/build` (XDG, обычно `~/.cache/alr/build`),
скачивания — `/var/cache/alr/dl`. Константа `TempDir` удалена.

### Task 1.1: per-user PkgsDir в config.Load

**Файлы:**
- Modify: `internal/config/config.go:119-124`
- Test: `internal/config/paths_test.go` (новый)

- [ ] **Шаг 1: падающий тест**

```go
package config_test

import (
	"path/filepath"
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/config"
)

func TestLoadSetsPerUserBuildDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)

	cfg := config.New()
	if err := cfg.Load(); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}

	want := filepath.Join(xdg, "alr", "build")
	if cfg.GetPaths().PkgsDir != want {
		t.Errorf("Expected PkgsDir %q, got %q", want, cfg.GetPaths().PkgsDir)
	}
}

func TestLoadKeepsSystemPaths(t *testing.T) {
	cfg := config.New()
	if err := cfg.Load(); err != nil {
		t.Fatalf("Expected no error, got %s", err)
	}
	if cfg.GetPaths().CacheDir != "/var/cache/alr" {
		t.Errorf("Expected CacheDir /var/cache/alr, got %q", cfg.GetPaths().CacheDir)
	}
	if cfg.GetPaths().RepoDir != "/var/cache/alr/repo" {
		t.Errorf("Expected RepoDir /var/cache/alr/repo, got %q", cfg.GetPaths().RepoDir)
	}
}
```

- [ ] **Шаг 2: убедиться, что тест падает**

Run: `go test ./internal/config/ -run TestLoadSetsPerUserBuildDir -v`
Expected: FAIL (`/tmp/alr/pkgs` вместо XDG-пути)

- [ ] **Шаг 3: реализация**

В `internal/config/config.go` заменить строку 123:

```go
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		// Фолбэк для сред без HOME/XDG (сервисные аккаунты):
		// системный каталог, права обеспечит fsutils
		userCacheDir = constants.SystemCachePath
		c.paths.PkgsDir = filepath.Join(userCacheDir, "build")
	} else {
		c.paths.PkgsDir = filepath.Join(userCacheDir, "alr", "build")
	}
```

(строку с `constants.TempDir` удалить)

- [ ] **Шаг 4: тест проходит**

Run: `go test ./internal/config/ -v`
Expected: PASS (оба теста)

- [ ] **Шаг 5: коммит**

```bash
git add internal/config/config.go internal/config/paths_test.go
git commit -m "Изменено: каталог сборки перенесён в per-user XDG-кэш"
```

### Task 1.2: dlcache в /var/cache/alr/dl

**Файлы:**
- Modify: `internal/build/source_downloader.go:78-80`
- Test: `internal/build/source_downloader_test.go` (новый, если удобно) —
  либо покрывается существующими тестами сборки; минимум — compile check

- [ ] **Шаг 1: реализация (dlcache.New сам добавляет подкаталог `dl`)**

```go
	// Используем системный кэш для загрузок
	// dlcache.New добавит свой подкаталог "dl" внутри
	opts.DlCache = dlcache.New(s.cfg.GetPaths().CacheDir)
```

Удалить импорт `internal/constants` из файла (больше не используется).

- [ ] **Шаг 2: сборка и тесты пакета**

Run: `go build ./... && go test ./internal/build/ ./pkg/dlcache/`
Expected: OK

- [ ] **Шаг 3: коммит**

```bash
git add internal/build/source_downloader.go
git commit -m "Изменено: кэш скачиваний перенесён в /var/cache/alr/dl"
```

### Task 1.3: удаление константы TempDir и триггера /tmp/alr

**Файлы:**
- Modify: `internal/constants/constants.go:23`
- Modify: `internal/fsutils/dirs.go:26-30`

- [ ] **Шаг 1: удалить `TempDir = "/tmp/alr"` из constants.go**

- [ ] **Шаг 2: в `internal/fsutils/dirs.go` убрать префикс `/tmp/alr`**

```go
// EnsureTempDirWithRootOwner создает каталог в /var/cache/alr с правами для привилегированной группы
// Все каталоги в /var/cache/alr принадлежат root:привилегированная_группа с правами 2775
// Для других каталогов использует стандартные права
func EnsureTempDirWithRootOwner(path string, mode os.FileMode) error {
	needsElevation := strings.HasPrefix(path, "/var/cache/alr")
```

Также обновить комментарий-дубль в `internal/utils/utils.go:33-35`.

- [ ] **Шаг 3: проверить, что TempDir нигде не остался**

Run: `grep -rn "TempDir\b" --include="*.go" internal/ cmd/ pkg/ | grep -v "_test\|os.TempDir\|t.TempDir"`
Expected: пусто (остаются только `os.TempDir()`/`t.TempDir()`)

- [ ] **Шаг 4: сборка**

Run: `go build ./...`
Expected: ошибки компиляции в `cmd/fix.go` (использует литералы /tmp/alr —
чинится в Task 1.4) — порядок: сначала Task 1.4, потом общая сборка

### Task 1.4: alr fix без /tmp/alr

**Файлы:**
- Modify: `cmd/fix.go:100-146`

- [ ] **Шаг 1: заменить блок очистки /tmp/alr на per-user build dir**

Вместо строк 100-146 (очистка и пересоздание `/tmp/alr`, `dl`, `pkgs`,
chown/chmod через sudo):

```go
			slog.Info(gotext.Get("Clearing build directory"))
			buildDir := paths.PkgsDir
			if _, err := os.Stat(buildDir); err == nil {
				if err := os.RemoveAll(buildDir); err != nil {
					return fmt.Errorf("%s: %w", gotext.Get("Unable to remove build directory"), err)
				}
			}
			if err := os.MkdirAll(buildDir, 0o755); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("Unable to create build directory"), err)
			}
```

(каталог `dl` теперь внутри `paths.CacheDir` и очищается существующим
циклом выше; chown/chmod для per-user каталога не нужны)

- [ ] **Шаг 2: новые строки в переводы**

Run: проверить Makefile на цель i18n (`grep -n "i18n\|gotext" Makefile scripts/i18n-precommit.sh`),
обновить `internal/translations` существующим инструментарием проекта,
добавить ru/en переводы новых строк.

- [ ] **Шаг 3: сборка и тесты**

Run: `go build ./... && go test ./cmd/ ./internal/...`
Expected: OK

- [ ] **Шаг 4: коммит**

```bash
git add cmd/fix.go internal/constants/constants.go internal/fsutils/dirs.go internal/utils/utils.go internal/translations/
git commit -m "Изменено: alr fix очищает per-user каталог сборки, убран /tmp/alr"
```

### Task 1.5: сквозная проверка Этапа 1

- [ ] **Шаг 1:** `go build ./... && go test ./... ` — зелёное
- [ ] **Шаг 2:** ручная проверка: `alr build -s <тестовый рецепт>` → дерево
  появляется в `~/.cache/alr/build/<pkg>`, в `/tmp/alr` ничего нет,
  скачивания в `/var/cache/alr/dl`
- [ ] **Шаг 3:** `df /tmp` до и после сборки — tmpfs не растёт
- [ ] **Шаг 4:** e2e (локально, если возможно): `go test ./e2e-tests/ -run TestBuild -v`

---

## Этап 2: проверка свободного места перед сборкой

### Task 2.1: fsutils.CheckFreeSpace

**Файлы:**
- Create: `internal/fsutils/freespace.go`
- Test: `internal/fsutils/freespace_test.go`

- [ ] **Шаг 1: падающий тест**

```go
package fsutils_test

import (
	"testing"

	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
)

func TestCheckFreeSpaceFails(t *testing.T) {
	dir := t.TempDir()
	// Требуем заведомо больше, чем есть на любой ФС
	err := fsutils.CheckFreeSpace(dir, 1<<62)
	if err == nil {
		t.Error("Expected error for impossible requirement, got nil")
	}
}

func TestCheckFreeSpaceOK(t *testing.T) {
	dir := t.TempDir()
	if err := fsutils.CheckFreeSpace(dir, 1); err != nil {
		t.Errorf("Expected no error, got %s", err)
	}
}
```

- [ ] **Шаг 2: убедиться в падении**

Run: `go test ./internal/fsutils/ -run TestCheckFreeSpace -v`
Expected: FAIL (функция не определена)

- [ ] **Шаг 3: реализация**

```go
package fsutils

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// CheckFreeSpace проверяет, что на ФС каталога dir свободно не менее
// required байт (для непривилегированного пользователя)
func CheckFreeSpace(dir string, required uint64) error {
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return fmt.Errorf("не удалось проверить свободное место на %s: %w", dir, err)
	}
	free := st.Bavail * uint64(st.Bsize)
	if free < required {
		return fmt.Errorf("недостаточно места на %s: свободно %d МБ, требуется %d МБ",
			dir, free/(1<<20), required/(1<<20))
	}
	return nil
}
```

(если `golang.org/x/sys` отсутствует в go.mod — `go get golang.org/x/sys@latest`;
проверить: `grep "x/sys" go.mod`)

- [ ] **Шаг 4: тесты проходят**

Run: `go test ./internal/fsutils/ -v`
Expected: PASS

- [ ] **Шаг 5: коммит** `Добавлено: проверка свободного места (fsutils.CheckFreeSpace)`

### Task 2.2: вызов проверки перед сборкой

**Файлы:**
- Modify: `internal/build/utils.go:51-77` (`prepareDirs`)

- [ ] **Шаг 1: в `prepareDirs` после создания `dirs.BaseDir` вызвать
  `fsutils.CheckFreeSpace(dirs.BaseDir, minBuildSpace)`**

```go
// минимум свободного места для сборки — 2 ГиБ; тяжёлым пакетам (Rust/Tauri)
// может не хватить, но катастрофического ENOSPC посреди сборки не будет
const minBuildSpace = 2 << 30
```

- [ ] **Шаг 2: тест** — сборка любого пакета с существующими тестами;
unit: `prepareDirs` на tmp-ФС с огромным лимитом сложно — ограничиться
unit-тестом CheckFreeSpace (Task 2.1) и ручной проверкой

Run: `go build ./... && go test ./internal/build/ -v`
Expected: OK

- [ ] **Шаг 3: коммит** `Добавлено: проверка свободного места перед сборкой`

---

## Этапы 3-10: состав работ (детальный план — перед стартом каждого)

### Этап 3: блокировки
- flock на `<BaseDir>.lock` вокруг сборки пакета (`internal/build/utils.go`,
  `x/sys/unix.Flock`), non-blocking с понятной ошибкой «пакет уже собирается»
- flock на pull репозитория (`internal/repos/pull.go`)
- SQLite: `?_busy_timeout=5000` в DSN (`internal/db/db.go:70`)
- Тесты: два параллельных процесса на один пакет

### Этап 4: разделение build dir и кэша пакетов
- `Paths.BuildDir` (дерево) отдельно от `Paths.PkgsDir` (готовые пакеты,
  `~/.cache/alr/pkgs`); готовый пакет копируется из дерева в кэш
- `CheckForBuiltPackage`: поиск в `PkgsDir` + проверка владельца файла
  (`syscall.Stat_t.Uid`: свой или 0/65534), чужой — пересборка
- `internal/build/dirs.go`, `cache.go`, `script_executor.go` (вывод пакета)

### Этап 5: семантический ключ кэша
- Каноническая форма сборочно-значимых полей из `alrsh.Package`
  (версия, sources, checksums, зависимости, тексты функций) → sha256
- Файл `<pkg>.meta` рядом с кэшированным пакетом; несовпадение — пересборка;
  битые метаданные — пересборка
- Тесты: правка description → кэш валиден; правка build() → инвалидация

### Этап 6: группа alr для dlcache
- Установщик/пакеты создают группу `alr`; `/var/cache/alr/dl` — `root:alr 2775`
- `fsutils.EnsureTempDirWithRootOwner` перестаёт дёргать sudo для членов
  группы (проверка `user.GroupIds`)
- Убрать sudo-спам из `pkg/dlcache/dlcache_prod.go`
- **Из финального ревью Этапов 1-2 (Minor 2):** добавить
  `CheckFreeSpace(paths.CacheDir, …)` перед `DownloadSources` — сейчас
  проверяется только ФС дерева сборки, а `/var/cache/alr/dl` может быть
  другой ФС (ENOSPC посреди скачивания).

### Этап 7: модель привилегий CLI
- Убрать `CheckUserPrivileges` из `cmd/build.go:49`
- В `cmd/install.go` проверка переносится ближе к фазе установки (после
  сборки), текст ошибки: «установка требует членства в wheel/sudo»
- e2e: `alr build` от непривилегированного пользователя работает

### Этап 8: D-Bus polkit + setuid-сборка
- Подключить `internal/dbus/policykit.go`: subject из D-Bus credentials
  вызывающего (`GetConnectionCredentials`), CheckAuthorization по action
- Сборка в демоне: форк с setuid в uid вызывающего, сборка в его XDG-кэш;
  фолбэк `nobody` + `/var/cache/alr/build`
- Запрет интерактивных промптов скриптов в демон-режиме (нет TTY)
- Тесты: mock polkit, неавторизованный вызов Install → отказ

### Этап 9: встроенная песочница
- `internal/sandbox/`: re-exec с `CLONE_NEWUSER|CLONE_NEWNS|CLONE_NEWPID`,
  uid-map, bind дерева сборки rw + `/usr`,`/lib`,`/etc` ro, pivot_root
- Фолбэк landlock (ядро 5.13+), затем none + warning
- Интеграция в `script_executor` (подпроцесс-плагин стартует в песочнице)
- Тесты: скрипт, пишущий в `$HOME` → ошибка; пишущий в дерево сборки → OK

### Этап 10: alr fix без чужих данных
- fix чистит per-user каталоги текущего пользователя; системный кэш — только
  при наличии прав (root), без sudo-удаления чужих per-user деревьев
- **Найдено при проверке Этапа 1 (Task 1.5):** `alr fix` без root сначала
  вытирает содержимое `/var/cache/alr` (записи, принадлежащие пользователю),
  затем падает на `EnsureTempDirWithRootOwner` (`cmd/fix.go:113`), не
  пересобрав БД — частичный деструктивный отказ. Нужна проверка доступности
  root-операций ДО удаления (fail-fast) или атомарный порядок:
  сначала гарантировать возможность пересоздания, потом удалять.
- **Из финального ревью Этапов 1-2 (Minor 3):** в фолбэк-режиме без HOME
  (`PkgsDir = /var/cache/alr/build`) fix создаёт системный build-каталог
  от пользователя с 0755 вместо `root:group 2775` — либо пропускать
  создание, когда build dir внутри `CacheDir`, либо использовать
  `EnsureTempDirWithRootOwner`.
- **Из финального ревью (Minor 1):** решить конвенцию по gotext для
  ошибок внутри пакетов — `fsutils.CheckFreeSpace` сейчас возвращает
  русские строки мимо gotext (прецедент в fsutils уже был; либо принять
  как конвенцию пакета, либо поднимать формирование сообщения в cmd-слой).

---

## Замечания для всех этапов

- Коммиты в стиле проекта: `Добавлено: …` / `Изменено: …` / `Исправлено: …`
- Новые строки UI — через `gotext.Get` + обновление `internal/translations`
- `CI=true`-режим не должен сломаться: e2e прогон после каждого этапа
- Каждый этап — отдельный PR в master, порядок строго последовательный
  (этапы 3+ можно параллелить только после Этапа 1)
