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

package dbus

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"
	"github.com/godbus/dbus/v5/prop"
)

// DBusJob реализует ru.alr-pkg.ALR.Job интерфейс
type DBusJob struct {
	service *Service
	info    JobInfo

	// Каналы для синхронизации
	done   chan struct{}
	cancel context.CancelFunc

	// D-Bus properties
	properties *prop.Properties

	// Mutex для потокобезопасности
	mu sync.RWMutex
}

// NewDBusJob создает новую задачу
func NewDBusJob(service *Service, id uint32, jobType JobType, pkgName, repo string) *DBusJob {
	job := &DBusJob{
		service: service,
		info:    NewJobInfo(id, jobType, pkgName, repo),
		done:    make(chan struct{}),
	}

	// Инициализация properties
	path := GetJobObjectPath(id)
	propsSpec := map[string]map[string]*prop.Prop{
		JobInterfaceName: {
			"Id": {
				Value:    id,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Type": {
				Value:    string(jobType),
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Status": {
				Value:    string(JobStatusPending),
				Writable: true,
				Emit:     prop.EmitTrue,
			},
			"Progress": {
				Value:    0.0,
				Writable: true,
				Emit:     prop.EmitTrue,
			},
			"PackageName": {
				Value:    pkgName,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Repository": {
				Value:    repo,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Message": {
				Value:    "",
				Writable: true,
				Emit:     prop.EmitTrue,
			},
			"ErrorMessage": {
				Value:    "",
				Writable: true,
				Emit:     prop.EmitTrue,
			},
			"CreatedAt": {
				Value:    job.info.CreatedAt,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"CompletedAt": {
				Value:    int64(0),
				Writable: true,
				Emit:     prop.EmitTrue,
			},
		},
	}

	if conn := service.GetConn(); conn != nil {
		job.properties = prop.New(conn, path, propsSpec)
	}

	return job
}

// setProp обновляет D-Bus property, если properties инициализированы
func (j *DBusJob) setProp(name string, value dbus.Variant) {
	if j.properties != nil {
		j.setProp(name, value)
	}
}

// Cancel отменяет задачу
func (j *DBusJob) Cancel() *dbus.Error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.info.Status == string(JobStatusCompleted) || j.info.Status == string(JobStatusFailed) {
		return dbus.NewError("ru.alr-pkg.ALR.Error.JobAlreadyFinished", []interface{}{"job already finished"})
	}

	j.info.Status = string(JobStatusCancelled)
	j.info.CompletedAt = time.Now().Unix()

	// Обновляем properties
	j.setProp("Status", dbus.MakeVariant(string(JobStatusCancelled)))
	j.setProp("CompletedAt", dbus.MakeVariant(j.info.CompletedAt))

	// Отправляем сигналы
	j.emitSignal(JobSignalStatusChanged, string(JobStatusCancelled))
	j.emitSignal(JobSignalCompleted, false, "cancelled by user")

	// Вызываем cancel функцию если есть
	if j.cancel != nil {
		j.cancel()
	}

	// Закрываем канал done
	close(j.done)

	// Удаляем из реестра через некоторое время
	go j.scheduleCleanup()

	return nil
}

// Wait ожидает завершения задачи
func (j *DBusJob) Wait(timeout int32) (bool, string, *dbus.Error) {
	var ctx context.Context
	var cancel context.CancelFunc

	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()

	select {
	case <-j.done:
		j.mu.RLock()
		success := j.info.Status == string(JobStatusCompleted)
		errMsg := j.info.ErrorMessage
		j.mu.RUnlock()
		return success, errMsg, nil
	case <-ctx.Done():
		return false, "timeout", nil
	}
}

// GetInfo возвращает информацию о задаче
func (j *DBusJob) GetInfo() (JobInfo, *dbus.Error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.info, nil
}

// SetStatus устанавливает статус задачи
func (j *DBusJob) SetStatus(status JobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.info.Status = string(status)
	j.setProp("Status", dbus.MakeVariant(string(status)))
	j.emitSignal(JobSignalStatusChanged, string(status))
}

// SetProgress устанавливает прогресс
func (j *DBusJob) SetProgress(progress float64, message string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.info.Progress = progress
	j.info.Message = message

	j.setProp("Progress", dbus.MakeVariant(progress))
	j.setProp("Message", dbus.MakeVariant(message))

	j.emitSignal(JobSignalProgressChanged, progress, message)
}

// SetCompleted помечает задачу как завершенную
func (j *DBusJob) SetCompleted() {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.info.Status = string(JobStatusCompleted)
	j.info.Progress = 1.0
	j.info.CompletedAt = time.Now().Unix()

	j.setProp("Status", dbus.MakeVariant(string(JobStatusCompleted)))
	j.setProp("Progress", dbus.MakeVariant(1.0))
	j.setProp("CompletedAt", dbus.MakeVariant(j.info.CompletedAt))

	j.emitSignal(JobSignalStatusChanged, string(JobStatusCompleted))
	j.emitSignal(JobSignalCompleted, true, "")

	// Закрываем канал
	close(j.done)

	// Удаляем из реестра через некоторое время
	go j.scheduleCleanup()
}

// SetFailed помечает задачу как проваленную
func (j *DBusJob) SetFailed(errorMessage string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.info.Status = string(JobStatusFailed)
	j.info.ErrorMessage = errorMessage
	j.info.CompletedAt = time.Now().Unix()

	j.setProp("Status", dbus.MakeVariant(string(JobStatusFailed)))
	j.setProp("ErrorMessage", dbus.MakeVariant(errorMessage))
	j.setProp("CompletedAt", dbus.MakeVariant(j.info.CompletedAt))

	j.emitSignal(JobSignalStatusChanged, string(JobStatusFailed))
	j.emitSignal(JobSignalCompleted, false, errorMessage)

	// Закрываем канал
	close(j.done)

	// Удаляем из реестра через некоторое время
	go j.scheduleCleanup()
}

// SetContext устанавливает контекст с отменой
func (j *DBusJob) SetContext(ctx context.Context, cancel context.CancelFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cancel = cancel
}

// GetContext возвращает контекст задачи
func (j *DBusJob) GetContext() context.Context {
	return j.service.Context()
}

// emitSignal отправляет сигнал
func (j *DBusJob) emitSignal(name string, values ...interface{}) {
	path := GetJobObjectPath(j.info.ID)
	j.service.EmitSignal(path, JobInterfaceName, name, values...)
}

// scheduleCleanup планирует удаление задачи из реестра
func (j *DBusJob) scheduleCleanup() {
	// Ждем 5 минут перед удалением
	time.Sleep(5 * time.Minute)

	path := GetJobObjectPath(j.info.ID)
	j.service.UnregisterJob(path)
}

// IntrospectionData возвращает данные для introspection
func (j *DBusJob) IntrospectionData() introspect.Interface {
	return introspect.Interface{
		Name: JobInterfaceName,
		Methods: []introspect.Method{
			{
				Name: "Cancel",
				Args: []introspect.Arg{
					{Name: "error", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "Wait",
				Args: []introspect.Arg{
					{Name: "timeout", Type: "i", Direction: "in"},
					{Name: "success", Type: "b", Direction: "out"},
					{Name: "error_message", Type: "s", Direction: "out"},
				},
			},
			{
				Name: "GetInfo",
				Args: []introspect.Arg{
					{Name: "info", Type: "(usssddsstt)", Direction: "out"},
				},
			},
		},
		Signals: []introspect.Signal{
			{
				Name: "ProgressChanged",
				Args: []introspect.Arg{
					{Name: "progress", Type: "d"},
					{Name: "message", Type: "s"},
				},
			},
			{
				Name: "StatusChanged",
				Args: []introspect.Arg{
					{Name: "status", Type: "s"},
				},
			},
			{
				Name: "Completed",
				Args: []introspect.Arg{
					{Name: "success", Type: "b"},
					{Name: "error", Type: "s"},
				},
			},
		},
		Properties: []introspect.Property{
			{Name: "Id", Type: "u", Access: "read"},
			{Name: "Type", Type: "s", Access: "read"},
			{Name: "Status", Type: "s", Access: "read"},
			{Name: "Progress", Type: "d", Access: "read"},
			{Name: "PackageName", Type: "s", Access: "read"},
			{Name: "Repository", Type: "s", Access: "read"},
			{Name: "Message", Type: "s", Access: "read"},
			{Name: "ErrorMessage", Type: "s", Access: "read"},
			{Name: "CreatedAt", Type: "t", Access: "read"},
			{Name: "CompletedAt", Type: "t", Access: "read"},
		},
	}
}
