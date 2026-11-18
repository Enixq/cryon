package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"Cryon2/internal/logging"
	"Cryon2/internal/store"
)

const jobsChangedEvent = "jobs:changed"

var errUnknownJobKind = errors.New("неизвестный тип задания")

const (
	JobKindLibraryRescan          = "library.rescan"
	JobKindRecommendationsRefresh = "recommendations.refresh"
)

func validJobKind(kind string) bool {
	return kind == JobKindLibraryRescan || kind == JobKindRecommendationsRefresh
}

// EnqueueJob persists background work before returning to the caller. The
// worker is deliberately owned by the Go engine, not a UI runtime, so jobs
// survive WebView recreation and are observable through API v1.
func (a *App) EnqueueJob(kind string, priority int, payloadJSON string) (store.Job, error) {
	if a.store == nil || a.ctx == nil {
		return store.Job{}, fmt.Errorf("хранилище недоступно")
	}
	if !validJobKind(strings.TrimSpace(kind)) {
		return store.Job{}, fmt.Errorf("%w %q", errUnknownJobKind, kind)
	}
	job, err := a.store.JobEnqueue(a.ctx, store.Job{
		Kind:        strings.TrimSpace(kind),
		Priority:    priority,
		PayloadJSON: payloadJSON,
	})
	if err == nil {
		a.platform.Emit(jobsChangedEvent)
	}
	return job, err
}

func (a *App) startJobWorker(ctx context.Context) {
	if a.store == nil || ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	a.workerCancel = cancel
	a.workerDone = make(chan struct{})
	go func() {
		defer close(a.workerDone)
		defer a.recoverGoroutine("JobWorker")
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		if locker, ok := a.store.(interface {
			AcquireJobWorker(context.Context) (func(), bool, error)
		}); ok {
			for {
				if ctx.Err() != nil {
					return
				}
				release, acquired, err := locker.AcquireJobWorker(ctx)
				if err != nil && ctx.Err() == nil {
					logging.L().Error("не удалось запустить обработчик заданий", "err", err)
					return
				}
				if err == nil && acquired {
					defer release()
					if a.platform != nil {
						a.platform.Emit(jobsChangedEvent)
					}
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}
		for {
			if ctx.Err() != nil {
				return
			}
			if !a.runOneJob(ctx) {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}
	}()
}

func (a *App) runOneJob(ctx context.Context) bool {
	if a.store == nil {
		return false
	}
	job, found, err := a.store.JobClaimNext(ctx, time.Now().UnixMilli())
	if err != nil {
		return false
	}
	if !found {
		return false
	}
	a.platform.Emit(jobsChangedEvent)
	err = a.executeJob(ctx, job)
	if err == nil {
		_ = a.store.JobFinish(ctx, job.ID, store.JobCompleted, "Готово", "")
		a.platform.Emit(jobsChangedEvent)
		return true
	}
	state := store.JobFailed
	message := "Задание не выполнено"
	if job.Attempt < job.MaxAttempts && !errors.Is(err, errUnknownJobKind) {
		state = store.JobRetrying
		message = "Повторяем после ошибки"
	}
	_ = a.store.JobFinish(ctx, job.ID, state, message, err.Error())
	a.platform.Emit(jobsChangedEvent)
	return true
}

func (a *App) executeJob(ctx context.Context, job store.Job) error {
	if err := a.store.JobProgress(ctx, job.ID, 0.05, "Запуск"); err != nil {
		return err
	}
	switch job.Kind {
	case JobKindLibraryRescan:
		if err := a.store.JobProgress(ctx, job.ID, 0.2, "Сканируем локальную библиотеку"); err != nil {
			return err
		}
		if _, err := a.RescanLocalLibrary(); err != nil {
			return err
		}
		return a.store.JobProgress(ctx, job.ID, 0.95, "Обновляем библиотеку")
	case JobKindRecommendationsRefresh:
		if err := a.store.JobProgress(ctx, job.ID, 0.2, "Обновляем рекомендации"); err != nil {
			return err
		}
		if _, err := a.ListRecommendations(40); err != nil {
			return err
		}
		return a.store.JobProgress(ctx, job.ID, 0.95, "Подборки обновлены")
	default:
		return fmt.Errorf("%w %q", errUnknownJobKind, job.Kind)
	}
}
