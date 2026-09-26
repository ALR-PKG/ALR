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

package db

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"github.com/leonelquinteros/gotext"
	_ "modernc.org/sqlite"
	"xorm.io/xorm"

	"git.alr-pkg.ru/ALR/ALR/internal/cliutils"
	"git.alr-pkg.ru/ALR/ALR/internal/config"
	"git.alr-pkg.ru/ALR/ALR/internal/constants"
	"git.alr-pkg.ru/ALR/ALR/internal/fsutils"
	"git.alr-pkg.ru/ALR/ALR/pkg/alrsh"
)

const CurrentVersion = 8

type Version struct {
	Version int `xorm:"'version'"`
}

// PackageAvailabilityCache хранит информацию о доступности пакетов в репозиториях
type PackageAvailabilityCache struct {
	Name        string `xorm:"'name' pk"`
	Manager     string `xorm:"'manager' pk"` // dnf, apt, pacman и т.д.
	IsAvailable bool   `xorm:"'is_available'"`
	CheckedAt   int64  `xorm:"'checked_at'"`
}

type Config interface {
	GetPaths() *config.Paths
}

type Database struct {
	engine *xorm.Engine
	config Config
}

func New(config Config) *Database {
	return &Database{
		config: config,
	}
}

func (d *Database) Connect() error {
	dbPath := d.config.GetPaths().DBPath

	// Проверяем директорию для БД
	dbDir := filepath.Dir(dbPath)
	if _, err := os.Stat(dbDir); err != nil {
		if os.IsNotExist(err) {
			// Директория не существует - создаем автоматически
			slog.Info(gotext.Get("Cache directory does not exist, creating it"))
			if err := fsutils.EnsureTempDirWithRootOwner(dbDir, 0o2775); err != nil {
				return fmt.Errorf("failed to create cache directory: %w", err)
			}
		} else {
			return fmt.Errorf("failed to check database directory: %w", err)
		}
	}

	// file-URI с busy_timeout=5с: при конкурентном доступе нескольких процессов
	// SQLite ждёт освобождения БД вместо мгновенного "database is locked".
	// Ограничение: путь к БД не должен содержать '?' или '#' (ломают file-URI),
	// для реальных путей (/var/cache/alr, ~/.cache/alr) это неактуально.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", dbPath)

	engine, err := xorm.NewEngine("sqlite", dsn)
	// engine.SetLogLevel(log.LOG_DEBUG)
	// engine.ShowSQL(true)
	if err != nil {
		return err
	}
	d.engine = engine

	// Общая БД обязана быть доступной на запись всей группе кэша: файл,
	// созданный любым пользователем с обычным umask, получает 644, и
	// остальные получают "attempt to write a readonly database". Каждый
	// процесс нормализует права на свой файл (root — и владельца).
	if dbPath == filepath.Join(constants.SystemCachePath, "alr.db") {
		normalizeSharedDBPerms(dbPath)
	}

	return nil
}

// normalizeSharedDBPerms выставляет общей БД права 664 и группу кэша.
// Ошибки не фатальны: процесс, не владеющий файлом, исправить его не в
// силах. chown срабатывает от root всегда, а от обычного пользователя —
// если он владелец файла и входит в группу кэша
func normalizeSharedDBPerms(dbPath string) {
	if err := os.Chmod(dbPath, 0o664); err != nil {
		slog.Debug("не удалось выставить права общей БД", "path", dbPath, "error", err)
	}
	g, err := user.LookupGroup(fsutils.CacheShareGroup())
	if err != nil {
		return
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return
	}
	if err := os.Chown(dbPath, -1, gid); err != nil {
		slog.Debug("не удалось выставить группу общей БД", "path", dbPath, "error", err)
	}
}

func (d *Database) Init(ctx context.Context) error {
	if err := d.Connect(); err != nil {
		return err
	}
	if err := d.engine.Sync2(new(alrsh.Package), new(Version), new(PackageAvailabilityCache)); err != nil {
		return err
	}
	ver, ok := d.GetVersion(ctx)
	if ok && ver != CurrentVersion {
		slog.Warn(gotext.Get("Database version mismatch; resetting"), "version", ver, "expected", CurrentVersion)
		if err := d.reset(); err != nil {
			return err
		}
		return d.Init(ctx)
	} else if !ok {
		slog.Warn(gotext.Get("Database version does not exist. Run alr fix if something isn't working."))
		return d.addVersion(CurrentVersion)
	}
	return nil
}

func (d *Database) GetVersion(ctx context.Context) (int, bool) {
	var v Version
	has, err := d.engine.Get(&v)
	if err != nil || !has {
		return 0, false
	}
	return v.Version, true
}

func (d *Database) addVersion(ver int) error {
	_, err := d.engine.Insert(&Version{Version: ver})
	return err
}

func (d *Database) reset() error {
	return d.engine.DropTables(new(alrsh.Package), new(Version), new(PackageAvailabilityCache))
}

func (d *Database) InsertPackage(ctx context.Context, pkg alrsh.Package) error {
	session := d.engine.Context(ctx)

	affected, err := session.Where("name = ? AND repository = ?", pkg.Name, pkg.Repository).Update(&pkg)
	if err != nil {
		return err
	}

	if affected == 0 {
		_, err = session.Insert(&pkg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Database) GetPkgs(_ context.Context, where string, args ...any) ([]alrsh.Package, error) {
	var pkgs []alrsh.Package
	err := d.engine.Where(where, args...).Find(&pkgs)
	return pkgs, err
}

func (d *Database) GetPkg(where string, args ...any) (*alrsh.Package, error) {
	var pkg alrsh.Package
	has, err := d.engine.Where(where, args...).Get(&pkg)
	if err != nil || !has {
		return nil, err
	}
	return &pkg, nil
}

func (d *Database) DeletePkgs(_ context.Context, where string, args ...any) error {
	_, err := d.engine.Where(where, args...).Delete(&alrsh.Package{})
	return err
}

func (d *Database) IsEmpty() bool {
	count, err := d.engine.Count(new(alrsh.Package))
	if err != nil {
		slog.Warn("Error checking if database is empty", "error", err)
		return true
	}
	return count == 0
}

// HasRepoPackages проверяет, есть ли пакеты указанного репозитория в БД
func (d *Database) HasRepoPackages(repoName string) bool {
	count, err := d.engine.Where("repository = ?", repoName).Count(new(alrsh.Package))
	if err != nil {
		slog.Warn("Error checking repository packages", "repo", repoName, "error", err)
		return false
	}
	slog.Debug("HasRepoPackages check", "repo", repoName, "count", count)
	return count > 0
}

func (d *Database) Close() error {
	return d.engine.Close()
}

// CountPkgs возвращает количество пакетов, удовлетворяющих условию
func (d *Database) CountPkgs(_ context.Context, where string, args ...any) (int64, error) {
	return d.engine.Where(where, args...).Count(new(alrsh.Package))
}

// GetMinCachedAt возвращает минимальное значение cached_at среди пакетов,
// удовлетворяющих условию по имени (или всех, если prefix пустой).
func (d *Database) GetMinCachedAt(ctx context.Context, prefix string) (int64, error) {
	session := d.engine.Context(ctx)
	sql := "SELECT MIN(cached_at) as min FROM " + d.engine.TableName(new(alrsh.Package)) + " WHERE cached_at > 0"
	var args []any
	if prefix != "" {
		sql += " AND name LIKE ? ESCAPE ?"
		args = append(args, cliutils.EscapeLikePattern(prefix)+"%", "\\")
	}

	var result struct {
		Min *int64
	}
	has, err := session.SQL(sql, args...).Get(&result)
	if err != nil {
		return 0, err
	}
	if !has || result.Min == nil {
		return 0, fmt.Errorf("no cached packages found")
	}
	return *result.Min, nil
}

// GetPackageAvailability получает информацию о доступности пакета из кэша
func (d *Database) GetPackageAvailability(name, manager string) (bool, bool) {
	var cache PackageAvailabilityCache
	has, err := d.engine.Where("name = ? AND manager = ?", name, manager).Get(&cache)
	if err != nil || !has {
		return false, false
	}
	return cache.IsAvailable, true
}

// SetPackageAvailability сохраняет информацию о доступности пакета в кэш
func (d *Database) SetPackageAvailability(name, manager string, isAvailable bool) error {
	cache := &PackageAvailabilityCache{
		Name:        name,
		Manager:     manager,
		IsAvailable: isAvailable,
		CheckedAt:   time.Now().Unix(),
	}

	// Проверяем, существует ли запись
	var existing PackageAvailabilityCache
	has, err := d.engine.Where("name = ? AND manager = ?", name, manager).Get(&existing)
	if err != nil {
		return err
	}

	if has {
		// Обновляем существующую запись
		_, err = d.engine.Where("name = ? AND manager = ?", name, manager).Update(cache)
	} else {
		// Вставляем новую запись
		_, err = d.engine.Insert(cache)
	}

	return err
}
