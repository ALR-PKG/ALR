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
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"git.alr-pkg.ru/Plemya-x/ALR/pkg/alrsh"
)

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
	rootScriptPath := filepath.Join(repodir, repository, "alr.sh")
	if _, err := os.Stat(rootScriptPath); err == nil {
		// A repository with a single alr.sh at the root
		script = rootScriptPath
	} else {
		// Multi-package repository - we are looking for alr.sh in the subfolder
		var scriptPath string
		if pkg.BasePkgName != "" {
			scriptPath = filepath.Join(repodir, repository, pkg.BasePkgName, "alr.sh")
		} else {
			scriptPath = filepath.Join(repodir, repository, pkg.Name, "alr.sh")
		}
		script = scriptPath
	}

	// Если скрипт не существует локально, но есть прямая ссылка через RPC — скачиваем
	if _, err := os.Stat(script); err != nil && pkg.AlrShURL != "" {
		slog.Debug("ScriptResolver: alr.sh не найден локально, скачиваем по URL", "pkg", pkg.Name, "url", pkg.AlrShURL)
		if err := s.downloadFile(ctx, pkg.AlrShURL, script); err == nil {
			slog.Debug("ScriptResolver: alr.sh успешно скачан", "pkg", pkg.Name, "path", script)
			// Скачиваем дополнительные файлы из директории пакета (кроме alr.sh)
			for _, ef := range pkg.ExtraFiles {
				if ef.Name == "alr.sh" {
					continue
				}
				destPath := filepath.Join(repodir, repository, filepath.Dir(ef.Path), ef.Name)
				if err := s.downloadFile(ctx, ef.DownloadURL, destPath); err != nil {
					slog.Warn("ScriptResolver: не удалось скачать доп. файл", "pkg", pkg.Name, "file", ef.Name, "error", err)
				} else {
					slog.Debug("ScriptResolver: доп. файл скачан", "pkg", pkg.Name, "file", ef.Name)
				}
			}
		} else {
			slog.Warn("ScriptResolver: не удалось скачать alr.sh, будет использован fallback на git", "pkg", pkg.Name, "error", err)
		}
	}

	return &ScriptInfo{
		Repository: repository,
		Script:     script,
	}
}

// downloadFile скачивает файл по HTTP и сохраняет по указанному пути
func (s *ScriptResolver) downloadFile(ctx context.Context, url, destPath string) error {
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

	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("не удалось записать файл: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("не удалось переименовать файл: %w", err)
	}

	return nil
}
