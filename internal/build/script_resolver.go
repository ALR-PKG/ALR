// ALR - Any Linux Repository
// Copyright (C) 2025 The ALR Authors
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

package build

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

// maxDownloadSize — максимальный размер файла, скачиваемого с сайта
const maxDownloadSize = 10 << 20

type ScriptResolver struct {
	cfg Config
}

type ScriptInfo struct {
	Script     string
	Repository string
}

func (s *ScriptResolver) ResolveScript(
	ctx context.Context,
	pkg *alrsh.Package,
) *ScriptInfo {
	var repository, script string

	repodir := s.cfg.GetPaths().RepoDir
	repository = pkg.Repository

	// First, we check if there is a root alr.sh in the repository
	rootScriptPath, err := safeJoin(repodir, repository, "alr.sh")
	if err != nil {
		slog.Warn("ScriptResolver: небезопасный путь репозитория", "repo", repository, "error", err)
		return &ScriptInfo{Repository: repository}
	}
	if _, err := os.Stat(rootScriptPath); err == nil {
		// A repository with a single alr.sh at the root
		script = rootScriptPath
	} else {
		// Multi-package repository - we are looking for alr.sh in the subfolder
		pkgDir := pkg.BasePkgName
		if pkgDir == "" {
			pkgDir = pkg.Name
		}
		scriptPath, err := safeJoin(repodir, repository, pkgDir, "alr.sh")
		if err != nil {
			slog.Warn("ScriptResolver: небезопасный путь пакета", "pkg", pkg.Name, "error", err)
			return &ScriptInfo{Repository: repository}
		}
		script = scriptPath
	}

	// Скачиваем или обновляем скрипт по прямой ссылке через RPC
	if pkg.AlrShURL != "" {
		if err := s.validateDownloadURL(pkg.AlrShURL); err != nil {
			slog.Warn("ScriptResolver: отказ в скачивании alr.sh", "pkg", pkg.Name, "url", pkg.AlrShURL, "error", err)
			return &ScriptInfo{Repository: repository, Script: script}
		}
		if needsDownload(script, pkg.AlrShSHA256) {
			slog.Debug("ScriptResolver: скачиваем alr.sh по URL", "pkg", pkg.Name, "url", pkg.AlrShURL)
			if err := s.downloadFile(ctx, pkg.AlrShURL, script, pkg.AlrShSHA256); err == nil {
				slog.Debug("ScriptResolver: alr.sh успешно скачан", "pkg", pkg.Name, "path", script)
			} else {
				slog.Warn("ScriptResolver: не удалось скачать alr.sh, будет использован fallback на git", "pkg", pkg.Name, "error", err)
			}
		}
		// Скачиваем или обновляем дополнительные файлы из директории пакета (кроме alr.sh)
		for _, ef := range pkg.ExtraFiles {
			if ef.Name == "alr.sh" {
				continue
			}
			if err := s.validateDownloadURL(ef.DownloadURL); err != nil {
				slog.Warn("ScriptResolver: отказ в скачивании доп. файла", "pkg", pkg.Name, "file", ef.Name, "error", err)
				continue
			}
			destPath, err := safeJoin(repodir, repository, filepath.Dir(ef.Path), ef.Name)
			if err != nil {
				slog.Warn("ScriptResolver: небезопасный путь доп. файла", "pkg", pkg.Name, "file", ef.Name, "error", err)
				continue
			}
			if !needsDownload(destPath, ef.SHA256) {
				continue
			}
			if err := s.downloadFile(ctx, ef.DownloadURL, destPath, ef.SHA256); err != nil {
				slog.Warn("ScriptResolver: не удалось скачать доп. файл", "pkg", pkg.Name, "file", ef.Name, "error", err)
			} else {
				slog.Debug("ScriptResolver: доп. файл скачан", "pkg", pkg.Name, "file", ef.Name)
			}
		}
	}

	return &ScriptInfo{
		Repository: repository,
		Script:     script,
	}
}

// validateDownloadURL проверяет, что URL скачивания указывает на доверенный
// хост (site.url или одно из зеркал) и использует ту же схему
func (s *ScriptResolver) validateDownloadURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("некорректный URL: %q", rawURL)
	}

	site := s.cfg.Site()
	allowed := append([]string{site.URL}, site.Mirrors...)
	for _, repo := range s.cfg.Repos() {
		allowed = append(allowed, repo.URL)
		allowed = append(allowed, repo.Mirrors...)
	}
	for _, base := range allowed {
		bu, err := url.Parse(base)
		if err != nil || bu.Host == "" {
			continue
		}
		if u.Scheme != bu.Scheme {
			continue
		}
		if u.Host == bu.Host {
			return nil
		}
		// Субдомены доверенного хоста (например git.alr-pkg.ru для
		// alr-pkg.ru): сайт отдаёт alr_sh_url на raw-файлы хостинга
		// репозиториев внутри своего домена
		if strings.HasSuffix(u.Host, "."+bu.Host) {
			return nil
		}
	}

	return fmt.Errorf("URL %q не относится к доверенным хостам", rawURL)
}

// safeJoin объединяет элементы пути с base, отклоняя абсолютные пути
// и выход за пределы base через «..»
func safeJoin(base string, elems ...string) (string, error) {
	for _, e := range elems {
		if e == "" || filepath.IsAbs(e) {
			return "", fmt.Errorf("некорректный элемент пути: %q", e)
		}
		for _, part := range strings.Split(filepath.ToSlash(e), "/") {
			if part == ".." {
				return "", fmt.Errorf("недопустимый элемент «..» в пути: %q", e)
			}
		}
	}

	joined := filepath.Join(append([]string{base}, elems...)...)
	rel, err := filepath.Rel(base, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("путь %q выходит за пределы %q", joined, base)
	}

	return joined, nil
}

// needsDownload определяет, нужно ли скачивать файл: он отсутствует
// локально, либо задан ожидаемый sha256 и хеш локального файла не совпадает.
// Если ожидаемый хеш не задан, существующий файл считается актуальным
func needsDownload(path, expectedSHA256 string) bool {
	if _, err := os.Stat(path); err != nil {
		return true
	}
	if expectedSHA256 == "" {
		return false
	}
	return verifySHA256(path, expectedSHA256) != nil
}

// downloadFile скачивает файл по HTTP и сохраняет по указанному пути.
// Если expectedSHA256 не пустой, содержимое проверяется по хешу до установки на место
func (s *ScriptResolver) downloadFile(ctx context.Context, url, destPath, expectedSHA256 string) error {
	// Создаём директорию если нужно
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("не удалось создать директорию %s: %w", dir, err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("не удалось создать запрос: %w", err)
	}
	req.Header.Set("User-Agent", "ALR")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP запрос не удался: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Пишем во временный файл, затем переименовываем для атомарности
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("не удалось создать файл: %w", err)
	}

	n, err := io.Copy(f, io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("не удалось записать файл: %w", err)
	}
	if n > maxDownloadSize {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("файл превышает максимальный размер %d байт", maxDownloadSize)
	}
	f.Close()

	if expectedSHA256 != "" {
		if err := verifySHA256(tmpPath, expectedSHA256); err != nil {
			os.Remove(tmpPath)
			return err
		}
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("не удалось переименовать файл: %w", err)
	}

	return nil
}

// verifySHA256 сравнивает sha256 файла с ожидаемым хешом
func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("не удалось открыть файл для проверки хеша: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("не удалось вычислить хеш: %w", err)
	}

	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("хеш скачанного файла не совпадает: ожидался %s, получен %s", expected, actual)
	}
	return nil
}
