// This file was originally part of the project "LURE - Linux User REpository", created by Elara Musayelyan.
// It has been modified as part of "ALR - Any Linux Repository" by the ALR Authors.
//
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

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"git.alr-pkg.ru/ALR/ALR/cmd"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/logger"
	"git.alr-pkg.ru/ALR/ALR/internal/translations"
	"github.com/mattn/go-isatty"
)

func setLogLevel(newLevel string) {
	level := slog.LevelInfo
	switch strings.ToUpper(newLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "INFO":
		level = slog.LevelInfo
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		if newLevel != "" {
			slog.Warn("unknown logLevel value, falling back to INFO", "value", newLevel)
		}
	}
	if l, ok := slog.Default().Handler().(*logger.Logger); ok {
		l.SetLevel(level)
	}
}

func main() {
	translations.Setup()
	logger.SetupDefault()

	cfg := config.New()
	if err := cfg.Load(); err != nil {
		slog.Error("Error loading config", "err", err)
		os.Exit(1)
	}
	setLogLevel(cfg.LogLevel())
	if envLevel := os.Getenv("ALR_LOG_LEVEL"); envLevel != "" {
		setLogLevel(envLevel)
	}
	if os.Getenv("ALR_LOG_LEVEL") == "" && !isatty.IsTerminal(os.Stdout.Fd()) {
		setLogLevel("WARN")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	root := cmd.NewRootCmd()
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)

	if err := root.ExecuteContext(ctx); err != nil {
		slog.Error("Error while running app", "err", err)
		os.Exit(1)
	}
}
