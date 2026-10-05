<p align="center">
    <img src="assets/logo.png" width="15%">
</p>
<b></b>

[![Go Report Card](https://goreportcard.com/badge/git.alr-pkg.ru/ALR/ALR)](https://goreportcard.com/report/git.alr-pkg.ru/ALR/ALR) ![Test coverage](./assets/coverage-badge.svg) ![ru translate](./assets/i18n-ru-badge.svg)

# ALR (Any Linux Repository)

ALR - это независимая от дистрибутива система сборки для Linux (форк [LURE](https://github.com/lure-sh/lure), аналогичная [AUR](https://wiki.archlinux.org/title/Arch_User_Repository). В настоящее время она находится в стадии бета-тестирования. Исправлено большинство основных ошибок и добавлено большинство важных функций. ALR готов к общему использованию, но все еще может время от времени ломаться или изменяться.

ALR написан на чистом Go и после сборки не имеет зависимостей. Для повышения привилегий ALR требуется команда, такая как `sudo`, `doas` и т.д., а также поддерживаемый менеджер пакетов. В настоящее время ALR поддерживает `apt`, `apt-get` `pacman`, `apk`, `dnf`, `yum`, and `zypper`. Если в вашей системе используется поддерживаемый менеджер пакетов, то он будет обнаружен и использован автоматически.

---

## Установка

### Установка скриптом

Установочный скрипт автоматически загрузит и установит соответствующий пакет ALR в вашей системе. Чтобы использовать его, просто выполните следующую команду:

```bash
curl -fsSL https://git.alr-pkg.ru/ALR/ALR/raw/branch/master/scripts/install.sh | bash
```

**ВАЖНО**: При этом скрипт будет загружен и запущен [скрипт](https://git.alr-pkg.ru/ALR/ALR/src/branch/master/scripts/install.sh). Пожалуйста, просматривайте любые скрипты, которые вы скачиваете из Интернета (включая этот), прежде чем запускать их.

### Сборка из исходного кода

Чтобы собрать ALR из исходного кода, вам понадобится версия Go 1.18 или новее. Как только Go будет установлен, клонируйте это репозиторий и запустите:

```shell
make build -B
sudo make install
```

---

## Почему?

ALR был создан потому, что упаковка программного обеспечения для нескольких дистрибутивов Linux может быть сложной и чреватой ошибками, а установка этих пакетов может стать кошмаром для пользователей, если они не доступны в официальных репозиториях их дистрибутива. Он автоматизирует процесс создания и установки неофициальных пакетов.

---

## Сайт и RPC-режим

ALR умеет работать с сайтом [alr-pkg.ru](https://alr-pkg.ru) напрямую, без клонирования git-репозиториев: поиск (`alr search`), список пакетов (`alr list`), проверка обновлений (`alr upgrade`) и автодополнение в shell используют RPC API сайта с локальным кэшированием и автоматическим откатом на локальную БД при недоступности сайта.

Управление настройками сайта:

```bash
alr config set site.url https://alr-pkg.ru   # адрес сайта
alr config set site.timeout 10               # таймаут запросов (сек)
alr config set site.preferRPC true           # RPC-first для search/list/upgrade
alr config set site.disableRPC false         # полностью отключить RPC
```

Те же настройки можно задать переменными окружения: `ALR_SITE_URL`, `ALR_SITE_TIMEOUT`, `ALR_SITE_PREFER_RPC`, `ALR_SITE_DISABLE_RPC`.

Пользовательские настройки хранятся в `~/.config/alr/alr.toml` и имеют приоритет над системным `/etc/alr/alr.toml`. Управление — через `alr config set/get/show`.

---

## Документация

Документация находится в [Wiki](https://alr-pkg.ru/wiki/ALR).

---

## Репозитории

Репозитории alr - это git-хранилища, которые содержат каталог для каждого пакета с файлом `alr.sh` внутри. Файл `alr.sh` содержит все инструкции по сборке пакета и информацию о нем. Скрипты `alr.sh` аналогичны скриптам Aur PKGBUILD. 

Например, репозиторий с ALR [alr-default](https://git.alr-pkg.ru/ALR/alr-default.git)
```
alr repo add alr-default https://git.alr-pkg.ru/ALR/alr-default.git
```
Репозиторий пакетов [alr-repo](https://git.alr-pkg.ru/ALR/alr-repo.git) можно подключить так:
```
alr repo add  alr-repo https://git.alr-pkg.ru/ALR/alr-repo.git
```
Репозиторий Linux-Gaming [alr-RG](https://git.alr-pkg.ru/ALR/alr-RG.git) можно подключить так:
```
alr repo add alr-LG https://git.linux-gaming.ru/Linux-Gaming/alr-LG.git
```

---
## Соцсети
Telegram - https://t.me/alr_pkg

## Спасибы

Благодарим следующие проекты за то, что они сделали все возможное:

- <https://github.com/mvdan/sh>
- <https://github.com/go-git/go-git>
- <https://github.com/mholt/archiver>
- <https://github.com/goreleaser/nfpm>
- <https://github.com/charmbracelet/bubbletea>
- <https://gitlab.com/cznic/sqlite>
