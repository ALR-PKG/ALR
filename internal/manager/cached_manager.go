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

package manager

import (
	"log/slog"
	"sync"
)

// CachedManager оборачивает Manager и кэширует результаты
type CachedManager struct {
	Manager
	db          DBCacheInterface
	cacheMu     sync.RWMutex
	isAvailHit  int
	isAvailMiss int
}

// DBCacheInterface интерфейс для работы с кэшем в БД
type DBCacheInterface interface {
	GetPackageAvailability(name, manager string) (isAvailable, found bool)
	SetPackageAvailability(name, manager string, isAvailable bool) error
}

// NewCachedManager создаёт новый CachedManager
func NewCachedManager(m Manager, db DBCacheInterface) *CachedManager {
	return &CachedManager{
		Manager: m,
		db:      db,
	}
}

// ListAvailable возвращает список доступных пакетов с кэшированием (оставляем для обратной совместимости)
func (c *CachedManager) ListAvailable(prefix string) ([]string, error) {
	return c.Manager.ListAvailable(prefix)
}

// IsAvailable проверяет, доступен ли пакет, с кэшированием в БД
func (c *CachedManager) IsAvailable(name string) (bool, error) {
	// Сначала проверяем кэш в БД
	if c.db != nil {
		if isAvail, found := c.db.GetPackageAvailability(name, c.Manager.Name()); found {
			c.cacheMu.Lock()
			c.isAvailHit++
			c.cacheMu.Unlock()
			slog.Debug("IsAvailable cache hit", "pkg", name, "manager", c.Manager.Name(), "available", isAvail)
			return isAvail, nil
		}
	}

	// Кэш промах - вызываем реальный метод
	isAvail, err := c.Manager.IsAvailable(name)
	if err != nil {
		return false, err
	}

	// Сохраняем в кэш БД
	if c.db != nil {
		if err := c.db.SetPackageAvailability(name, c.Manager.Name(), isAvail); err != nil {
			slog.Warn("Failed to cache package availability", "pkg", name, "error", err)
		}
	}

	c.cacheMu.Lock()
	c.isAvailMiss++
	c.cacheMu.Unlock()

	slog.Debug("IsAvailable cache miss", "pkg", name, "manager", c.Manager.Name(), "available", isAvail)

	return isAvail, nil
}

// GetCacheStats возвращает статистику кэша
func (c *CachedManager) GetCacheStats() (hits, misses int) {
	c.cacheMu.RLock()
	defer c.cacheMu.RUnlock()
	return c.isAvailHit, c.isAvailMiss
}
