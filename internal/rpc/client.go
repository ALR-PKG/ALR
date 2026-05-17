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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"git.alr-pkg.ru/Plemya-x/ALR/pkg/distro"
	"git.alr-pkg.ru/Plemya-x/ALR/pkg/types"
)

// Client — HTTP клиент для ALR Site RPC API
type Client struct {
	baseURL string
	client  *http.Client
}

// NewClient создаёт новый RPC клиент
func NewClient(site types.Site) *Client {
	timeout := time.Duration(site.Timeout) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	return &Client{
		baseURL: site.URL,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// get выполняет HTTP GET запрос и декодирует JSON ответ
func (c *Client) get(ctx context.Context, endpoint string, query url.Values, result any) error {
	u, err := url.Parse(c.baseURL + endpoint)
	if err != nil {
		return fmt.Errorf("не удалось распарсить URL: %w", err)
	}
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("не удалось создать запрос: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("RPC запрос не удался: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("RPC вернул статус %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("не удалось декодировать ответ: %w", err)
	}

	return nil
}

// post выполняет HTTP POST запрос с JSON телом и декодирует ответ
func (c *Client) post(ctx context.Context, endpoint string, body any, result any) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("не удалось сериализовать тело запроса: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("не удалось создать запрос: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("RPC запрос не удался: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("RPC вернул статус %d: %s", resp.StatusCode, string(bodyBytes))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("не удалось декодировать ответ: %w", err)
	}

	return nil
}

// GetPackages получает метаданные пакетов по именам
func (c *Client) GetPackages(ctx context.Context, names []string) ([]Package, error) {
	if len(names) == 0 {
		return nil, nil
	}

	query := url.Values{}
	for _, name := range names {
		query.Add("arg", name)
	}

	var resp InfoResponse
	if err := c.get(ctx, "/rpc/v1/info", query, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

// SearchPackages выполняет поиск пакетов
func (c *Client) SearchPackages(ctx context.Context, by, query string) ([]Package, error) {
	q := url.Values{}
	q.Set("by", by)
	q.Set("arg", query)

	var resp SearchResponse
	if err := c.get(ctx, "/rpc/v1/search", q, &resp); err != nil {
		return nil, err
	}

	return resp.Results, nil
}

// ListPackages получает список пакетов с пагинацией
func (c *Client) ListPackages(ctx context.Context, repo string, page, limit int) ([]Package, int, error) {
	q := url.Values{}
	if repo != "" {
		q.Set("repo", repo)
	}
	q.Set("page", fmt.Sprintf("%d", page))
	q.Set("limit", fmt.Sprintf("%d", limit))

	var resp ListResponse
	if err := c.get(ctx, "/rpc/v1/list", q, &resp); err != nil {
		return nil, 0, err
	}

	return resp.Results, resp.Total, nil
}

// CountPackages получает количество пакетов
func (c *Client) CountPackages(ctx context.Context, by, query string) (int, error) {
	q := url.Values{}
	q.Set("by", by)
	q.Set("arg", query)

	var resp CountResponse
	if err := c.get(ctx, "/rpc/v1/count", q, &resp); err != nil {
		return 0, err
	}

	return resp.Count, nil
}

// ListAllPackages загружает все пакеты через пагинацию
func (c *Client) ListAllPackages(ctx context.Context, repo string) ([]Package, error) {
	const pageSize = 200
	var all []Package
	page := 1

	for {
		pkgs, total, err := c.ListPackages(ctx, repo, page, pageSize)
		if err != nil {
			return nil, err
		}
		all = append(all, pkgs...)
		if len(all) >= total {
			break
		}
		page++
	}

	return all, nil
}

// SearchAllPackages загружает все пакеты по поиску через пагинацию
func (c *Client) SearchAllPackages(ctx context.Context, by, query string) ([]Package, error) {
	const pageSize = 200
	var all []Package
	page := 1

	for {
		q := url.Values{}
		q.Set("by", by)
		q.Set("arg", query)
		q.Set("page", fmt.Sprintf("%d", page))
		q.Set("limit", fmt.Sprintf("%d", pageSize))

		var resp SearchResponse
		if err := c.get(ctx, "/rpc/v1/search", q, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Results...)
		if len(resp.Results) < pageSize {
			break
		}
		page++
	}

	return all, nil
}

// CheckUpdates проверяет доступные обновления для установленных пакетов
func (c *Client) CheckUpdates(ctx context.Context, installed []InstalledPackage, distroInfo *distro.OSRelease) ([]UpdateInfo, error) {
	if len(installed) == 0 {
		return nil, nil
	}

	req := UpdatesRequest{
		Packages: installed,
	}

	if distroInfo != nil {
		req.Distro = &DistroInfo{
			ID:        distroInfo.ID,
			VersionID: distroInfo.VersionID,
		}
	}

	var resp UpdatesResponse
	if err := c.post(ctx, "/rpc/v1/updates", req, &resp); err != nil {
		return nil, err
	}

	return resp.Updates, nil
}
