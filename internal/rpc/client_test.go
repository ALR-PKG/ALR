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

package rpc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(types.Site{URL: srv.URL, Timeout: 5}), srv
}

func TestGetPackages(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/rpc/v1/info", r.URL.Path)
		assert.Equal(t, []string{"foo", "bar"}, r.URL.Query()["arg"])
		w.Write([]byte(`{"version":1,"resultcount":1,"results":[{"name":"foo","version":"1.0","repo":"alr-repo"}]}`))
	}))

	pkgs, err := client.GetPackages(context.Background(), []string{"foo", "bar"})
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	assert.Equal(t, "foo", pkgs[0].Name)
}

func TestGetPackagesErrorStatus(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"boom"}`))
	}))

	_, err := client.GetPackages(context.Background(), []string{"foo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestGetPackagesInvalidJSON(t *testing.T) {
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`not json`))
	}))

	_, err := client.GetPackages(context.Background(), []string{"foo"})
	require.Error(t, err)
}

func TestListAllPackagesPagination(t *testing.T) {
	// Сервер отдаёт 2 страницы по 200 и 50 пакетов
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		var results string
		var total int
		switch page {
		case "1":
			results = genPackages(200)
			total = 250
		case "2":
			results = genPackages(50)
			total = 250
		default:
			results = "[]"
			total = 250
		}
		fmt.Fprintf(w, `{"version":1,"resultcount":0,"total":%d,"page":%s,"limit":200,"results":%s}`, total, page, results)
	}))

	pkgs, err := client.ListAllPackages(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, pkgs, 250)
}

func TestListAllPackagesStopsOnEmptyPage(t *testing.T) {
	// «Лживый» сервер: total недостижим, но пустая страница останавливает цикл
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "1" {
			fmt.Fprintf(w, `{"version":1,"resultcount":0,"total":999999,"page":1,"limit":200,"results":%s}`, genPackages(200))
			return
		}
		fmt.Fprint(w, `{"version":1,"resultcount":0,"total":999999,"page":2,"limit":200,"results":[]}`)
	}))

	pkgs, err := client.ListAllPackages(context.Background(), "")
	require.NoError(t, err)
	assert.Len(t, pkgs, 200)
}

func genPackages(n int) string {
	out := "["
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf(`{"name":"pkg%d","version":"1.0","repo":"r"}`, i)
	}
	return out + "]"
}
