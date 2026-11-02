# Localized Git Progress Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить английский серверный прогресс Git локализованными спиннером и градиентным progress-bar ALR.

**Architecture:** В `internal/repos` появится writer для sideband-потока go-git. Он будет объединять фрагменты в сообщения, распознавать этап, текущее и общее количество и делегировать отображение общему компоненту прогресса; необработанные сообщения не попадут в вывод. `pullRepoFromURL` подключит writer к `FetchContext`.

**Tech Stack:** Go, go-git sideband `io.Writer`, gotext, существующие lipgloss-градиенты.

**Spec:** Требования зафиксированы в диалоге: спиннер при неизвестном итоге, существующий стиль progress-bar при известном, локализация всех сообщений, без изменений других индикаторов.

## Global Constraints

- Не изменять существующие progress-bar скачивания, обработки пакетов и обновлений.
- Не добавлять зависимости.
- Все пользовательские подписи получать через `gotext.Get`.
- Не выполнять git commit без отдельной просьбы пользователя.

## Review Focus

- Фрагментированные sideband-сообщения и `\r`: тесты writer подают одно логическое сообщение несколькими вызовами `Write`.
- Неизвестный итог: тест `Enumerating objects` проверяет режим спиннера и отсутствие процентов.
- Известный итог: тесты `Counting objects`, `Compressing objects`, `Receiving objects`, `Resolving deltas` проверяют процент и счётчики.
- Неизвестные сообщения: тест подтверждает, что серверный английский текст не протекает в интерфейс.
- Ошибки записи: тест подтверждает корректный возврат `(len(p), nil)` и завершение индикатора.

---

### Task 1: Парсер и отображение Git-прогресса

**Files:**
- Create: `internal/repos/git_progress.go`
- Create: `internal/repos/git_progress_test.go`
- Modify: `internal/repos/pull.go:200`
- Modify: `internal/translations/po/ru/default.po`
- Modify: `internal/translations/po/en/default.po`
- Modify: `internal/translations/default.pot`

**Interfaces:**
- Consumes: go-git `FetchOptions.Progress io.Writer`.
- Produces: `newGitProgressWriter(out io.Writer) io.WriteCloser`, принимающий сырой sideband-текст и закрывающий активный индикатор через `Close`.

- [ ] **Step 1: Написать failing-тесты**

Проверить фрагментированный ввод; спиннер для `Enumerating objects: 20298, done.`; градиентный бар и локализованную подпись для `Counting objects: 50% (10/20)`; распознавание `Compressing objects`, `Receiving objects`, `Resolving deltas`; отсутствие необработанных английских строк; повторный `Close`.

- [ ] **Step 2: Запустить тесты и увидеть ожидаемый FAIL**

Run: `go test ./internal/repos -run GitProgress -v`

- [ ] **Step 3: Реализовать writer и общий renderer**

Writer накапливает байты до `\r` или `\n`, парсит подпись, процент и `(current/total)`. Для неизвестного итога используется спиннер, для известного — полоса из 40 градиентных `●`/`○`, процент и счётчики. Подписи: `Enumerating objects`, `Counting objects`, `Compressing objects`, `Receiving objects`, `Resolving deltas`.

- [ ] **Step 4: Подключить writer к FetchContext**

Заменить `Progress: os.Stderr` на новый writer, гарантированно закрывая его после fetch и до последующих сообщений.

- [ ] **Step 5: Обновить переводы**

Добавить английские исходные строки и русские переводы через штатный `make i18n`, проверив diff на отсутствие посторонних изменений.

- [ ] **Step 6: Проверить результат**

Run: `go test ./internal/repos -run GitProgress -v`, затем `go test ./...`, `go build ./...` и сценарий `alr in` с получением Git-репозитория. Ожидаются только локализованные индикаторы без серверных английских строк.
