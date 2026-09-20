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

- [ ] **Шаг 1: падающий тест** — пакет+совпадающая мета → hit; мета
  отсутствует/битая/не совпала → miss (даже при существующем пакете);
  недоверенный владелец меты тоже → miss (мета проверяется как и пакет).

- [ ] **Шаг 2: реализация** — `CheckForBuiltPackage(ctx, input, vars, funcsKey
  string)`; внутри после findTrustedCachedPkg: вычислить
  `SemanticCacheKey(vars, funcsKey)`, прочитать и сверить мету.
  Обновить интерфейс `cacheExecutor` (build.go:190) и вызовы:
  - BuildPackage: `funcsKey` из sf (доступен после ExecuteFirstPass);
  - checkCacheForAllSubpackages: скрипт резолвится через
    `b.scriptResolver.ResolveScript(ctx, pkg)` + `b.scriptExecutor.ReadScript`;
    ошибка резолва/чтения → miss с slog.Debug (безопасная сторона), НЕ ошибка.

- [ ] **Шаг 3: тесты проходят**

Run: `go test ./internal/build/ -v && go build ./...`
Expected: PASS

- [ ] **Шаг 4: коммит** `Добавлено: проверка семантического ключа кэша (.meta)`

### Task 5.4: сквозная проверка Этапа 5

- Сборка → кэш-хит; правка description в alr.sh → всё ещё кэш-хит; правка
  тела build() → пересборка; бамп version → новый файл (поведение как раньше)
- Поток install/dep через checkCacheForAllSubpackages — проверить по логам
  (скрипт резолвится, мета сверяется)
- `go build ./...`, `go test ./...`, pre-commit

---

## Этапы 6-10: состав работ (детальный план — перед стартом каждого)

### Этап 6: группа alr для dlcache
- Установщик/пакеты создают группу `alr`; `/var/cache/alr/dl` — `root:alr 2775`
- `fsutils.EnsureTempDirWithRootOwner` перестаёт дёргать sudo для членов
  группы (проверка `user.GroupIds`)
- Убрать sudo-спам из `pkg/dlcache/dlcache_prod.go`
- **Из финального ревью Этапов 1-2 (Minor 2):** добавить
  `CheckFreeSpace(paths.CacheDir, …)` перед `DownloadSources` — сейчас
  проверяется только ФС дерева сборки, а `/var/cache/alr/dl` может быть
  другой ФС (ENOSPC посреди скачивания).
- **Из ре-ревью Этапа 5 (Important-follow-up):** реестр локов fsutils
  (`heldLocks`) сейчас держит общий `locksMu` на время БЛОКИРУЮЩЕГО
  `unix.Flock` в `fsutils.Lock` — все TryLock/Lock/Close процесса в этот
  момент сериализуются (для демона с горутинами на D-Bus job реально).
  Семантика ErrLocked не сломана, дедлока нет, но нужен рефакторинг:
  per-path pending-записи («acquiring» под мьютексом, ожидание на
  channel/cond), flock — вне мьютекса; заодно retry на EINTR. НЕ чинить
  наивным «flock вне мьютекса» — это сломает реентерабельность (два
  конкурентных Lock одного пути откроют по OFD вместо refs++).

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
