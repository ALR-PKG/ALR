// ALR - Any Linux Repository
// Copyright (C) 2026 The ALR Authors
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

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMain перехватывает re-exec: Wrap запускает этот же тестовый
// бинарник с подкомандой `_internal-sandbox-init -- <команда>`.
func TestMain(m *testing.M) {
	if len(os.Args) >= 2 && os.Args[1] == "_internal-sandbox-init" {
		os.Exit(RunInit(os.Args[2:]))
	}
	if len(os.Args) >= 2 && os.Args[1] == "_internal-sandbox-ll" {
		os.Exit(RunLandlockInit(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func usernsAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("unshare"); err != nil {
		t.Skip("unshare недоступен")
	}
	if out, err := exec.Command("unshare", "-U", "-m", "-p", "-f", "true").CombinedOutput(); err != nil {
		t.Skipf("userns недоступен: %v (%s)", err, out)
	}
}

func TestWrapSandboxIntegration(t *testing.T) {
	usernsAvailable(t)

	rwDir := t.TempDir()
	pkgsDir := t.TempDir()
	// ScriptDir не должна попадать внутрь rw-путей (/tmp и т.п.), иначе
	// по правилам дедупликации её ro-запись покрывается rw-родителем.
	scriptDir, err := os.MkdirTemp("/var/tmp", "alr-sandbox-script-")
	if err != nil {
		t.Skipf("/var/tmp недоступен: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(scriptDir) })

	layout := LayoutFromPaths(rwDir, pkgsDir, scriptDir)

	script := `
if touch "$RWDIR/ok" 2>/dev/null; then echo "MARK_RW_OK"; else echo "MARK_RW_FAIL"; fi
if touch "$HOME/escape" 2>/dev/null; then echo "MARK_HOME_OK"; else echo "MARK_HOME_FAIL"; fi
if cat /etc/os-release >/dev/null 2>&1; then echo "MARK_ETC_OK"; else echo "MARK_ETC_FAIL"; fi
echo "MARK_PROCPIDS $(ls /proc | grep -c '^[0-9]')"
if touch "$SCRIPTDIR/x" 2>/dev/null; then echo "MARK_SCRIPT_OK"; else echo "MARK_SCRIPT_FAIL"; fi
`

	cmd := exec.Command("/bin/sh", "-c", script)
	cmd.Env = append(os.Environ(), "RWDIR="+rwDir, "SCRIPTDIR="+scriptDir)

	if err := Wrap(cmd, layout, os.Getuid(), os.Getgid()); err != nil {
		t.Fatalf("Wrap: %v", err)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == ExitSetupFailure {
			t.Skipf("песочница не завелась на этом ядре (exit %d): %s", ExitSetupFailure, out)
		}
		t.Fatalf("запуск в песочнице: %v\n%s", err, out)
	}
	t.Logf("вывод:\n%s", out)
	output := string(out)

	if !strings.Contains(output, "MARK_RW_OK") {
		t.Error("запись в rw-путь макета должна работать")
	}
	if strings.Contains(output, "MARK_HOME_OK") {
		t.Error("запись в $HOME должна быть запрещена (EACCES)")
	}
	if !strings.Contains(output, "MARK_HOME_FAIL") {
		t.Error("ожидался отказ записи в $HOME")
	}
	if !strings.Contains(output, "MARK_ETC_OK") {
		t.Error("чтение /etc/os-release должно работать")
	}
	if strings.Contains(output, "MARK_SCRIPT_OK") {
		t.Error("запись в ScriptDir (ro) должна быть запрещена")
	}

	var n int
	if _, err := fmt.Sscanf(output[strings.Index(output, "MARK_PROCPIDS"):], "MARK_PROCPIDS %d", &n); err != nil {
		t.Fatalf("нет маркера PROCPIDS: %v", err)
	}
	hostPIDs, _ := filepath.Glob("/proc/[0-9]*")
	if n > 5 {
		t.Errorf("свежий /proc должен содержать мало pid'ов, получено %d (на хосте %d)", n, len(hostPIDs))
	}
	t.Logf("pid'ов в /proc песочницы: %d, на хосте: %s", n, strconv.Itoa(len(hostPIDs)))

	// Файл, записанный внутри песочницы, виден снаружи (bind rw).
	if _, err := os.Stat(filepath.Join(rwDir, "ok")); err != nil {
		t.Error("файл, созданный в rw-пути внутри песочницы, должен быть виден на хосте")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), "escape")); err == nil {
		t.Fatal("файл escape появился в $HOME хоста — песочница пробита")
	}
}
