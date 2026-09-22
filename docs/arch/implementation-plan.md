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

## Этап 3: блокировки

Результат: два параллельных процесса alr не могут одновременно собирать один
пакет или обновлять один репозиторий — второй получает понятную ошибку.
SQLite ждёт освобождения БД до 5 секунд вместо мгновенного `database is locked`.

### Task 3.1: fsutils.TryLock + блокировка сборки пакета

**Файлы:**
- Create: `internal/fsutils/lock.go`
- Test: `internal/fsutils/lock_test.go`
- Modify: `internal/build/build.go` (`BuildPackage`, после ExecuteFirstPass)

- [x] **Шаг 1: падающий тест** `internal/fsutils/lock_test.go`

  - `TryLock` создаёт файл и возвращает его;
  - повторный `TryLock` на тот же путь → `errors.Is(err, fsutils.ErrLocked)`;
  - после `Close` первого файла повторный `TryLock` успешен;
  - `TryLock` создаёт родительский каталог, если его нет.

Run: `go test ./internal/fsutils/ -run TestTryLock -v`
Expected: FAIL (нет fsutils.TryLock)

- [x] **Шаг 2: реализация `internal/fsutils/lock.go`**

```go
package fsutils

var ErrLocked = errors.New("resource is locked by another process")

// TryLock создаёт (при необходимости) файл path и берёт exclusive
// non-blocking flock(2). Возвращённый файл снимает блокировку при Close.
// Если блокировку уже держит другой процесс — ErrLocked.
func TryLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o2775); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o664)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return f, nil
}
```

(`golang.org/x/sys/unix` уже в go.mod)

- [x] **Шаг 3: блокировка в `BuildPackage`** — сразу после `ExecuteFirstPass`
  (basePkg уже известен), ДО проверки кэша `CheckForBuiltPackage`:

```go
lock, err := fsutils.TryLock(getBaseDir(b.cfg, basePkg) + ".lock")
if errors.Is(err, fsutils.ErrLocked) {
	return nil, errors.New(gotext.Get("Package is already being built by another process"))
}
if err != nil {
	return nil, err
}
defer lock.Close()
```

  Лок-файл лежит рядом с деревом сборки (`~/.cache/alr/build/<pkg>.lock`),
  поэтому переживает очистку SrcDir/PkgDir в `prepareDirs` и виден всем
  процессам пользователя. Блокировка держится до конца `BuildPackage` (defer).
  Новую строку добавить в `internal/translations` (en + ru).

- [x] **Шаг 4: тесты проходят**

Run: `go test ./internal/fsutils/ -v && go build ./...`
Expected: PASS

- [x] **Шаг 5: коммит** `Добавлено: блокировка сборки пакета (flock)`

### Task 3.2: блокировка pull репозитория

**Файлы:**
- Modify: `internal/repos/pull.go` (`pullRepo`)
- Test: `internal/repos/pull_test.go` (существующий) или `pull_internal_test.go`

- [x] **Шаг 1: падающий тест** — создать `Repos` с cfg на temp-dir, в тесте
  заранее взять `fsutils.TryLock(<RepoDir>/<name>.lock)`, вызвать
  `rs.pullRepo(ctx, &types.Repo{Name: name, URL: "https://example.invalid/x.git"}, false)` —
  ожидается ошибка с `errors.Is(err, fsutils.ErrLocked)` до любого сетевого
  обращения.

Run: `go test ./internal/repos/ -run TestPullLocked -v`
Expected: FAIL

- [x] **Шаг 2: в `pullRepo` первым действием** (до цикла по URL):

```go
repoDir := filepath.Join(rs.cfg.GetPaths().RepoDir, repo.Name)
lock, err := fsutils.TryLock(repoDir + ".lock")
if err != nil {
	if errors.Is(err, fsutils.ErrLocked) {
		return fmt.Errorf(gotext.Get("Repository %s is already being pulled by another process: %w"), repo.Name, err)
	}
	return err
}
defer lock.Close()
```

  Блокировка per-repo: параллельные pull разных репозиториев не мешают друг
  другу; конкуренцию за SQLite закрывает busy_timeout (Task 3.3).
  `Pull` и `PullOneAndUpdateFromConfig` проходят через `pullRepo` — оба
  покрываются автоматически. Строку — в переводы (en + ru).

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/repos/ -v`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: блокировка обновления репозитория (flock)`

### Task 3.3: SQLite busy_timeout

**Файлы:**
- Modify: `internal/db/db.go:70` (`Connect`)
- Test: `internal/db/db_test.go` (существующий)

- [x] **Шаг 1: падающий тест** — открыть БД через `Database` на temp-dir,
  выполнить запрос `PRAGMA busy_timeout` через engine, ожидать значение 5000.

Run: `go test ./internal/db/ -run TestBusyTimeout -v`
Expected: FAIL

- [x] **Шаг 2: DSN через file-URI с параметром modernc.org/sqlite:**

```go
dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", d.config.GetPaths().DBPath)
```

  (драйвер — `modernc.org/sqlite`, см. import в db.go; `_pragma` —
  документированный параметр его DSN)

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/db/ -v`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: busy_timeout 5с для SQLite`

### Task 3.4: сквозная проверка Этапа 3

- [x] Два параллельных `alr build -s <один и тот же рецепт>`: второй сразу падает
  с «Package is already being built by another process»
- [x] Два параллельных `alr pull`: второй падает с ошибкой про занятый репозиторий
- [x] `go build ./...`, `go test ./...`, pre-commit

---

## Этап 4: разделение build dir и кэша пакетов

Результат: дерево сборки (`Paths.BuildDir`, `~/.cache/alr/build/<pkg>/{src,pkg}`)
отделено от кэша готовых пакетов (`Paths.PkgsDir`, `~/.cache/alr/pkgs`, плоский).
Готовый пакет после сборки копируется из дерева в кэш. Попадание в кэш
определяется поиском в `PkgsDir` + проверкой владельца файла: доверен
свой uid, root (0) и nobody (65534); чужой владелец — пересборка.
Миграция: старые пакеты внутри деревьев сборки просто не находятся
(разовая пересборка); `alr fix` чистит оба каталога.

### Task 4.1: Paths.BuildDir в config

**Файлы:**
- Modify: `internal/config/paths.go` (поле `BuildDir`)
- Modify: `internal/config/config.go:123-132` (`Load`)
- Test: `internal/config/paths_test.go`

- [x] **Шаг 1: падающий тест** — в `paths_test.go` обновить ожидания:
  успешный `os.UserCacheDir` → `BuildDir = <ucd>/alr/build`,
  `PkgsDir = <ucd>/alr/pkgs`; фолбэк без HOME → `BuildDir = /var/cache/alr/build`,
  `PkgsDir = /var/cache/alr/pkgs`.

Run: `go test ./internal/config/ -v`
Expected: FAIL

- [x] **Шаг 2: реализация** — добавить поле `BuildDir string` в `Paths`
  (paths.go) и заполнить оба пути в `config.go` по правилам выше.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/config/ -v && go build ./...`
Expected: PASS (пока BuildDir никем не читается — только config)

- [x] **Шаг 4: коммит** `Добавлено: пути BuildDir и PkgsDir (кэш пакетов) в конфиг`

### Task 4.2: дерево сборки на BuildDir

**Файлы:**
- Modify: `internal/build/dirs.go` (`getDirsForPackage`, `getBaseDir`, `getSrcDir`)
- Modify: `cmd/fix.go:100-109` (чистка обоих каталогов)
- Test: `internal/build/dirs_test.go` (если есть) или смежные тесты build

- [x] **Шаг 1: падающий тест** — `getDirs`/`getBaseDir`/`getSrcDir` строятся
  от `BuildDir`, а не `PkgsDir` (тест с мок-конфигом: BuildDir=/tmp/b,
  PkgsDir=/tmp/p → BaseDir=/tmp/b/<pkg>).

- [x] **Шаг 2: реализация** — `getBaseDir`/`getSrcDir`/`getDirsForPackage`
  используют `cfg.GetPaths().BuildDir`. Лок-файл сборки
  (`build.go:337`, `<BaseDir>.lock`) автоматически переезжает на BuildDir —
  это желаемо (лок дерева сборки). В `cmd/fix.go` блок «Clearing build
  directory» чистит `paths.BuildDir` И `paths.PkgsDir` (оба RemoveAll +
  MkdirAll 0755, как сейчас).

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ ./internal/config/ -v && go build ./...`
Expected: PASS. ВНИМАНИЕ: после этого шага готовые пакеты всё ещё пишутся
в дерево (BaseDir), а CheckForBuiltPackage ищет там же — поведение
временно эквивалентно текущему, разрыв появится в Task 4.3/4.4.

- [x] **Шаг 4: коммит** `Изменено: дерево сборки переведено на Paths.BuildDir`

### Task 4.3: копирование готового пакета в кэш

**Файлы:**
- Modify: `internal/build/script_executor.go:194-231` (вывод nfpm-пакета)

- [x] **Шаг 1: падающий тест** — если есть тестовая инфраструктура для
  ExecuteSecondPass — тест, что после упаковки файл лежит в `PkgsDir`.
  Если инфраструктуры нет — вынести копирование в хелпер
  `copyToCache(cfg, srcPath, filename) (dstPath string, err error)` и
  покрыть его юнит-тестом (temp-dir: файл копируется, режим 0644,
  существующий чужой файл заменяется через Remove+Create).

- [x] **Шаг 2: реализация** — после `packager.Package` и проверки
  существования файла: обеспечить `PkgsDir` (per-user — `os.MkdirAll(dir,
  0o755)`; общий фолбэк под `/var/cache` — `utils.EnsureTempDirWithRootOwner`
  как в prepareDirs, выбирать по тому же признаку, что config: если
  PkgsDir внутри `constants.SystemCachePath`), скопировать пакет в
  `filepath.Join(PkgsDir, pkgName)` (если dst существует — сначала
  `os.Remove`: каталог group-writable, замена чужого файла допустима),
  после успешного копирования удалить файл из дерева (он больше не
  артефакт кэша). `BuiltDep.Path` = путь в кэше.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -v && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: копирование готового пакета в кэш PkgsDir`

### Task 4.4: CheckForBuiltPackage — кэш + доверие по владельцу

**Файлы:**
- Modify: `internal/build/cache.go`
- Test: `internal/build/cache_test.go` (новый или существующий)

- [x] **Шаг 1: падающий тест** — файл пакета в `PkgsDir` (temp-dir)
  от текущего пользователя → найден; отсутствует → не найден; юнит на
  хелпер `isTrustedOwner(uid uint32) bool`: доверенные — `os.Geteuid()`,
  0, 65534; остальные — нет.

- [x] **Шаг 2: реализация** — `CheckForBuiltPackage` ищет
  `filepath.Join(c.cfg.GetPaths().PkgsDir, filename)` (плоско, без
  baseName-подкаталога — conventional filename уже уникален: имя+репо,
  версия, релиз, арх, формат). При находке: `os.Stat` →
  `stat.Sys().(*syscall.Stat_t).Uid`; владелец не из доверенных
  (`os.Geteuid()`, 0, 65534) → `slog.Debug` о недоверии и `false`
  (пересборка; файл будет заменён на шаге копирования).

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -v && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Изменено: кэш пакетов в PkgsDir с проверкой владельца`

### Task 4.5: сквозная проверка Этапа 4

- [x] Сборка тестового пакета: файл появляется в `~/.cache/alr/pkgs`, в дереве
  не остаётся; повторная сборка → «Using cached package»
- [x] `alr fix` чистит оба каталога (проверить от пользователя)
- [x] Параллельная сборка того же пакета → ErrLocked (Этап 3 не сломан)
- [x] `go build ./...`, `go test ./...`, pre-commit
- **Найдено при проверке (Task 4.5), ИСПРАВЛЕНО:** «Using cached package»
  через CLI `alr build` был недостижим — `cmd/build.go` в конце перемещал
  готовый пакет из `PkgsDir` в рабочий каталог, осушая кэш после каждой
  сборки. Исправлено: `osutils.Move` заменён на `osutils.Copy`
  (`cmd/build.go:177`), пакет остаётся в кэше, копия — в cwd; кэш-хит
  через CLI подтверждён повторной сквозной проверкой. Осталось в силе:
  `-c/--clean` обходит проверку кэша (`internal/build/build.go:350`),
  кэш-хит проверяется без `-c`.
- **Подтверждено повторно (Task 4.5):** частичный деструктивный отказ
  `alr fix` без root (уже зафиксирован в Этапе 10): содержимое
  `/var/cache/alr` вытерто, затем падение на `sudo chmod` в
  `EnsureTempDirWithRootOwner` без пересборки БД.

---

## Этап 5: семантический ключ кэша

Результат: попадание в кэш определяется не только именем файла
(имя+репо, версия, релиз, арх, формат — уже в conventional filename), но и
семантическим ключом: sha256 от сборочно-значимых полей `alrsh.Package` +
канонических текстов функций `prepare`/`build`/`package`. Правка
description/summary/группы/лицензий или форматирования кэш НЕ
инвалидирует; правка sources/checksums/зависимостей/тел функций —
инвалидирует. Ключ хранится в `<PkgsDir>/<filename>.meta` (JSON),
записывается атомарно (temp+rename) вместе с пакетом; мета
отсутствует/битая/не совпала — пересборка.

Известные ограничения (задокументировать в плане):
- порядок элементов в списках значим (перестановка deps инвалидирует —
  безопасная избыточная инвалидация);
- локальные файлы-источники с checksum SKIP: изменение содержимого без
  смены списка sources кэш не инвалидирует;
- содержимое файлов install-скриптов (preinstall и т.д. из scriptdir)
  хешируется только по именам.

### Task 5.1: хелперы ключа (ScriptFuncsKey, SemanticCacheKey)

**Файлы:**
- Create: `internal/build/cachekey.go`
- Test: `internal/build/cachekey_test.go`

- [x] **Шаг 1: падающий тест**
  - `ScriptFuncsKey`: стабилен для одного скрипта; меняется при правке тела
    `build()`; НЕ меняется при правке комментария/пробелов вне функций и при
    правке тела `version()` (version влияет через поле Version);
  - `SemanticCacheKey`: меняется при правке Version/Release/Epoch/Sources/
    Checksums/Depends/BuildDepends/Scripts/Backup/FireJailed/ExtraFiles;
    НЕ меняется при правке Summary/Description/Group/Homepage/Maintainer/
    Licenses.

- [x] **Шаг 2: реализация**

  - `ScriptFuncsKey(sf *alrsh.ScriptFile) string` — `syntax.Walk` по AST
    (`sf.File()`), для `*syntax.FuncDecl` с именами prepare/build/package
    каноническая печать через `syntax.NewPrinter()` (комментарии не
    сохраняются — нормализация форматирования бесплатно), всё в sha256.
  - `SemanticCacheKey(vars *alrsh.Package, funcsKey string) string` —
    sha256 от `json.Marshal` структуры со сборочно-значимыми полями
    (resolved-значения OverridableField) + funcsKey. Поля:
    Version, Release, Epoch, Architectures, Provides, Conflicts, Replaces,
    Depends/BuildDepends/OptDepends (Resolved), Sources/Checksums (Resolved),
    Backup (Resolved), Scripts (Resolved), AutoReq/AutoProv/skiplists,
    FireJailed/FireJailProfiles, ExtraFiles.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -run 'TestScriptFuncsKey|TestSemanticCacheKey' -v`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: семантический ключ кэша (хелперы)`

### Task 5.2: запись .meta при публикации в кэш

**Файлы:**
- Modify: `internal/build/utils.go` (`copyToCache`)
- Modify: `internal/build/script_executor.go` (`ExecuteSecondPass` — вычисление
  ключа, вызов copyToCache)
- Test: `internal/build/utils_internal_test.go` (расширить TestCopyToCache)

- [x] **Шаг 1: падающий тест** — после copyToCache рядом лежит
  `<filename>.meta` (JSON `{"key": "<hex>"}`); мета пишется атомарно (нет
  `*.tmp-*` остатков); при ошибке — ни пакета, ни меты.

- [x] **Шаг 2: реализация** — `copyToCache` получает параметр `key string`;
  после rename пакета так же атомарно (CreateTemp+Rename) пишет мету.
  `ExecuteSecondPass` (LocalScriptExecutor) вычисляет `funcsKey` один раз из
  `sf`, в цикле подпакетов — `SemanticCacheKey(vars, funcsKey)`. Сигнатуры
  интерфейса ScriptExecutor НЕ меняются (sf и vars уже параметры) — RPC
  (plugins_executors_gen.go) не трогаем, реализация общая.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -v && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: запись .meta кэша при публикации пакета`

### Task 5.3: проверка .meta в CheckForBuiltPackage

**Файлы:**
- Modify: `internal/build/cache.go` (интерфейс build.go:190, реализация Cache)
- Modify: `internal/build/build.go` (вызовы: BuildPackage ~:352,
  checkCacheForAllSubpackages ~:711)
- Test: `internal/build/cache_test.go` (расширить)

- [x] **Шаг 1: падающий тест** — пакет+совпадающая мета → hit; мета
  отсутствует/битая/не совпала → miss (даже при существующем пакете);
  недоверенный владелец меты тоже → miss (мета проверяется как и пакет).

- [x] **Шаг 2: реализация** — `CheckForBuiltPackage(ctx, input, vars, funcsKey
  string)`; внутри после findTrustedCachedPkg: вычислить
  `SemanticCacheKey(vars, funcsKey)`, прочитать и сверить мету.
  Обновить интерфейс `cacheExecutor` (build.go:190) и вызовы:
  - BuildPackage: `funcsKey` из sf (доступен после ExecuteFirstPass);
  - checkCacheForAllSubpackages: скрипт резолвится через
    `b.scriptResolver.ResolveScript(ctx, pkg)` + `b.scriptExecutor.ReadScript`;
    ошибка резолва/чтения → miss с slog.Debug (безопасная сторона), НЕ ошибка.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -v && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: проверка семантического ключа кэша (.meta)`

### Task 5.4: сквозная проверка Этапа 5

- [x] Сборка → кэш-хит; правка description в alr.sh → всё ещё кэш-хит; правка
  тела build() → пересборка; бамп version → новый файл (поведение как раньше)
  - Проверено на Fedora 44/rpm (бинарник из HEAD ветки): второй прогон
    `alr build -s` — «Используется кешированный пакет», рядом с пакетом
    `<pkg>.meta` (`{"v":1,"key":"..."}`); правка `desc` — хит, ключ не
    изменился; правка тела `build()` — miss с debug-строкой
    «Семантический ключ кэша не совпал» (expected/actual), пересборка, мета
    обновлена; бамп `version` — новый conventional filename, старый пакет
    не используется.
- [x] Поток install/dep через checkCacheForAllSubpackages — проверить по логам
  (скрипт резолвится, мета сверяется)
  - Проверено через локальный git-репозиторий (file://, repo `e5test`):
    dep собран по `-s` (ключ из скрипта), затем собран пакет с транзитивной
    зависимостью на него — «Используется кешированный пакет name=e5-dep»
    (hit, НЕ ложный miss). Ключи из скрипта и из БД совпали
    (3e6b35ce… в обоих направлениях: -s→проверка-из-БД и -p→проверка-из--s).
  - Замечание (существующее поведение, не Этап 5): прямые runtime-deps при
    `alr build -s` попадают в targetSet и исключаются из AllALRPackages
    (dependency_tree.go:315-333) — собираются/проверяются только транзитивные.
- [x] `go build ./...`, `go test ./...`, pre-commit

**Находки 5.4:**

1. **(pre-existing, ИСПРАВЛЕНО в этой ветке) arch='all' → вечный cache-miss.**
   `getBasePkgInfo` (internal/build/utils.go) всегда ставил
   `Arch: cpu.Arch()`, а publish-путь `buildPkgMetadata`
   (internal/build/script_executor.go) при `architectures=('all')` ставил
   `Arch = "all"` (nfpm rpm → `noarch`). Проверка кэша искала
   `…-1.x86_64.rpm`, опубликован `…-1.noarch.rpm` — miss до сверки меты.
   Баг старше семантического ключа (в master тот же код).
   Фикс: общий хелпер `resolvePkgArch(vars)` ("all", если architectures
   содержит "all", иначе cpu.Arch()), используется в getBasePkgInfo;
   дублирующий override в buildPkgMetadata удалён. Тесты:
   TestGetBasePkgInfoArch, TestPkgFileNameArchAll (rpm→noarch, deb→all,
   регрессия для обычной архитектуры). Сквозная проверка: рецепт с
   architectures=('all') — публикация `…noarch.rpm`, повторная сборка —
   кэш-хит.
2. **RPC-режим:** downloadUrl скриптов указывает на `git.alr-pkg.ru`, а
   trusted-host — только site.url (`alr-pkg.ru`) → WARN «не относится к
   доверенным хостам» и fallback на локальный git-клон репозитория. Работает,
   но RPC-скачивание alr.sh фактически не используется.
3. dlcache (`/var/cache/alr/dl`) требует sudo-elevation (известно, скоуп
   Этапа 6) — для RPC smoke без sudo использован `CI=true`.

---

## Этап 6: группа alr для dlcache + хвосты ревью

Результат: sudo-спам исчезает для штатной работы. Общие каталоги
(`/var/cache/alr/*`) создаются/чинятся без sudo, если текущий пользователь —
член группы-владельца каталога и права корректны. Целевая группа для новых
установок — `alr` (создаётся установщиком); существующие установки на
wheel/sudo продолжают работать (не перекидываем). Плюс два хвоста из ревью:
CheckFreeSpace для dlcache и рефакторинг реестра локов fsutils.

Модель групп:
- Целевая группа для НОВЫХ каталогов: `alr`, если существует, иначе
  wheel/sudo (как сейчас).
- Fast-path (без sudo): пользователь — член группы, которой принадлежит
  существующий каталог (или ближайший существующий предок с setgid),
  и права корректны (2775) — работаем без эскалации. Чинить/chown через
  sudo — только когда реально нужно (нет каталога, чужая группа, не член).
- Миграции wheel→alr для существующего дерева НЕТ: работающая раскладка
  не трогается; новые подкаталоги под setgid-родителем наследуют его группу.

### Task 6.1: групповые хелперы в fsutils

**Файлы:**
- Modify: `internal/fsutils/privileged_group.go`
- Test: `internal/fsutils/privileged_group_test.go` (новый)

- [x] **Шаг 1: падающий тест**
  - `CacheShareGroup()`: возвращает "alr", если группа существует в системе,
    иначе `GetPrivilegedGroup()` (детерминировать в тесте через подмену
    lookup — вынести `var lookupGroup = user.LookupGroup` пакетного уровня);
  - `CurrentUserGroupGIDs()`: возвращает множество gid текущего пользователя
    (основная + дополнительные группы);
  - `dirSharedWith(gid uint32, ...)`: stat каталога → его группа ∈ gid'ам
    пользователя && setgid && group rwx.

- [x] **Шаг 2: реализация** — `CacheShareGroup()` (без sync.Once — чтобы
  группа alr, появившаяся после старта процесса, подхватывалась;
  `lookupGroup("alr")` → "alr", иначе GetPrivilegedGroup);
  `CurrentUserGroupGIDs()` (`os/user.Current()` + `GroupIds()`);
  хелпер проверки «каталог общий для меня» (Lstat, gid, mode).

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/fsutils/ -v -count=1`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: группа alr и хелперы членства (fsutils)`

### Task 6.2: EnsureTempDirWithRootOwner без sudo для членов группы

**Файлы:**
- Modify: `internal/fsutils/dirs.go`
- Test: `internal/fsutils/dirs_test.go` (новый)

- [x] **Шаг 1: падающий тест** — тестируемость: префикс `/var/cache/alr`
  выносится в пакетную переменную (`var systemCachePrefix = "/var/cache/alr"`,
  в тесте подменяется на temp-dir через t.Cleanup). Кейсы (все на temp-dir,
  без root):
  - каталог существует, его группа ∈ моим, 2775 → nil без эскалации
    (проверить отсутствие вызова sudo — через подмену exec или по факту
    отсутствия ошибок в среде без sudo);
  - каталога нет, предок setgid+group-writable и группа ∈ моим →
    создание MkdirAll+chmod без sudo, наследование группы;
  - чужая группа/не setgid → путь эскалации (в тесте без sudo — ошибка
    эскалации, как сейчас).

- [x] **Шаг 2: реализация** fast-path в EnsureTempDirWithRootOwner до
  sudo-ветки (порядок: prefix? → CI? → root? → fast-path члена группы →
  sudo-ветка как fallback). Целевая группа для chown в sudo/root-ветках —
  `CacheShareGroup()`.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/fsutils/ -v -count=1 && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Изменено: общие каталоги кэша без sudo для членов группы`

### Task 6.3: dlcache без лишнего враппера

**Файлы:**
- Modify: `pkg/dlcache/dlcache_prod.go`

- [x] **Шаг 1: реализация** — `createDir` больше не дублирует проверку
  префикса: просто `return utils.EnsureTempDirWithRootOwner(itemPath, mode)`
  (хелпер сам проверяет префикс и делает MkdirAll для прочих путей).
  Заодно унифицировать источник префикса: fsutils использует тот же
  `systemCachePrefix`, что и Task 6.2 (убрать свой HasPrefix из
  EnsureTempDirWithRootOwner в пользу общего — если префикс живёт в fsutils,
  dlcache_prod просто делегирует).

- [x] **Шаг 2: тесты проходят**

Run: `go build ./... && go test ./pkg/dlcache/ ./internal/fsutils/ -count=1`
Expected: PASS (существующие тесты dlcache — с тегом test, проверь оба варианта)

- [x] **Шаг 3: коммит** `Изменено: dlcache делегирует создание каталогов fsutils`

### Task 6.4: CheckFreeSpace для CacheDir перед скачиванием

**Файлы:**
- Modify: `internal/build/source_downloader.go` (`DownloadSources`, ~:79)

- [x] **Шаг 1: падающий тест** — по возможности юнит: DownloadSources с
  пустым списком sources всё равно проверяет место? (выбери проверяемый
  контракт: проверка выполняется ДО цикла, даже если sources пуст — или
  только при непустом; реши, задокументируй, покрой тестом хелпера).
  Выбран контракт: проверка только при непустом списке sources — без
  скачиваний dlcache не создаёт каталог dl, а CacheDir может не существовать.

- [x] **Шаг 2: реализация** — до цикла скачиваний:
  `fsutils.CheckFreeSpace(s.cfg.GetPaths().CacheDir, minDownloadSpace)`
  (константа 1 ГиБ с комментарием: dlcache — отдельная ФС от дерева сборки;
  точный размер неизвестен до скачивания, это страховка от ENOSPC посреди).

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -v -count=1 && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: проверка свободного места перед скачиванием исходников`

### Task 6.5: реестр локов fsutils — per-path pending, flock вне мьютекса

**Файлы:**
- Modify: `internal/fsutils/lock.go`
- Test: `internal/fsutils/lock_test.go`

- [x] **Шаг 1: падающий тест** — блокирующий `Lock` не должен останавливать
  `TryLock` по ДРУГОМУ пути: горутина держит ожидание `Lock(pathA)` (pathA
  залочен probe-fd), в это время `TryLock(pathB)` возвращается сразу;
  конкурентные `Lock` одного пути из двух горутин → refs==2, одна запись в
  реестре; EINTR-retry сложно триггерить — оставить кодом + комментарием.

- [x] **Шаг 2: реализация** — реестр: map[path]*lockEntry{file, refs,
  pending bool, ch chan}; `Lock`/`TryLock`: под мьютексом — если pending,
  ждать на channel (вне мьютекса, цикл); если есть file — refs++; иначе
  пометить pending, отпустить мьютекс, делать open+flock (с retry на EINTR),
  снова под мьютексом — зарегистрировать/снять pending, разбудить ждущих.
  Close без изменений по семантике (refs--, реальный close при 0).
  НЕ ломать: реентерабельность, normalizeLockAccess, ErrLocked-маппинг
  TryLock.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/fsutils/ -v -race -count=5`
Expected: PASS (стресс-прогон без гонок)

- [x] **Шаг 4: коммит** `Исправлено: блокирующий flock больше не останавливает реестр локов`

### Task 6.6: установщик создаёт группу alr

**Файлы:**
- Modify: `scripts/install.sh`

- [x] **Шаг 1: после installPkg** — `$rootCmd groupadd -f alr`;
  `$rootCmd mkdir -p /var/cache/alr/dl && $rootCmd chown root:alr
  /var/cache/alr /var/cache/alr/dl && $rootCmd chmod 2775 ...`
  (только если каталогов нет или владелец root:root — не ломать wheel-системы:
  chown делать, только если текущая группа не wheel/sudo/alr? — реши и
  задокументируй простое правило: если группа каталога уже wheel/sudo/alr —
  не трогать; иначе выставить root:alr); добавить вызвавшего пользователя
  (`$SUDO_USER`/`$DOAS_USER`, если не root) в группу alr
  (`usermod -aG alr`) с сообщением о необходимости перелогина.
- [x] **Шаг 2: проверка синтаксиса** `bash -n scripts/install.sh`
- [x] **Шаг 3: коммит** `Добавлено: группа alr и общий каталог загрузок в установщик`

### Task 6.7: сквозная проверка Этапа 6

- [x] На реальной машине (пользователь в wheel, /var/cache/alr root:wheel
  2775): сборка со скачиванием источников НЕ вызывает sudo (подмена sudo на
  падающий логгер в PATH); до и после — diff вызовов
  - Проверено 2026-09-20 (пользователь в wheel, /var/cache/alr root:wheel
    2775): фейковый `sudo` в PATH пишет вызовы в лог и падает с exit 1.
    Рецепт с `sources=(http://…)` и sha256. Ветка (c742820): сборка
    успешна, rpm собран, `/var/cache/alr/dl` создан как user:wheel 2775,
    лог sudo ПУСТ. Master (e8c11ea): падает — «не удалось создать
    директорию /var/cache/alr/dl/<hash>: exit status 1», в логе
    `sudo mkdir -p /var/cache/alr/dl/<hash>` — подтверждена проблема,
    которую чинит этап. Перед каждым прогоном очищались dlcache, build- и
    pkgs-кэш — иначе семантический кэш (Этап 5) пропускает сборку молча.
- [x] `alr fix` без root — поведение не ухудшилось (известный отказ Этапа 10
  остаётся как был)
  - ИЗМЕНЕНИЕ ПОВЕДЕНИЯ (улучшение): master `alr fix` без root вытирает
    содержимое `/var/cache/alr` (alr.db, repo, dl) и падает на
    `sudo chmod 2775 /var/cache/alr` — частичный деструктивный отказ, БД не
    пересобрана. Ветка: fast-path EnsureTempDirWithRootOwner (каталог
    root:wheel 2775, пользователь в wheel) проходит без sudo, fix
    завершается полностью (exit 0), БД пересоздана, вызовов sudo нет.
  - После fix site-репозитории не переклонируются: Pull пропускает их
    («обновляется через RPC», internal/repos/pull.go:81 — то же в master,
    не регрессия этапа); данные подтягиваются лениво через RPC
    (`alr info alr-bin` после fix работает). Следствие: локальный
    git-checkout репозиториев после fix не восстанавливается, fallback на
    локальную БД при недоступном RPC пуст до первых RPC-запросов.
    `/var/cache/alr` после проверки восстановлен из снапшота.
- [x] `go build ./...`, `go test ./...` (exit 0),
  `go test ./internal/fsutils/ -race -count=3` (ok),
  `pre-commit run --all-files` (все хуки Passed; хук покрытия обновил
  `assets/coverage-badge.svg`)
- [ ] Хвост: postinstall пакета alr-bin (репозиторий ALR/alr-default на
  сайте) тоже должен создавать группу alr — открытый пункт, отдельный
  репозиторий.

---

## Этап 7: модель привилегий CLI

Результат: `alr build` работает от полностью непривилегированного
пользователя (не root, не wheel/sudo/alr): дерево сборки и кэш пакетов —
per-user, кэш скачиваний — общий для членов группы, а для остальных —
per-user фолбэк без sudo-подсказок. `alr install` проверяет привилегии
перед установкой, а не при старте команды. Остальные команды
(upgrade/remove/repo/refresh) — без изменений (мутируют систему или
общий кэш — там проверка оправдана).

### Task 7.1: убрать CheckUserPrivileges из alr build

**Файлы:**
- Modify: `cmd/build.go:49` (удалить проверку; импорт utils — если стал лишним)

- [x] **Шаг 1: реализация** — удалить вызов; `go build ./...`
- [x] **Шаг 2: ручная проверка** — сборка тестового пакета от текущего
  пользователя (в wheel — регрессии нет); от непривилегированного — Task 7.4
- [x] **Шаг 3: коммит** `Изменено: alr build не требует привилегий`

### Task 7.2: per-user фолбэк кэша скачиваний для пользователей без групп

**Файлы:**
- Modify: `internal/build/source_downloader.go` (выбор корня dlcache)
- Modify: `internal/fsutils/privileged_group.go` (хелпер членства)
- Test: `internal/fsutils/privileged_group_test.go`, `internal/build/source_downloader_test.go`

- [x] **Шаг 1: падающий тест** — `fsutils.UserInGroups(gids, names...)`:
  true, если хотя бы одна именованная группа существует и её gid ∈ gids.
  В source_downloader: пользователь без членства ни в одной кандидатной
  группе → корень dlcache = per-user (`os.UserCacheDir()/alr`, dlcache сам
  добавляет `dl`), без вызова EnsureTempDirWithRootOwner (без sudo).

- [x] **Шаг 2: реализация** — хелпер выбора корня (например
  `resolveDlCacheRoot(cfg) (string, error)`):
  1. root (euid 0) → общий `CacheDir`;
  2. `CurrentUserGroupGIDs()` + `UserInGroups(gids, "alr",
     fsutils.GetPrivilegedGroup())` — член → общий, причём сначала
     `EnsureTempDirWithRootOwner(CacheDir/dl, 0o2775)`; ошибка → фолбэк
     per-user + slog.Warn;
  3. не член → сразу per-user (sudo не дёргаем — у него нет прав);
  4. per-user фолбэк: `os.MkdirAll(<ucd>/alr/dl, 0o755)`, ошибка
     UserCacheDir → вернуть исходную ошибку общего пути.
  Один раз на DownloadSources (до цикла), slog.Debug о выбранном корне.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ ./internal/fsutils/ -v -count=1 && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: per-user фолбэк кэша скачиваний`

### Task 7.3: install — проверка привилегий перед установкой

**Файлы:**
- Modify: `cmd/install.go:43` → перед `builder.InstallPkgs`

- [x] **Шаг 1: реализация** — перенести `utils.CheckUserPrivileges()` из
  начала RunE к точке непосредственно перед вызовом InstallPkgs.
  Существующий текст ошибки («…необходимо состоять в группе %s…»)
  удовлетворяет требованию плана — не менять (единый текст для всех
  команд; смена текста — отдельный вопрос консистентности, не этот этап).
- [x] **Шаг 2: проверка** — `go build ./...`; `alr install` без прав
  падает с понятной ошибкой ДО любых установок в систему (можно проверить
  на заведомо нечлене группы в docker)
- [x] **Шаг 3: коммит** `Изменено: проверка привилегий в install перед фазой установки`

### Task 7.4: сквозная проверка Этапа 7 (docker, непривилегированный пользователь)

- [x] Непривилегированный пользователь (uid 1001, без групп wheel/sudo/alr,
  sudo подменён падающим логгером). Docker/podman на хосте отсутствуют —
  проверка выполнена в bubblewrap-сэндбоксе (user namespace, host root ro,
  подменённые /etc/passwd и /etc/group, rw только /tmp/alr-e7):
  - [x] `alr build -s` рецепта без sources → успех, rpm в cwd, кэш
    скачиваний не используется, под /var/cache/alr ничего не создано
  - [x] `alr build -s` рецепта с sources (`local:///hello.txt`) → успех,
    скачивание в `~/.cache/alr/dl/<hash>/`, /var/cache/alr не тронут
    (контроль по mtime), sudo не вызывался (лог пуст)
  - [x] `alr install` → ошибка «пользователь testuser не имеет необходимых
    привилегий… необходимо… состоять в группе wheel» ДО установки,
    exit 1, без sudo и без panic
- [x] Регрессия: сборка того же рецепта от пользователя wheel на хосте —
  успех, общий `/var/cache/alr/dl` (fast-path, без sudo: fake-sudo в PATH
  не вызывался)
- [x] `go build ./...`, `go test ./...` (без e2e) — PASS; `pre-commit run
  --all-files` — все 5 хуков Passed

**Находки 7.4 (не блокеры):**
- go-plugin (плагин-процессы сборки) фильтрует env (`setCommonCmdEnv` в
  `internal/build/plugins.go`) и пишет временные файлы в `/tmp` — в
  полностью ro-окружении без writable /tmp сборка упадёт («open
  /tmp/plugin…: read-only file system»). Для будущей песочницы Этапа 9
  /tmp должен быть writable (tmpfs/bind).
- Схема `local://` в источниках требует формы `local:///file` (три слэша):
  `local://file` парсится как host=file с пустым path и падает с «is a
  directory». Стоит задокументировать или нормализовать.
- **Из финального ревью (Important, принято):** после переноса проверки
  привилегий в `install` (7.3) непривилегированный пользователь с
  `autoPull: true` сначала упадёт на доступе к общему кэшу
  (`db.Init`/pull до точки проверки, `cmd/install.go:52-88`) — до
  «понятной ошибки про группу» дойдёт не всегда. Приемлемо: сообщение об
  ошибке всё равно указывает на права; поднимать проверку выше WithDB не
  требуется.

---

## Этап 8: D-Bus polkit + setuid-сборка

Результат: мутирующие методы демона (Install/Remove/RefreshRepositories)
авторизуются через polkit по uid ВЫЗЫВАВШЕГО (не демона); при недоступном
polkit — fail-closed (только uid 0). Сборочная часть install в демоне
выполняется от uid вызвавшего (plugin-процесс с SysProcAttr.Credential +
HOME вызвавшего) в его XDG-кэше; кэш-доверие демона параметризовано uid
задачи. Интерактивные промпты в демон-режиме выключены (нет TTY).

Ключевые факты текущего кода (для задач):
- `DBusPackage.checkAuthorization` (package.go:389) — заглушка, всегда nil.
- `PolicyKitAuthorizer.CheckAuthorization` (policykit.go) — subject из
  PID/UID ДЕМОНА + упрощённый start-time; первый аргумент polkit-метода
  (subject kind) не передаётся — вызов невалиден, нужен реворк.
- godbus v5.2.2 поддерживает `dbus.Sender` первым параметром метода
  (в introspection не попадает — интерфейс не меняется).
- Скрипты выполняются в plugin-процессе `_internal-safe-script-executor`
  (internal/build/plugins.go:getSafeExecutor); в нём же prepareDirs и
  copyToCache — setuid в этой точке покрывает всё недоверенное исполнение.
- Демон строит пути из СВОЕГО конфига (root) — для job нужен конфиг с
  XDG-путями вызвавшего (HOME из `user.LookupId(uid).HomeDir`).
- Доверие кэша (Этап 4): isTrustedOwner пропускает только euid/0/65534 —
  в демоне (root) файлы вызвавшего не пройдут; нужен job-scope trusted uid.

### Task 8.1: uid вызвавшего + polkit-авторизация

**Файлы:**
- Create: `internal/dbus/caller.go` (резолвер uid по sender)
- Modify: `internal/dbus/policykit.go` (subject kind "unix-user", per-caller)
- Modify: `internal/dbus/package.go` (Install/Remove: sender + auth),
  `internal/dbus/manager.go` (RefreshRepositories: sender + auth)
- Modify: `internal/dbus/service.go` (поле Authorizer + резолвер, инъекция)
- Test: `internal/dbus/auth_test.go` (новый)

- [x] **Шаг 1: падающий тест** — Install с фейковым Authorizer
  (возвращает false) → ошибка NotAuthorized, job НЕ создаётся; с
  возвращающим true → проходит; резолвер подменяется, Authorizer получает
  uid 1000 и action "ru.alr-pkg.install".

- [x] **Шаг 2: реализация**
  - `caller.go`: `resolveCallerUID(conn, sender) (uint32, error)` через
    `org.freedesktop.DBus.GetConnectionUnixUser`; за переменной-швом
    (`var getConnectionUnixUser = ...`) для тестов.
  - `policykit.go`: `CheckAuthorization(actionID string, uid uint32,
    details, allowUserInteraction)` — subject kind `"unix-user"`,
    details `{"uid": uid}`; корректная сигнатура polkit-метода
    (kind, subject, actionID, details, flags, cancellationID);
    polkit недоступен/ошибка → fallback: разрешено только uid==0.
  - `Service`: поля `authorizer Authorizer` (default — PolicyKitAuthorizer)
    и возможность подмены в тестах.
  - `Install(sender dbus.Sender, options)`, `Remove(sender, options)`,
    `RefreshRepositories(sender)` — sender первым параметром; auth до
    создания job; ошибка → `ru.alr-pkg.ALR.Error.NotAuthorized`.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/dbus/ -v -count=1 && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: polkit-авторизация по uid вызвавшего в демоне`

### Task 8.2: job-конфиг вызвавшего (пути + доверие кэша)

**Файлы:**
- Create: `internal/dbus/caller_env.go` (HOME/USER по uid, конфиг job)
- Modify: `internal/build/cache.go` (параметризация доверенных uid)
- Modify: `internal/build/build.go` (проброс trusted uid в
  verifyTrustedPaths)
- Test: `internal/dbus/caller_env_test.go`, `internal/build/cache_test.go`

- [x] **Шаг 1: падающий тест**
  - `callerEnv(uid)`: HOME/USER/LOGNAME из `user.LookupId` (шов подмены);
    uid 65534 без passwd-записи → фолбэк HOME=/var/cache/alr (nobody).
  - `Cache` с `trustedUIDs [1000]`: файл владельца 1000 → hit
    (без — miss); то же для verifyTrustedPaths.

- [x] **Шаг 2: реализация**
  - `internal/dbus/caller_env.go`: `callerEnv(uid uint32) ([]string,
    error)` (env для plugin-процесса: HOME, USER, LOGNAME, PATH — как
    setCommonCmdEnv, но для вызвавшего; nobody → /var/cache/alr) и
    `callerCachePaths(uid) (buildDir, pkgsDir string)` —
    `<home>/.cache/alr/build|pkgs` (nobody → /var/cache/alr/build|pkgs).
  - `internal/build/cache.go`: `Cache` + поле trustedUIDs (по умолчанию
    как сейчас); `isTrustedOwner` → метод/вариант с учётом списка;
    `verifyTrustedPaths` получает тот же список (подумай о сигнатуре —
    минимальное инвазивное изменение; существующие вызовы CLI не ломать).
  - Конструирование job-Builder'а — Task 8.3.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/dbus/ ./internal/build/ -v -count=1 && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: job-окружение вызвавшего и доверие кэша по uid`

### Task 8.3: setuid-запуск script executor в демоне

**Файлы:**
- Modify: `internal/build/plugins.go` (вариант запуска с Credential+env)
- Modify: `internal/dbus/package.go` (runInstall: job-Builder с
  caller-исполнителем и caller-путями)
- Test: `internal/build/plugins_test.go` (конфиг команды)

- [x] **Шаг 1: падающий тест** — `getSafeExecutorAs` строит cmd с
  `SysProcAttr.Credential{Uid,Gid}` и env вызвавшего (проверка через шов —
  вынести построение cmd в функцию, возвращающую *exec.Cmd для
  инспекции до запуска).

- [x] **Шаг 2: реализация**
  - `internal/build/plugins.go`: `GetSafeScriptExecutorAs(uid, gid uint32,
    env []string)` (или options-структура) — cmd + `syscall.SysProcAttr{
    Credential: &syscall.Credential{Uid: uid, Gid: gid}}`, env = callerEnv
    (HOME/USER/LOGNAME/PATH), TMPDIR — оставить /tmp (plugin пишет в
    os.TempDir; /tmp доступен всем).
  - `internal/dbus/package.go` runInstall: uid из auth-контекста;
    job-Builder: cfg с подменёнными BuildDir/PkgsDir (callerCachePaths),
    Cache с trustedUIDs[uid], scripter = GetSafeScriptExecutorAs(uid,...);
    installer — как раньше (root).
  - Не ломать CLI-путь: GetSafeScriptExecutor без uid работает как раньше.

- [x] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ ./internal/dbus/ -v -count=1 && go build ./...`
Expected: PASS

- [x] **Шаг 4: коммит** `Добавлено: сборка в демоне от uid вызвавшего`

### Task 8.4: запрет интерактивности в демон-режиме

**Файлы:**
- Modify: `internal/dbus/package.go` (runInstall/runRemove)

- [x] **Шаг 1: реализация** — BuildOpts.Interactive принудительно false
  (опция `interactive` из options игнорируется с slog.Warn; NoConfirm для
  Remove = true), чтобы скрипты не могли зависнуть на промпте без TTY.
- [x] **Шаг 2: тесты/проверка** — `go build ./...`, `go test ./internal/dbus/`
- [x] **Шаг 3: коммит** `Изменено: демон всегда работает неинтерактивно`

### Task 8.5: сквозная проверка Этапа 8

- [x] Сессионная шина: демон (`_internal-dbus-service --session`) стартует,
  владеет `ru.alr-pkg.ALR`, `alr dbus status` → running
- [x] polkit на хосте есть (polkitd + KDE-агент), но policy-файл
  `configs/polkit/ru.alr-pkg.policy` НЕ установлен в систему (нет root):
  `alr dbus install alr-repo/flake8` → отказ `not authorized for action
  ru.alr-pkg.install` (ошибка CheckAuthorization по неизвестному action →
  fallbackCheck, uid 1000 ≠ 0 → deny). Грант-ветка polkit живьём не
  проверена — требуется установка .policy в /usr/share/polkit-1/actions
- [x] fail-closed без polkit: демон под
  `bwrap --dev-bind / / --tmpfs /run/dbus` (системная шина недоступна) →
  `dbus install` → NotAuthorized, в журнале демона
  `Authorization denied action=ru.alr-pkg.install uid=1000`;
  `Manager.RefreshRepositories` → `ru.alr-pkg.ALR.Error.NotAuthorized`
- [x] Неавторизованный вызов: NotAuthorized на живой шине (оба сценария
  выше) + моки в `internal/dbus/auth_test.go`
- [ ] Сборка в демоне + владельцы файлов: НЕ проверено живьём — до
  runInstall не добраться без гранта polkit (см. выше); сессионный демон
  и так имеет uid вызвавшего (setuid неотличим). Root-режим демона
  (системная шина + setuid на uid вызвавшего) не проверен — нет sudo;
  bwrap-userns не даёт CAP_SETUID для смены на произвольный хост-uid.
  Покрыто юнитами (`caller_job_test.go`, Credential/trustedUIDs)
- [x] Регрессия CLI: `alr build -s` тестового рецепта (alr-e8-test) —
  сборка и кэш-хит OK, пакет в `~/.cache/alr/pkgs` принадлежит
  пользователю (xpamych:xpamych); `alr info` — как раньше
- [x] `go build ./...`, `go test ./...` (без e2e), pre-commit

**Найдено при проверке (Task 8.5) — блокеры живой работы D-Bus, предшествуют
ветке (из eee3f35), исправлены локально для проверки и откачены; нужен
отдельный фикс:**

1. **Невалидные имена интерфейсов и ошибок D-Bus.** `ru.alr-pkg.ALR.Manager`,
   `.Package`, `.Job`, `ru.alr-pkg.ALR.Error.*` содержат дефис — по спеке
   D-Bus дефисы запрещены в именах интерфейсов/ошибок (в bus names — можно).
   godbus отвергает такие сообщения на отправке (`dbus: invalid message:
   invalid interface name`), gdbus падает по assert. Итог: НИ ОДИН вызов
   клиент→демон на живой шине не работает (`dbus status` всегда
   "not running", install/search падают). Фикс: `alr-pkg` → `alr_pkg` в
   константах интерфейсов/ошибок (`internal/dbus/types.go` и по коду);
   well-known name `ru.alr-pkg.ALR` менять не нужно.
2. **Невалидные object path.** `GetPackageObjectPath` подставляет repo/name
   как есть — `alr-repo`/`alr-default` содержат дефис (запрещён в элементах
   пути) → `invalid object path` в ответе демона. `GetJobObjectPath`
   кодирует id как `string(rune(id))` (control-символ) — тоже невалидный
   путь. Фикс: санитайз элементов пути (замена недопустимых символов на
   `_`) и `strconv.FormatUint` для id (`internal/dbus/types.go:178-185`).
3. Сопутствующее: fuzzy-перевод "Failed to get package" →
   «Не удалось создать пакет» (`internal/translations/po/ru/default.po`) —
   вводит в заблуждение при диагностике, поправить перевод.
4. При запуске демона с БД старой версии (6 vs 8) демон молча сбрасывает
   настройки пользователя («Несоответствие версий базы данных; сброс
   настроек») — для мутирующего сервиса стоит логировать louder/требовать
   `alr fix`, а не сбрасывать в фоне.
5. `alr dbus start` запускает `alr` из PATH (системный, старой версии),
   а не текущий бинарник — при смешении версий на шине оказывается чужой
   демон. Стоит использовать `os.Executable()`.

### Task 8.6: фикс валидности D-Bus имён (по находкам 8.5)

**Файлы:**
- Modify: `internal/dbus/types.go` (константы интерфейсов, санитайз путей)
- Modify: `internal/dbus/*.go` (имена ошибок), `cmd/dbus.go` (os.Executable)
- Modify: `internal/translations/po/ru/default.po` (fuzzy-перевод)
- Test: `internal/dbus/types_test.go` (новый или существующий)

- [x] **Шаг 1: падающий тест** — имена интерфейсов валидны по спеке D-Bus
  (без дефиса: `ru.alr_pkg.ALR.*`); `GetPackageObjectPath("alr-repo", "x")`
  — валидный путь (санитайз дефиса); `GetJobObjectPath(1)` — цифры, не
  control-символы.
- [x] **Шаг 2: реализация** — `alr-pkg` → `alr_pkg` в именах
  интерфейсов/ошибок (well-known name `ru.alr-pkg.ALR` НЕ менять);
  санитайз элементов object path (недопустимые символы → `_`);
  `GetJobObjectPath` — `strconv.FormatUint`; `cmd/dbus.go` start —
  `os.Executable()`; fuzzy-перевод «Failed to get package».
- [x] **Шаг 3: проверка на живой шине** — `alr dbus status`, `search`
  работают без танцев; `go test ./internal/dbus/ ./cmd/ -count=1`
- [x] **Шаг 4: коммит** `Исправлено: валидность имён и путей D-Bus`

---

## Этап 9: встроенная песочница

Результат: код alr.sh (prepare/build/package и всё, что они запускают)
выполняется в песочнице: rw только дерево сборки, кэш пакетов и /tmp;
система ro; $HOME недоступен; свежий /proc. Без внешних зависимостей
(только ядро). Каскад деградации: user+mount+pid namespaces → landlock
(5.13+) → без песочницы + warning. Ручка отключения: `ALR_SANDBOX=off`.

Ключевые факты текущего кода:
- Недоверенное исполнение — в plugin-процессе `_internal-safe-script-executor`
  (internal/build/plugins.go:getExecutorCmd). Именно его и песочним.
- Уже есть зависимость `git.alr-pkg.ru/ALR/fakeroot` (v0.0.4): CLONE_NEWUSER
  + uid-map 0→euid на каждую команду скрипта (handlers/fakeroot.go) —
  показывает, что userns на целевых системах доступен; внутри нашей
  песочницы (uid 0 в ns) fakeroot.Apply завершается no-op по uid==0.
- На хосте разработки проверено: `unshare -U -m -p -f` работает, ядро 6.12.
- Из находки 7.4: go-plugin пишет temp в /tmp — /tmp обязан быть rw.
- `golang.org/x/sys/unix` v0.33 содержит landlock API
  (LandlockCreateRuleset/AddRule/RestrictSelf).

Макет ФС в песочнице (вычисляется из путей конфига):
- rw: `BuildDir` (дерево сборки), `PkgsDir` (кэш пакетов — copyToCache и
  `.alr-cache.lock` внутри plugin'а), `/tmp` (go-plugin socket, os.TempDir)
- ro: `ScriptDir` (alr.sh), `/usr`, `/lib`, `/lib64`, `/bin`, `/sbin`,
  `/etc` (resolv.conf и т.п. — сеть сохраняется, CLONE_NEWNET нет)
- /dev: только null/zero/random/urandom (ro bind), /proc — свежий
- dl-кэш и $HOME вне BuildDir/PkgsDir — не монтируются (скачивание
  sources — в родителе; plugin видит только готовый srcdir в дереве)
- gid-mapping: только основная группа (0→gid); групповые rw-каталоги
  хоста в песочницу не монтируются — задокументированное ограничение

Протокол: родитель передаёт макет init'у JSON'ом через fd 3
(`exec.Cmd.ExtraFiles`) — без временных файлов.

### Task 9.1: internal/sandbox — макет и опции

**Файлы:**
- Create: `internal/sandbox/sandbox.go` (Options, Layout, вычисление из cfg)
- Test: `internal/sandbox/sandbox_test.go`

- [x] **Шаг 1: падающий тест** — `LayoutFromPaths(buildDir, pkgsDir,
  scriptDir)` → корректный набор mount-записей (rw/ro, дедупликация
  вложенных путей: BuildDir внутри PkgsDir и наоборот не дублируется;
  scriptDir совпадает с rw-путём → rw побеждает), JSON-маршалинг туда-обратно.
- [x] **Шаг 2: реализация** — типы `Mount{Source, Target, RW bool}`,
  `Layout{Mounts []Mount, DevNodes []string, ProcFresh bool}`; дедупликация
  по принципу «наиболее специфичный путь, rw сильнее».
- [x] **Шаг 3: тесты проходят** — `go test ./internal/sandbox/ -v`
- [x] **Шаг 4: коммит** `Добавлено: пакет sandbox (макет ФС)`

### Task 9.2: init-команда и Wrap (namespaces)

**Файлы:**
- Create: `internal/sandbox/init.go` (точка входа `_internal-sandbox-init`)
- Modify: `internal/sandbox/sandbox.go` (`Wrap(cmd, layout)` — SysProcAttr)
- Modify: регистрация скрытой команды (cmd/)
- Test: `internal/sandbox/integration_test.go`

- [x] **Шаг 1: падающий интеграционный тест** (skip, если userns
  недоступен): в песочнице запустить `/bin/sh -c`: запись в layout-rw —
  OK; в $HOME — EACCES; чтение /etc/os-release — OK; `ls /proc | wc -l`
  маленький (fresh proc, pid 1 — сам init).
- [x] **Шаг 2: реализация**
  - `Wrap(cmd, layout, uid, gid)`: cmd.Path → os.Executable, args →
    `_internal-sandbox-init -- <original args>`, SysProcAttr:
    `Cloneflags = CLONE_NEWUSER|CLONE_NEWNS|CLONE_NEWPID`,
    `UidMappings/GidMappings [{0, uid, 1}]`, ExtraFiles + JSON layout на fd 3.
    Возвращает ошибку «userns недоступен» для каскада (детект — на запуске,
    пробный exec? решай: фактический запуск init сообщает код выхода;
    для каскада в 9.3 нужен способ отличить «песочница не завелась» от
    «команда внутри упала» — exit code init'а зарезервировать, напр. 70).
  - `_internal-sandbox-init`: читает layout с fd 3; создаёт newroot
    (tmpfs в /tmp/alr-sandbox-<pid>); bind-mounts по макету (ro через
    MS_BIND|MS_REMOUNT|MS_RDONLY); /dev ноды (bind ro); mount fresh /proc;
    pivot_root(newroot, put_old) + umount2(MNT_DETACH); chdir в дерево
    или /; syscall.Exec(target, args, env).
- [x] **Шаг 3: тесты проходят** — `go test ./internal/sandbox/ -v -count=1`
- [x] **Шаг 4: коммит** `Добавлено: песочница на namespaces (re-exec init)`

### Task 9.3: landlock-фолбэк и интеграция в script executor

**Файлы:**
- Create: `internal/sandbox/landlock.go`
- Modify: `internal/build/plugins.go` (script executor — в песочнице)
- Test: `internal/sandbox/landlock_test.go`, обновить plugins_test.go

- [x] **Шаг 1: падающий тест** — `ApplyLandlock(layout)`: после вызова
  запись вне rw-путей → EACCES, чтение системы — OK (тест на живом ядре,
  skip < 5.13).
- [x] **Шаг 2: реализация** — landlock ABI v1: ruleset с
  handled_access_fs = все FS_* (что доступно в x/sys v0.33), правила
  path_beneath для rw-путей (write) и ro-путей+системы (read);
  RestrictSelf(NoNewPrivs). В plugins.go: script executor →
  `sandbox.Wrap`; при ошибке запуска (exit 70 / отказ ядра) → повтор с
  landlock-режимом (init применяет landlock к себе перед exec — отдельный
  режим `_internal-sandbox-ll`? или флаг init'а «если ns не удалось» — решай
  простое: родитель при недоступности userns запускает target напрямую,
  а landlock применяет обёртка-init без ns) → при недоступности и этого
  → прямой запуск + slog.Warn. `ALR_SANDBOX=off` — прямой запуск.
- [x] **Шаг 3: тесты проходят** — `go test ./internal/sandbox/ ./internal/build/ -v`
- [x] **Шаг 4: коммит** `Добавлено: landlock-фолбэк песочницы, script executor в песочнице`

### Task 9.4: демон-путь (caller-mapping вместо Credential)

**Файлы:**
- Modify: `internal/dbus/package.go` + `internal/build/plugins.go`
  (GetSafeScriptExecutorAs — через sandbox с uid/gid вызвавшего)

- [x] **Шаг 1: реализация** — в демоне plugin стартует через sandbox.Wrap
  с uid/gid вызвавшего (map 0→callerUID) вместо/поверх Credential
  (совместимость Credential+NEWUSER: выбрано — в ns-попытке Credential
  сбрасывается, его заменяет маппинг 0→callerUID/0→callerGID; в landlock
  и прямом режиме Credential{Uid,Gid,Groups} сохраняется).
  Layout демона: callerCachePaths(uid) rw (через callerJobConfig),
  RepoDir ro. Supplementary groups в ns теряются (маппинг одного uid/gid)
  — задокументированное ограничение: скачивание sources выполняет
  родитель (демон, root), поэтому групповые rw-каталоги (dl-кэш
  root:alr) в песочнице не нужны.
- [x] **Шаг 2: тесты** — unit на конфиг Wrap для демон-пути; прогон всех
  `go test ./internal/dbus/ ./internal/build/ ./internal/sandbox/`
- [x] **Шаг 3: коммит** `Изменено: демон собирает в песочнице с uid-маппингом вызвавшего`

### Task 9.5: сквозная проверка Этапа 9

- [x] CLI `alr build -s`: рецепт, пишущий в $HOME (`touch $HOME/escape`) →
  ошибка сборки; пишущий в дерево/пакет — OK; кэш-хиты работают
  (мета/пакеты пишутся внутри песочницы)
  - Проверено 2026-09-22 (RED OS 8.0.3, userns доступен): `touch
    $HOME/escape-attempt` в build() → EROFS («файловая система доступна
    только для чтения»), сборка падает (exit status 1), файл в $HOME не
    появился; `touch /usr/escape` → аналогично EROFS, сборка падает.
    Рецепт, пишущий в pkgdir/srcdir — собирается (rpm + .meta в кэше).
    Кэш-хит на повторе: «Используется кешированный пакет name=…»
    (мета/пакет опубликованы из песочницы). В sandboxed-сборке build()
    видит новые user/pid ns (отличные от хостовых).
  - Находка (9.4, не мешает): при ALR_LOG_LEVEL=DEBUG в ns-режиме видно
    `Lock: не удалось сменить группу лок-файла … .alr-cache.lock: invalid
    argument` — normalizeLockAccess (fsutils/lock.go:210) дёргает chown
    на незамапленный gid внутри userns. Ошибка Debug-уровня и нефатальна:
    сборка и публикация в кэш проходят, exit 0.
- [x] Рецепт, читающий /etc и качающий из сети (если есть) — OK
  - `head /etc/os-release` в build() читается; скачивание с локального
    http.server на 127.0.0.1 (CLONE_NEWNET нет, loopback работает) — OK,
    пакет собран.
- [x] ALR_SANDBOX=off — старое поведение
  - С ALR_SANDBOX=off сборка того же рецепта проходит, `touch
    $HOME/escape-attempt` УСПЕШЕН (файл создан, удалён после проверки) —
    прямой запуск без песочницы. uid=0 в build() при этом — mvdan/sh
    FakerootExecHandler (script_executor.go:102), предсуществующее
    поведение, не песочница.
- [x] Регрессия: `go build ./...`, `go test ./...`, pre-commit
  - `go build ./...` — OK; `go test` по всем пакетам (без e2e-tests) —
    все ok; `pre-commit run --all-files` — Passed (включая gofmt/license/
    i18n/coverage хуки).
- [x] Демон (сессионная шина, как в 8.5): install маленького пакета
  - `alr _internal-dbus-service --session` (фон), `alr dbus install
    alr-default/alr-bin` → «not authorized for action ru.alr-pkg.install»
    (fail-closed: .policy на хосте не установлен, пароль не запрашивался);
    в логе демона warn «Authorization denied» uid=1000. Auth работает;
    сборка через демон НЕ проверена (нет root/разрешения polkit на
    хосте). Демон остановлен после проверки.

---

## Этап 10: состав работ (детальный план — перед стартом)

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
  fix создаёт системные каталоги `/var/cache/alr/build` и
  `/var/cache/alr/pkgs` от пользователя с 0755 вместо `root:group 2775`
  (`cmd/fix.go:100-109`) — либо пропускать создание, когда каталог внутри
  `CacheDir`, либо использовать `EnsureTempDirWithRootOwner`. Заодно:
  сообщения fix говорят «build directory» и про кэш пакетов
  (`Unable to remove/create build directory` для PkgsDir) — параметризовать.
- **Из финального ревью (Minor 1):** решить конвенцию по gotext для
  ошибок внутри пакетов — `fsutils.CheckFreeSpace` сейчас возвращает
  русские строки мимо gotext (прецедент в fsutils уже был; либо принять
  как конвенцию пакета, либо поднимать формирование сообщения в cmd-слой).
- **Из финального ревью Этапа 3 (Minor 3):** fix удаляет `.lock`-файлы
  работающих процессов вместе с кэшем (`cmd/fix.go` — RemoveAll CacheDir и
  PkgsDir) — flock живёт на inode, поэтому взаимное исключение ломается.
  При переработке fix: перед удалением проверять активные локи
  (`fsutils.TryLock` на каждый `.lock` — занят ⇒ не трогать/предупредить).

---

## Замечания для всех этапов

- Коммиты в стиле проекта: `Добавлено: …` / `Изменено: …` / `Исправлено: …`
- Новые строки UI — через `gotext.Get` + обновление `internal/translations`
- `CI=true`-режим не должен сломаться: e2e прогон после каждого этапа
- Каждый этап — отдельный PR в master, порядок строго последовательный
  (этапы 3+ можно параллелить только после Этапа 1)
