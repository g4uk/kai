package main

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"github.com/g4uk/kai/internal/db"
	"github.com/g4uk/kai/internal/job"
	"github.com/g4uk/kai/internal/redisconn"
	"github.com/g4uk/kai/internal/user"
)

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/popup-notifications+sse/plan.md step 4)
//
// runLoop's signature changes from runLoop(ctx) to
// runLoop(ctx context.Context, db *sql.DB, redisClient *redis.Client); the
// call site below is updated to match (passing nil, nil — the test cancels
// the context before runLoop ever touches either dependency, so nils are
// safe here, exactly as plan.md step 4 specifies).
//
// This file also references a not-yet-existing standalone function:
//
//	func processTick(ctx context.Context, db *sql.DB, redisClient *redis.Client)
//
// extracted from runLoop's per-tick body so it's callable directly without
// waiting on the real 5s ticker. Per tick, ordering is load-bearing (see
// TestProcessTick_AdvancesPendingAndProcessingSeparately below and plan.md's
// Risks section): job.ListByStatus(ctx, db, "pending") and
// job.ListByStatus(ctx, db, "processing") must both be snapshotted BEFORE
// any writes happen this tick, so a job spends exactly one tick in
// "processing" before advancing to "done" rather than jumping straight
// through pending->done in the same tick it was created.
// ----------------------------------------------------------------------------

func TestRunLoop_Cancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before runLoop is even called

	done := make(chan struct{})
	go func() {
		runLoop(ctx, nil, nil)
		close(done)
	}()

	select {
	case <-done:
		// runLoop returned promptly after ctx was cancelled — expected
	case <-time.After(1 * time.Second):
		t.Fatal("runLoop did not return within 1s after context cancellation")
	}
}

// testDB mirrors internal/job/job_test.go's helper; package-local, not
// shared (test helpers can't be imported across packages).
func testDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set; skipping integration test")
	}

	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.Up(sqlDB); err != nil {
		t.Fatalf("db.Up: %v", err)
	}

	return sqlDB
}

// testRedisClient connects via internal/redisconn.Connect (the same
// constructor cmd/worker/main.go's main() uses), gated on TEST_REDIS_ADDR
// per internal/session/session_test.go's convention.
func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()

	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := redisconn.Connect(ctx, addr)
	if err != nil {
		t.Fatalf("redisconn.Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return client
}

// mustCreateUser mirrors internal/job/job_test.go's helper; package-local,
// not shared.
func mustCreateUser(t *testing.T, sqlDB *sql.DB, phone string) uint64 {
	t.Helper()

	u, err := user.Create(context.Background(), sqlDB, phone)
	if err != nil {
		t.Fatalf("user.Create(%q): %v", phone, err)
	}
	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM users WHERE id = ?`, u.ID)
	})
	return u.ID
}

// cleanupJob mirrors internal/job/job_test.go's helper; package-local, not
// shared.
func cleanupJob(t *testing.T, sqlDB *sql.DB, jobID uint64) {
	t.Helper()

	t.Cleanup(func() {
		_, _ = sqlDB.Exec(`DELETE FROM job_summaries WHERE job_id = ?`, jobID)
		_, _ = sqlDB.Exec(`DELETE FROM participant_metrics WHERE participant_id IN (SELECT id FROM participants WHERE job_id = ?)`, jobID)
		_, _ = sqlDB.Exec(`DELETE FROM participants WHERE job_id = ?`, jobID)
		_, _ = sqlDB.Exec(`DELETE FROM analysis_jobs WHERE id = ?`, jobID)
	})
}

func TestProcessTick_AdvancesPendingAndProcessingSeparately(t *testing.T) {
	sqlDB := testDB(t)
	redisClient := testRedisClient(t)
	ctx := context.Background()

	userID := mustCreateUser(t, sqlDB, "+15559995001")

	pendingJob, err := job.Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=process-tick-pending-1")
	if err != nil {
		t.Fatalf("Create pending job: %v", err)
	}
	cleanupJob(t, sqlDB, pendingJob.ID)

	processingJob, err := job.Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=process-tick-processing-1")
	if err != nil {
		t.Fatalf("Create processing-seed job: %v", err)
	}
	cleanupJob(t, sqlDB, processingJob.ID)
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE analysis_jobs SET status = 'processing' WHERE id = ?`, processingJob.ID,
	); err != nil {
		t.Fatalf("seed processingJob to 'processing': %v", err)
	}

	processTick(ctx, sqlDB, redisClient)

	var gotPendingStatus, gotProcessingStatus string
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status FROM analysis_jobs WHERE id = ?`, pendingJob.ID,
	).Scan(&gotPendingStatus); err != nil {
		t.Fatalf("query pendingJob status: %v", err)
	}
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status FROM analysis_jobs WHERE id = ?`, processingJob.ID,
	).Scan(&gotProcessingStatus); err != nil {
		t.Fatalf("query processingJob status: %v", err)
	}

	// Critical assertion (plan.md's snapshot-before-write ordering note): a
	// job that started this tick as 'pending' must land on 'processing', NOT
	// 'done' — it must not jump through both transitions in a single tick.
	if gotPendingStatus != "processing" {
		t.Errorf("pendingJob status after one processTick = %q, want %q (not %q)", gotPendingStatus, "processing", "done")
	}
	if gotProcessingStatus != "done" {
		t.Errorf("processingJob status after one processTick = %q, want %q", gotProcessingStatus, "done")
	}
}

// TestProcessTick_RedisUnavailableStillAdvancesDBStatus covers spec edge
// case 6 ("Redis temporarily unavailable"): a failed jobevents.Publish call
// must not stop processTick's DB write from succeeding or crash the worker.
// Unlike TestProcessTick_AdvancesPendingAndProcessingSeparately, this test
// is gated only on TEST_DSN (not TEST_REDIS_ADDR) — it deliberately builds a
// *redis.Client pointed at a port nothing listens on, so the publish itself
// is exercised and fails, rather than being skipped.
func TestProcessTick_RedisUnavailableStillAdvancesDBStatus(t *testing.T) {
	sqlDB := testDB(t)
	ctx := context.Background()

	// Deliberately unreachable: nothing listens on 127.0.0.1:1. A short
	// DialTimeout keeps the test fast instead of waiting on the client's
	// default dial timeout.
	unreachableRedis := redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 200 * time.Millisecond,
	})
	t.Cleanup(func() { _ = unreachableRedis.Close() })

	userID := mustCreateUser(t, sqlDB, "+15559995002")

	pendingJob, err := job.Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=process-tick-redis-down-1")
	if err != nil {
		t.Fatalf("Create pending job: %v", err)
	}
	cleanupJob(t, sqlDB, pendingJob.ID)

	done := make(chan struct{})
	go func() {
		processTick(ctx, sqlDB, unreachableRedis)
		close(done)
	}()

	select {
	case <-done:
		// processTick returned without panicking despite Redis being down —
		// expected.
	case <-time.After(5 * time.Second):
		t.Fatal("processTick did not return within 5s with Redis unavailable")
	}

	var gotStatus string
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status FROM analysis_jobs WHERE id = ?`, pendingJob.ID,
	).Scan(&gotStatus); err != nil {
		t.Fatalf("query pendingJob status: %v", err)
	}

	// The DB write must succeed independently of whether the subsequent
	// jobevents.Publish call succeeds.
	if gotStatus != "processing" {
		t.Errorf("pendingJob status after processTick with Redis unavailable = %q, want %q", gotStatus, "processing")
	}
}
