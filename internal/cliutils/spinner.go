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

package cliutils

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// spinnerFrames — кадры анимации спиннера
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner показывает анимированную строку состояния для длительных операций
// с заранее неизвестным числом шагов. Одна строка в stderr перерисовывается
// по тикеру, Stop стирает её. Stop можно вызвать повторно и позже запустить
// спиннер заново через Start (например, на время интерактивного меню)
type Spinner struct {
	message  func() string
	mu       sync.Mutex
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once
	started  atomic.Bool
}

// NewSpinner создаёт спиннер; message вызывается на каждом кадре,
// поэтому должна быть потокобезопасной (атомики, мьютексы)
func NewSpinner(message func() string) *Spinner {
	return &Spinner{
		message: message,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

// Start запускает анимацию; повторный Start при запущенном спиннере — no-op
func (s *Spinner) Start() {
	if !s.started.CompareAndSwap(false, true) {
		return
	}

	go func() {
		defer close(s.doneCh)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		frame := 0
		for {
			select {
			case <-s.stopCh:
				// Стираем строку спиннера
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r\033[K%s %s", spinnerFrames[frame%len(spinnerFrames)], s.message())
				frame++
			}
		}
	}()
}

// Stop останавливает анимацию и стирает строку; безопасен при повторных
// вызовах и после Stop спиннер можно запустить снова через Start
func (s *Spinner) Stop() {
	if !s.started.Load() {
		return
	}

	s.stopOnce.Do(func() {
		close(s.stopCh)
		<-s.doneCh
	})

	// Сбрасываем состояние для возможного перезапуска
	s.mu.Lock()
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.stopOnce = sync.Once{}
	s.mu.Unlock()
	s.started.Store(false)
}
