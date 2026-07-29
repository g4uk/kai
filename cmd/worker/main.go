package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/g4uk/kai/internal/db"
	"github.com/g4uk/kai/internal/job"
	"github.com/g4uk/kai/internal/jobevents"
	"github.com/g4uk/kai/internal/redisconn"
)

// processTick is the worker's stub progression (specs/popup-notifications+sse/spec.md
// Scope: "a deliberate stand-in for the real analysis pipeline"): it
// snapshots BOTH the currently-pending and currently-processing jobs BEFORE
// writing anything this tick (load-bearing ordering — see plan.md's Risks
// section), then advances the snapshotted pending jobs to "processing" and
// the snapshotted (pre-tick) processing jobs to "done", publishing a
// jobevents.StatusChanged after each successful UpdateStatus. A publish
// error is logged and does not stop the tick or crash the process (spec
// edge case 6) — only the notification path degrades if Redis is briefly
// unavailable.
func processTick(ctx context.Context, sqlDB *sql.DB, redisClient *redis.Client) {
	pending, err := job.ListByStatus(ctx, sqlDB, "pending")
	if err != nil {
		slog.Error("worker: list pending jobs failed", "err", err)
		return
	}
	processing, err := job.ListByStatus(ctx, sqlDB, "processing")
	if err != nil {
		slog.Error("worker: list processing jobs failed", "err", err)
		return
	}

	for _, j := range pending {
		updated, err := job.UpdateStatus(ctx, sqlDB, j.ID, "processing")
		if err != nil {
			slog.Error("worker: advance pending->processing failed", "job_id", j.ID, "err", err)
			continue
		}
		publishStatusChanged(ctx, redisClient, updated)
	}

	for _, j := range processing {
		updated, err := job.UpdateStatus(ctx, sqlDB, j.ID, "done")
		if err != nil {
			slog.Error("worker: advance processing->done failed", "job_id", j.ID, "err", err)
			continue
		}
		publishStatusChanged(ctx, redisClient, updated)
	}
}

// publishStatusChanged publishes j's new status to jobevents.Channel,
// logging (and otherwise ignoring) a publish error so a Redis hiccup never
// stops job progression or crashes the worker (spec edge case 6).
func publishStatusChanged(ctx context.Context, redisClient *redis.Client, j job.Job) {
	err := jobevents.Publish(ctx, redisClient, jobevents.StatusChanged{
		JobID:  j.ID,
		UserID: j.UserID,
		Status: j.Status,
	})
	if err != nil {
		slog.Error("worker: publish job status change failed", "job_id", j.ID, "err", err)
	}
}

func runLoop(ctx context.Context, sqlDB *sql.DB, redisClient *redis.Client) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processTick(ctx, sqlDB, redisClient)
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	dsn := os.Getenv("DB_DSN")
	redisAddr := os.Getenv("REDIS_ADDR")

	sqlDB, err := db.Connect(ctx, dsn)
	if err != nil {
		slog.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = sqlDB.Close() }()

	redisClient, err := redisconn.Connect(ctx, redisAddr)
	if err != nil {
		slog.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()

	runLoop(ctx, sqlDB, redisClient)
}
