package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
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
// real network calls or yt-dlp/ffmpeg invocations. onStage is forwarded to
// the underlying video.Pipeline.Run's onStage callback (spec Scope item 1).
type Processor interface {
	Process(ctx context.Context, youtubeURL string, onStage func(string)) (video.Result, error)
}

// pipelineProcessor adapts a *video.Pipeline (whose method is named Run,
// per internal/video's Pipeline.Run(ctx, youtubeURL, onStage) (Result,
// error)) to the Processor interface.
type pipelineProcessor struct {
	pipeline *video.Pipeline
}

func (p pipelineProcessor) Process(ctx context.Context, youtubeURL string, onStage func(string)) (video.Result, error) {
	return p.pipeline.Run(ctx, youtubeURL, onStage)
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

		onStage := func(stage string) {
			publishStageChanged(ctx, redisClient, processing.ID, processing.UserID, stage)
		}
		result, procErr := processor.Process(ctx, processing.YoutubeURL, onStage)

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
			// procErr itself was never logged anywhere before this — only
			// written into job_summaries via failureSummary, invisible to
			// `docker compose logs worker` (and to anyone not querying the
			// DB directly). Found live: a real processing failure produced
			// zero worker log output, which looked identical to the worker
			// never running at all.
			slog.Error("worker: process failed", "job_id", processing.ID, "err", procErr)
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

// failureSummary renders a short, fixed human-readable failure reason for a
// job whose processing attempt(s) ended in failure — one of a small closed
// set of category strings, never the raw wrapped Go error text, which can
// include internal temp-directory paths and unfiltered yt-dlp/ffmpeg stderr
// (spec Constraints: security; acceptance criterion 11). The complete,
// unredacted err is still logged via slog by processTick's caller, purely
// for operator debugging.
//
// Categorization is errors.Is-based for the three categories with an
// existing sentinel (video.ErrCorruptDownload, video.ErrVideoTooShort,
// context.DeadlineExceeded). Distinguishing "download failed" from
// "analysis failed" — the two categories with no existing sentinel — has no
// typed stage-tagging mechanism to hook into: this falls back to matching
// the "video pipeline: download:"/"video pipeline: validate:"/
// "video pipeline: probe:"/"video pipeline: analyze:" prefixes runAttempt
// already wraps every error with (internal/video/video.go). This is a
// deliberate, smaller-than-planned deviation from plan.md step 10's
// preferred "typed sentinel/stage tag in internal/video" approach (plan.md
// Risks #7 anticipated this fallback as an acceptable alternative) —
// surfaced here per CLAUDE.md's deviation rule; a spec.md amendment is
// warranted if this lands.
//
// A generic (non-video.ErrCorruptDownload) Validator failure and any Prober
// failure both collapse into "analysis failed": both are post-download,
// pre-completion stages with no dedicated spec-documented category of their
// own — "analysis failed" is the natural, already-documented bucket for
// "something about analyzing this downloaded video didn't work" (reviewer
// finding: a Prober failure previously matched none of the cases below and
// silently fell to an undocumented 6th "processing failed" default).
//
// The default case below is reachable by a small number of pipeline-internal
// errors that predate this spec and aren't tied to any pipeline stage
// (runAttempt's temp-dir creation/removal failures, and attempt panics) —
// none of these can be ruled out structurally, so the default also returns
// one of the 5 spec-documented strings ("analysis failed") rather than an
// undocumented 6th category, per spec Scope item 5's closed set.
func failureSummary(err error) string {
	switch {
	case errors.Is(err, video.ErrCorruptDownload):
		return "downloaded file was incomplete or corrupt"
	case errors.Is(err, video.ErrVideoTooShort):
		return "video too short or corrupt to analyze"
	case errors.Is(err, context.DeadlineExceeded):
		return "analysis timed out"
	case strings.Contains(err.Error(), "video pipeline: download:"):
		return "download failed"
	case strings.Contains(err.Error(), "video pipeline: validate:"):
		return "analysis failed"
	case strings.Contains(err.Error(), "video pipeline: probe:"):
		return "analysis failed"
	case strings.Contains(err.Error(), "video pipeline: analyze:"):
		return "analysis failed"
	default:
		return "analysis failed"
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

// publishStageChanged publishes one processing-attempt stage transition to
// jobevents.Channel, logging (and otherwise ignoring) a publish error for
// the same reason publishStatusChanged does (spec edge case 6) — a stage
// event is ephemeral pub/sub only, never a checkpoint, so a dropped publish
// never affects job.status or correctness.
func publishStageChanged(ctx context.Context, redisClient *redis.Client, jobID, userID uint64, stage string) {
	err := jobevents.PublishStage(ctx, redisClient, jobevents.StageChanged{
		JobID:  jobID,
		UserID: userID,
		Stage:  stage,
	})
	if err != nil {
		slog.Error("worker: publish job stage change failed", "job_id", jobID, "stage", stage, "err", err)
	}
}

// loadAnalyzerConfig parses MOTION_GRID_CELL_PX/MOTION_THRESHOLD_PER_PAIR/
// MOTION_MIN_REGION_CELLS/MAX_PARTICIPANTS via getenv (production wires
// os.Getenv; tests inject a map lookup) into a video.FFMPEGAnalyzer, mirroring
// main()'s existing PROCESSING_TIMEOUT invalid-value handling (spec edge
// case 6): an unset or unparseable value logs a warning and falls back to
// FFMPEGAnalyzer's documented zero-value default for that field, never
// aborting startup or defaulting the other fields.
func loadAnalyzerConfig(getenv func(string) string) video.FFMPEGAnalyzer {
	var cfg video.FFMPEGAnalyzer

	if v := getenv("MOTION_GRID_CELL_PX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Error("worker: invalid MOTION_GRID_CELL_PX, using default", "value", v, "err", err)
		} else {
			cfg.GridCellPx = n
		}
	}

	if v := getenv("MOTION_THRESHOLD_PER_PAIR"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			slog.Error("worker: invalid MOTION_THRESHOLD_PER_PAIR, using default", "value", v, "err", err)
		} else {
			cfg.MotionThresholdPerPair = f
		}
	}

	if v := getenv("MOTION_MIN_REGION_CELLS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Error("worker: invalid MOTION_MIN_REGION_CELLS, using default", "value", v, "err", err)
		} else {
			cfg.MinRegionCells = n
		}
	}

	if v := getenv("MAX_PARTICIPANTS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			slog.Error("worker: invalid MAX_PARTICIPANTS, using default", "value", v, "err", err)
		} else {
			cfg.MaxParticipants = n
		}
	}

	return cfg
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
		Validator:   video.FFProbeValidator{},
		Prober:      video.FFProbeProber{},
		Analyzer:    loadAnalyzerConfig(os.Getenv),
		BackoffBase: 1 * time.Second,
		Timeout:     processingTimeout,
	}
	processor := pipelineProcessor{pipeline: pipeline}

	runLoop(ctx, sqlDB, redisClient, processor)
}
