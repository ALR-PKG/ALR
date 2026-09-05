#!/bin/sh
# ALR - Any Linux Repository
# Copyright (C) 2026 The ALR Authors
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# build.sh — сборка e2e-образов для всех дистрибутивов.
#
# Использование:
#   ./build.sh                      # собрать все образы локально
#   PUSH=1 ./build.sh <prefix...>   # собрать и запушить в один или
#                                   # несколько реестров (prefix без суффикса
#                                   # дистрибутива)
#
# Пример:
#   PUSH=1 ./build.sh \
#     git.alr-pkg.ru/plemya-x/alr-e2e-test-image \
#     cr.gitverse.ru/alr/alr-e2e-test-image
#
# Итоговое имя образа: <prefix>-<distro>:latest

set -e
cd "$(dirname "$0")"

DISTROS="ubuntu-24.04 alt-sisyphus fedora-41 archlinux alpine opensuse-leap"
LOCAL_PREFIX="localhost/alr-e2e-test-image"

for distro in $DISTROS; do
	echo "=== Сборка $distro ==="
	podman build -f "Dockerfile.$distro" -t "$LOCAL_PREFIX-$distro:latest" .
done

if [ "$PUSH" = "1" ]; then
	if [ $# -eq 0 ]; then
		echo "PUSH=1 требует хотя бы один registry prefix" >&2
		exit 1
	fi
	for prefix in "$@"; do
		for distro in $DISTROS; do
			echo "=== Пуш $prefix-$distro:latest ==="
			podman tag "$LOCAL_PREFIX-$distro:latest" "$prefix-$distro:latest"
			podman push "$prefix-$distro:latest"
		done
	done
fi
