package sqlite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"Cryon2/internal/domain"
	"Cryon2/internal/store"
)

func TestDurableJobLifecycle(t *testing.T) {
	ctx := context.Background()
	db := &Store{dbPath: filepath.Join(t.TempDir(), "cryon.db")}
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	now := time.Now().UnixMilli()
	if _, err := db.JobEnqueue(ctx, store.Job{ID: "later", Kind: "import", Priority: 99, RunAfterMs: now + 60_000}); err != nil {
		t.Fatalf("enqueue later: %v", err)
	}
	queued, err := db.JobEnqueue(ctx, store.Job{ID: "ready", Kind: "scan", Priority: 10, PayloadJSON: `{"folder":"music"}`})
	if err != nil {
		t.Fatalf("enqueue ready: %v", err)
	}
	if queued.State != store.JobQueued || queued.MaxAttempts != 3 {
		t.Fatalf("unexpected enqueued job: %+v", queued)
	}

	claimed, ok, err := db.JobClaimNext(ctx, time.Now().UnixMilli())
	if err != nil || !ok {
		t.Fatalf("claim: job=%+v ok=%v err=%v", claimed, ok, err)
	}
	if claimed.ID != "ready" || claimed.State != store.JobRunning || claimed.Attempt != 1 {
		t.Fatalf("unexpected claimed job: %+v", claimed)
	}
	if err := db.JobProgress(ctx, claimed.ID, 0.5, "Сканирование файлов"); err != nil {
		t.Fatalf("progress: %v", err)
	}
	if err := db.JobFinish(ctx, claimed.ID, store.JobCompleted, "Готово", ""); err != nil {
		t.Fatalf("finish: %v", err)
	}

	jobs, err := db.JobList(ctx, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("job count: got %d, want 2", len(jobs))
	}
	var completed store.Job
	for _, job := range jobs {
		if job.ID == "ready" {
			completed = job
		}
	}
	if completed.State != store.JobCompleted || completed.Progress != 1 || completed.FinishedAtMs == 0 {
		t.Fatalf("unexpected completed job: %+v", completed)
	}

	if err := db.JobCancel(ctx, "later"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	jobs, err = db.JobList(ctx, 10)
	if err != nil {
		t.Fatalf("list after cancel: %v", err)
	}
	for _, job := range jobs {
		if job.ID == "later" && job.State != store.JobCancelled {
			t.Fatalf("job was not cancelled: %+v", job)
		}
	}
}

func TestNewerSchemaIsNotDowngraded(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cryon.db")
	db := &Store{dbPath: path}
	if err := db.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `UPDATE schema_migrations SET version = ? WHERE name = 'core'`, coreSchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	older := &Store{dbPath: path}
	if err := older.Init(ctx); err == nil || !strings.Contains(err.Error(), "более новой версией") {
		t.Fatalf("expected newer schema error, got %v", err)
	}
}

func TestRetryJobWaitsBeforeNextClaim(t *testing.T) {
	ctx := context.Background()
	db := &Store{dbPath: filepath.Join(t.TempDir(), "cryon.db")}
	if err := db.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	queued, err := db.JobEnqueue(ctx, store.Job{Kind: "library.rescan"})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, ok, err := db.JobClaimNext(ctx, time.Now().UnixMilli())
	if err != nil || !ok {
		t.Fatalf("claim: job=%+v ok=%v err=%v", claimed, ok, err)
	}
	if err := db.JobFinish(ctx, claimed.ID, store.JobRetrying, "Повторяем", "сеть недоступна"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, ok, err := db.JobClaimNext(ctx, time.Now().UnixMilli()); err != nil || ok {
		t.Fatalf("job claimed before retry delay: ok=%v err=%v", ok, err)
	}
	if queued.ID == "" {
		t.Fatal("job id missing")
	}
}

func TestJobWorkerLockAndRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cryon.db")
	first := &Store{dbPath: path}
	if err := first.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second := &Store{dbPath: path}
	if err := second.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	release, acquired, err := first.AcquireJobWorker(ctx)
	if err != nil || !acquired {
		t.Fatalf("first lock: acquired=%v err=%v", acquired, err)
	}
	if _, acquired, err := second.AcquireJobWorker(ctx); err != nil || acquired {
		t.Fatalf("second lock: acquired=%v err=%v", acquired, err)
	}
	if _, err := first.JobEnqueue(ctx, store.Job{ID: "interrupted", Kind: "scan", MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	job, found, err := first.JobClaimNext(ctx, time.Now().UnixMilli())
	if err != nil || !found || job.Attempt != 1 {
		t.Fatalf("claim: %+v found=%v err=%v", job, found, err)
	}
	release()
	releaseSecond, acquired, err := second.AcquireJobWorker(ctx)
	if err != nil || !acquired {
		t.Fatalf("reacquire: acquired=%v err=%v", acquired, err)
	}
	defer releaseSecond()
	job, found, err = second.JobClaimNext(ctx, time.Now().UnixMilli())
	if err != nil || !found || job.ID != "interrupted" || job.Attempt != 2 {
		t.Fatalf("recovered: %+v found=%v err=%v", job, found, err)
	}
}

func TestRecoveryDoesNotExceedAttemptLimit(t *testing.T) {
	ctx := context.Background()
	db := &Store{dbPath: filepath.Join(t.TempDir(), "cryon.db")}
	if err := db.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.JobEnqueue(ctx, store.Job{ID: "exhausted", Kind: "scan", MaxAttempts: 1}); err != nil {
		t.Fatal(err)
	}
	if _, found, err := db.JobClaimNext(ctx, time.Now().UnixMilli()); err != nil || !found {
		t.Fatalf("claim: %v", err)
	}
	release, acquired, err := db.AcquireJobWorker(ctx)
	if err != nil || !acquired {
		t.Fatalf("lock: %v", err)
	}
	defer release()
	jobs, err := db.JobList(ctx, 10)
	if err != nil || len(jobs) != 1 || jobs[0].State != store.JobFailed {
		t.Fatalf("jobs=%+v err=%v", jobs, err)
	}
}

func TestJobRecoveryAfterProcessExit(t *testing.T) {
	if path := os.Getenv("CRYON_TEST_CRASH_DB"); path != "" {
		db := &Store{dbPath: path}
		ctx := context.Background()
		if err := db.Init(ctx); err != nil {
			os.Exit(2)
		}
		_, acquired, err := db.AcquireJobWorker(ctx)
		if err != nil || !acquired {
			os.Exit(3)
		}
		if _, err := db.JobEnqueue(ctx, store.Job{ID: "crashed", Kind: "scan", MaxAttempts: 3}); err != nil {
			os.Exit(4)
		}
		if _, found, err := db.JobClaimNext(ctx, time.Now().UnixMilli()); err != nil || !found {
			os.Exit(5)
		}
		os.Exit(0)
	}
	path := filepath.Join(t.TempDir(), "cryon.db")
	cmd := exec.Command(os.Args[0], "-test.run=^TestJobRecoveryAfterProcessExit$")
	cmd.Env = append(os.Environ(), "CRYON_TEST_CRASH_DB="+path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v: %s", err, output)
	}
	db := &Store{dbPath: path}
	ctx := context.Background()
	if err := db.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	release, acquired, err := db.AcquireJobWorker(ctx)
	if err != nil || !acquired {
		t.Fatalf("lock after process exit: acquired=%v err=%v", acquired, err)
	}
	defer release()
	job, found, err := db.JobClaimNext(ctx, time.Now().UnixMilli())
	if err != nil || !found || job.ID != "crashed" || job.Attempt != 2 {
		t.Fatalf("recovered job=%+v found=%v err=%v", job, found, err)
	}
}

func TestSchemaMigrationRepairsOlderDatabaseVersion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "cryon.db")
	db := &Store{dbPath: path}
	if err := db.Init(ctx); err != nil {
		t.Fatalf("initial Init: %v", err)
	}
	if err := db.FavoriteAdd(ctx, domain.Track{ID: "saved", Service: domain.ServiceYouTube, Title: "Saved song", Artists: []string{"Artist"}, PlayableKind: domain.PlayableStream}); err != nil {
		t.Fatalf("save favorite: %v", err)
	}
	if err := db.SettingSet(ctx, "test.setting", "preserved"); err != nil {
		t.Fatalf("save setting: %v", err)
	}
	if _, err := db.db.ExecContext(ctx, `DROP TABLE jobs`); err != nil {
		t.Fatalf("drop jobs: %v", err)
	}
	if _, err := db.db.ExecContext(ctx, `UPDATE schema_migrations SET version = 1 WHERE name = 'core'`); err != nil {
		t.Fatalf("downgrade schema marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close old database: %v", err)
	}

	updated := &Store{dbPath: path}
	if err := updated.Init(ctx); err != nil {
		t.Fatalf("updated Init: %v", err)
	}
	t.Cleanup(func() { _ = updated.Close() })

	var version int
	if err := updated.db.QueryRowContext(ctx, `SELECT version FROM schema_migrations WHERE name = 'core'`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version != coreSchemaVersion {
		t.Fatalf("schema version: got %d, want %d", version, coreSchemaVersion)
	}
	var jobsTable string
	if err := updated.db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'jobs'`).Scan(&jobsTable); err != nil {
		t.Fatalf("jobs table was not migrated back: %v", err)
	}
	favorites, err := updated.FavoriteList(ctx)
	if err != nil || len(favorites) != 1 || favorites[0].ID != "saved" {
		t.Fatalf("favorites after migration: %+v, err=%v", favorites, err)
	}
	value, found, err := updated.SettingGet(ctx, "test.setting")
	if err != nil || !found || value != "preserved" {
		t.Fatalf("setting after migration: value=%q found=%v err=%v", value, found, err)
	}
}
