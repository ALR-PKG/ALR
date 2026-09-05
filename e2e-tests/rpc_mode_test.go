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

//go:build e2e

package e2etests_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.alt-gnome.ru/capytest"
)

// mockSitePackage — пакет в ответах mock-сервера сайта
type mockSitePackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Release int    `json:"release"`
	Epoch   int    `json:"epoch"`
	Repo    string `json:"repo"`
	Desc    string `json:"desc"`
}

var mockSiteFixture = []mockSitePackage{
	{Name: "rpc-mock-pkg", Version: "9.9", Release: 1, Repo: "alr-repo", Desc: "Package served only via RPC mock"},
}

// startMockSite поднимает mock RPC-сервера сайта на всех интерфейсах хоста
// и возвращает URL, по которому он доступен изнутри podman-контейнера
func startMockSite(t *testing.T) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/rpc/v1/search", func(w http.ResponseWriter, r *http.Request) {
		arg := r.URL.Query().Get("arg")
		var results []mockSitePackage
		for _, p := range mockSiteFixture {
			if strings.Contains(p.Name, arg) || strings.Contains(p.Desc, arg) {
				results = append(results, p)
			}
		}
		writeRPCResponse(w, map[string]any{
			"version": 1, "resultcount": len(results), "results": results,
		})
	})
	mux.HandleFunc("/rpc/v1/info", func(w http.ResponseWriter, r *http.Request) {
		var results []mockSitePackage
		for _, arg := range r.URL.Query()["arg"] {
			name := arg
			if i := strings.Index(arg, "+"); i > 0 {
				name = arg[:i]
			}
			for _, p := range mockSiteFixture {
				if p.Name == name {
					results = append(results, p)
				}
			}
		}
		writeRPCResponse(w, map[string]any{
			"version": 1, "resultcount": len(results), "results": results,
		})
	})
	mux.HandleFunc("/rpc/v1/count", func(w http.ResponseWriter, r *http.Request) {
		writeRPCResponse(w, map[string]any{"version": 1, "count": len(mockSiteFixture)})
	})
	mux.HandleFunc("/rpc/v1/repos", func(w http.ResponseWriter, r *http.Request) {
		writeRPCResponse(w, map[string]any{
			"version":      1,
			"repositories": []map[string]string{{"name": "alr-repo", "url": REPO_URL_FOR_E2E_TESTS}},
		})
	})

	// httptest.NewServer слушает только 127.0.0.1 — недоступен из контейнера,
	// поэтому слушаем на всех интерфейсах
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("не удалось открыть порт для mock-сервера: %v", err)
	}
	srv := httptest.NewUnstartedServer(mux)
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)

	port := listener.Addr().(*net.TCPAddr).Port
	return fmt.Sprintf("http://host.containers.internal:%d", port)
}

func writeRPCResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestE2ERPCMode(t *testing.T) {
	siteURL := startMockSite(t)

	runMatrixSuite(
		t,
		"rpc-mode",
		COMMON_SYSTEMS,
		func(t *testing.T, r capytest.Runner) {
			defaultPrepare(t, r)

			// Направляем RPC на mock-сервер и включаем RPC обратно
			for _, c := range [][]string{
				{"sudo", "alr", "config", "set", "site.url", siteURL},
				{"alr", "config", "set", "site.url", siteURL},
				{"sudo", "alr", "config", "set", "site.disableRPC", "false"},
				{"alr", "config", "set", "site.disableRPC", "false"},
			} {
				execShouldNoError(t, r, c[0], c[1:]...)
			}

			// Пакет существует только на mock-сервере: успешный поиск
			// доказывает, что запрос пошёл через RPC
			r.Command("sudo", "alr", "search", "-n", "rpc-mock-pkg").
				ExpectSuccess().
				ExpectStdoutContains("rpc-mock-pkg").
				Run(t)

			// Fallback: сайт недоступен — поиск обязан работать через
			// локальную БД (git-репозиторий)
			execShouldNoError(t, r, "sudo", "alr", "config", "set", "site.url", "http://host.containers.internal:1")
			r.Command("sudo", "alr", "search", "-n", "foo-pkg").
				ExpectSuccess().
				ExpectStdoutContains("foo-pkg").
				Run(t)
		},
	)
}
