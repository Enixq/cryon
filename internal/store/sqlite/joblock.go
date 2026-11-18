package sqlite

import (
	"context"
	"os"
	"time"
)

// AcquireJobWorker grants one process exclusive ownership of the job worker.
// The operating system releases the lock after a crash or process exit.
func (s *Store) AcquireJobWorker(ctx context.Context) (func(), bool, error) {
	file, err := os.OpenFile(s.dbPath+".jobs.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, false, err
	}
	if err := lockJobFile(file); err != nil {
		_ = file.Close()
		if jobLockBusy(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	release := func() { _ = unlockJobFile(file); _ = file.Close() }
	now := time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `UPDATE jobs SET
		state = CASE WHEN attempt >= max_attempts THEN 'failed' ELSE 'retrying' END,
		message = CASE WHEN attempt >= max_attempts THEN 'Прервано: попытки исчерпаны' ELSE 'Продолжим после перезапуска' END,
		error = 'Приложение было закрыто во время выполнения',
		progress = 0, updated_at = ?, run_after = ?,
		finished_at = CASE WHEN attempt >= max_attempts THEN ? ELSE NULL END
		WHERE state = 'running'`, now, now, now)
	if err != nil {
		release()
		return nil, false, err
	}
	return release, true, nil
}
