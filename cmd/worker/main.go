package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
	"github.com/g4uk/kai/internal/video"
)

// defaultProcessingTimeout is used when PROCESSING_TIMEOUT is unset (spec
// Scope: "a configurable timeout (env var, default 10 minutes)").
const defaultProcessingTimeout = 10 * time.Minute

// Processor runs the real (or, in tests, faked) download/probe/analyze
// pipeline for a job's youtube_url. It is a small, consumer-defined
// single-method interface (matching the OTPRequester/JobCreator pattern in
// internal/handler) so processTick's tests can substitute a fake with zero
// real network calls or yt-dlp/ffmpeg invocations.
type Processor interface {
	Process(ctx context.Context, youtubeURL string) (video.Result, error)
}

// pipelineProcessor adapts a *video.Pipeline (whose method is named Run,
// per internal/video's Pipeline.Run(ctx, youtubeURL) (Result, error)) to the
// Processor interface.
type pipelineProcessor struct {
	pipeline *video.Pipeline
}

func (p pipelineProcessor) Process(ctx context.Context, youtubeURL string) (video.Result, error) {
	return p.pipeline.Run(ctx, youtubeURL)
}

// processTick performs one pass over every job snapshotted as "pending" at
// call time and, for each, runs the full pipeline synchronously within this
// same call (specs/video-processing/plan.md steps 6/7 — replacing the
// popup-notifications+sse stub's separate-tick "advance pending, then
// advance processing" model, an intentional behavior change, not a
// regression: plan.md Risks #2).
//
// For each pending job: UpdateStatus->processing (publish fires), then
// processor.Process(ctx, youtubeURL) runs. On success: job.SaveResults +
// job.SaveSummary (success text) + UpdateStatus->done (second publish
// fires). On failure: job.SaveSummary (failure reason) + UpdateStatus->failed
// (second publish fires), with zero SaveResults calls.
//
// processTick never scans for pre-existing "processing" jobs — under this
// synchronous-per-job model, no job should be sitting in "processing" at the
// start of a tick during normal operation; a crash mid-pipeline is the one
// exception, and per spec Non-scope it is deliberately never picked back up.
func processTick(ctx context.Context, sqlDB *sql.DB, redisClient *redis.Client, processor Processor) {
	pending, err := job.ListByStatus(ctx, sqlDB, "pending")
	if err != nil {
		slog.Error("worker: list pending jobs failed", "err", err)
		return
	}

	for _, j := range pending {
		processing, err := job.UpdateStatus(ctx, sqlDB, j.ID, "processing")
		if err != nil {
			slog.Error("worker: advance pending->processing failed", "job_id", j.ID, "err", err)
			continue
		}
		publishStatusChanged(ctx, redisClient, processing)

		result, procErr := processor.Process(ctx, processing.YoutubeURL)

		var final job.Job
		if procErr == nil {
			if err := job.SaveResults(ctx, sqlDB, processing.ID, result.Participants); err != nil {
				slog.Error("worker: save results failed", "job_id", processing.ID, "err", err)
			}
			if err := job.SaveSummary(ctx, sqlDB, processing.ID, successSummary(result)); err != nil {
				slog.Error("worker: save summary failed", "job_id", processing.ID, "err", err)
			}

			final, err = job.UpdateStatus(ctx, sqlDB, processing.ID, "done")
			if err != nil {
				slog.Error("worker: advance processing->done failed", "job_id", processing.ID, "err", err)
				continue
			}
		} else {
			if err := job.SaveSummary(ctx, sqlDB, processing.ID, failureSummary(procErr)); err != nil {
				slog.Error("worker: save failure summary failed", "job_id", processing.ID, "err", err)
			}

			final, err = job.UpdateStatus(ctx, sqlDB, processing.ID, "failed")
			if err != nil {
				slog.Error("worker: advance processing->failed failed", "job_id", processing.ID, "err", err)
				continue
			}
		}
		publishStatusChanged(ctx, redisClient, final)
	}
}

// successSummary renders a short human-readable summary for a successfully
// processed job (spec Scope: "duration/resolution/fps and participant
// count").
func successSummary(result video.Result) string {
	m := result.Metadata
	return fmt.Sprintf(
		"duration %.1fs, %dx%d @ %.2ffps, %d participant(s) detected",
		m.Duration, m.Width, m.Height, m.FPS, len(result.Participants),
	)
}

// failureSummary renders a short human-readable failure reason for a job
// whose processing attempt(s) ended in failure. A too-short/corrupt video
// (spec edge case 3, video.ErrVideoTooShort) gets a reason text distinct
// from a generic network/timeout failure, so a coach reading job_summaries
// can tell "this video can't be analyzed" apart from "a transient error
// exhausted all retries."
func failureSummary(err error) string {
	if errors.Is(err, video.ErrVideoTooShort) {
		return fmt.Sprintf("video too short or corrupt to analyze: %v", err)
	}
	return fmt.Sprintf("processing failed: %v", err)
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

func runLoop(ctx context.Context, sqlDB *sql.DB, redisClient *redis.Client, processor Processor) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			processTick(ctx, sqlDB, redisClient, processor)
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

	processingTimeout := defaultProcessingTimeout
	if v := os.Getenv("PROCESSING_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			slog.Error("worker: invalid PROCESSING_TIMEOUT, using default", "value", v, "default", processingTimeout, "err", err)
		} else {
			processingTimeout = d
		}
	}

	pipeline := &video.Pipeline{
		Downloader:  video.YTDLPDownloader{},
		Prober:      video.FFProbeProber{},
		Analyzer:    video.FFMPEGAnalyzer{},
		BackoffBase: 1 * time.Second,
		Timeout:     processingTimeout,
	}
	processor := pipelineProcessor{pipeline: pipeline}

	runLoop(ctx, sqlDB, redisClient, processor)
}
