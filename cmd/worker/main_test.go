package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"github.com/g4uk/kai/internal/db"
	"github.com/g4uk/kai/internal/job"
	"github.com/g4uk/kai/internal/jobevents"
	"github.com/g4uk/kai/internal/redisconn"
	"github.com/g4uk/kai/internal/user"
	"github.com/g4uk/kai/internal/video"
)

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/video-processing/plan.md step 6)
//
// This file drives a redesign of cmd/worker/main.go's processTick, replacing
// the popup-notifications+sse stub's "advance pending and processing
// separately, one status transition per tick" model. Until main.go defines
// the following, this package fails to compile (expected, correct red
// state) — and, independently, until internal/video/video.go defines
// video.Result/video.ParticipantResult (see internal/video/video_test.go's
// own red-phase note), this file's import of internal/video also fails to
// compile:
//
//	type Processor interface {
//	    Process(ctx context.Context, youtubeURL string) (video.Result, error)
//	}
//
//	func processTick(ctx context.Context, db *sql.DB, redisClient *redis.Client, processor Processor)
//	func runLoop(ctx context.Context, db *sql.DB, redisClient *redis.Client, processor Processor)
//
// New synchronous-per-job model (plan.md step 6/7): for each job
// snapshotted as "pending" at the start of a processTick call,
// processTick now performs BOTH transitions in that same call —
// pending->processing (publish fires), then, synchronously,
// processor.Process(ctx, j.YoutubeURL) runs and, depending on its result,
// either:
//   - success: job.SaveResults + job.SaveSummary (success text) +
//     UpdateStatus->done (second publish fires), or
//   - failure: job.SaveSummary (failure reason) + UpdateStatus->failed
//     (second publish fires), with zero job.SaveResults calls.
//
// This deliberately changes the old "one tick in processing" invariant that
// TestProcessTick_AdvancesPendingAndProcessingSeparately asserted (removed
// below, plan.md Risks #2): under the new model, processTick never scans
// for pre-existing "processing" jobs at all — a job stuck in processing
// (e.g. from a worker crash mid-pipeline) is not picked back up, per spec
// Non-scope's "crash/restart recovery ... is a future spec, not this one."
// ----------------------------------------------------------------------------

// fakeProcessor is a test double for the Processor interface: it scripts a
// single (video.Result, error) outcome and records every youtubeURL it was
// called with, so tests can assert both "was it called" and "with what."
//
// stages (specs/video-processing-improvements/plan.md step 9a) is an
// optional list of stage names this fake reports via the onStage callback,
// in order, before returning its scripted result — simulating how the real
// video.Pipeline reports "downloading"/"probing"/"analyzing" progress, so
// tests can assert processTick wires onStage into a real
// jobevents.PublishStage call without depending on video.Pipeline itself.
type fakeProcessor struct {
	result video.Result
	err    error
	stages []string

	calls   int
	gotURLs []string
}

func (f *fakeProcessor) Process(_ context.Context, youtubeURL string, onStage func(string)) (video.Result, error) {
	f.calls++
	f.gotURLs = append(f.gotURLs, youtubeURL)
	for _, stage := range f.stages {
		if onStage != nil {
			onStage(stage)
		}
	}
	return f.result, f.err
}

// TestFailureSummary_DistinguishesTooShortVideoFromGenericFailure covers
// spec edge case 3's requirement that a job_summaries failure reason for a
// too-short/corrupt video is distinguishable from a generic/timeout failure
// reason (reviewer finding on video-processing): failureSummary must render
// different text for video.ErrVideoTooShort than for an arbitrary error.
func TestFailureSummary_DistinguishesTooShortVideoFromGenericFailure(t *testing.T) {
	genericErr := errors.New("simulated: all retry attempts exhausted")
	tooShortErr := fmt.Errorf("video pipeline: attempt 1 failed (non-retryable): %w", video.ErrVideoTooShort)

	genericText := failureSummary(genericErr)
	tooShortText := failureSummary(tooShortErr)

	if genericText == tooShortText {
		t.Fatalf("failureSummary produced identical text for a generic failure and ErrVideoTooShort: %q", genericText)
	}
	if strings.Contains(genericText, "too short") {
		t.Errorf("generic failure summary %q should not mention 'too short'", genericText)
	}
	if !strings.Contains(tooShortText, "too short") {
		t.Errorf("ErrVideoTooShort failure summary %q should mention 'too short'", tooShortText)
	}
}

func TestRunLoop_Cancels(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before runLoop is even called

	done := make(chan struct{})
	go func() {
		runLoop(ctx, nil, nil, nil)
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

// countParticipants mirrors internal/job/job_test.go's helper; package-local,
// not shared.
func countParticipants(t *testing.T, sqlDB *sql.DB, jobID uint64) int {
	t.Helper()

	var count int
	if err := sqlDB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM participants WHERE job_id = ?`, jobID,
	).Scan(&count); err != nil {
		t.Fatalf("countParticipants(%d): %v", jobID, err)
	}
	return count
}

// jobSummaryRow returns the single job_summaries row's summary text for
// jobID, and whether any row exists at all.
func jobSummaryRow(t *testing.T, sqlDB *sql.DB, jobID uint64) (summary string, exists bool) {
	t.Helper()

	err := sqlDB.QueryRowContext(context.Background(),
		`SELECT summary FROM job_summaries WHERE job_id = ?`, jobID,
	).Scan(&summary)
	switch {
	case err == nil:
		return summary, true
	case errors.Is(err, sql.ErrNoRows):
		return "", false
	default:
		t.Fatalf("jobSummaryRow(%d): %v", jobID, err)
		return "", false
	}
}

// TestProcessTick_PendingSucceedsReachesDoneWithinSameTick replaces
// TestProcessTick_AdvancesPendingAndProcessingSeparately (see this file's
// TDD RED PHASE NOTE / plan.md Risks #2): it asserts a pending job reaches
// "done" within a single processTick call — pending->processing->done, not
// spread across two ticks — when the injected Processor succeeds, and that
// SaveResults/SaveSummary were actually invoked (proven by querying the rows
// they write).
func TestProcessTick_PendingSucceedsReachesDoneWithinSameTick(t *testing.T) {
	sqlDB := testDB(t)
	redisClient := testRedisClient(t)
	ctx := context.Background()

	userID := mustCreateUser(t, sqlDB, "+15559997001")
	url := "https://www.youtube.com/watch?v=process-tick-success-1"

	pendingJob, err := job.Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("Create pending job: %v", err)
	}
	cleanupJob(t, sqlDB, pendingJob.ID)

	processor := &fakeProcessor{
		result: video.Result{
			Metadata: video.Metadata{Duration: 42.0, Width: 1280, Height: 720, FPS: 30},
			Participants: []video.ParticipantResult{
				{Label: "Participant 1", ActivityScore: 9.9},
				{Label: "Participant 2", ActivityScore: 4.4},
			},
		},
	}

	processTick(ctx, sqlDB, redisClient, processor)

	var gotStatus string
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status FROM analysis_jobs WHERE id = ?`, pendingJob.ID,
	).Scan(&gotStatus); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if gotStatus != "done" {
		t.Errorf("job status after one processTick call = %q, want %q (pending->processing->done within the same call)", gotStatus, "done")
	}

	if processor.calls != 1 {
		t.Fatalf("processor.calls = %d, want 1", processor.calls)
	}
	if processor.gotURLs[0] != url {
		t.Errorf("processor called with URL %q, want %q", processor.gotURLs[0], url)
	}

	if got := countParticipants(t, sqlDB, pendingJob.ID); got != 2 {
		t.Errorf("countParticipants = %d, want 2 (proves SaveResults was called)", got)
	}

	summary, exists := jobSummaryRow(t, sqlDB, pendingJob.ID)
	if !exists {
		t.Fatal("no job_summaries row found (proves SaveSummary was not called)")
	}
	if summary == "" {
		t.Error("job_summaries.summary is empty, want a human-readable success summary")
	}
}

// TestProcessTick_PendingFailsReachesFailedWithinSameTick is the symmetric
// failure-path test: when the injected Processor returns an error, the job
// reaches "failed" (not "done") within the same processTick call, exactly
// one job_summaries row is written with a failure reason, and zero
// participants/participant_metrics rows are written (zero SaveResults
// calls).
func TestProcessTick_PendingFailsReachesFailedWithinSameTick(t *testing.T) {
	sqlDB := testDB(t)
	redisClient := testRedisClient(t)
	ctx := context.Background()

	userID := mustCreateUser(t, sqlDB, "+15559997002")
	url := "https://www.youtube.com/watch?v=process-tick-failure-1"

	pendingJob, err := job.Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("Create pending job: %v", err)
	}
	cleanupJob(t, sqlDB, pendingJob.ID)

	processor := &fakeProcessor{
		err: errors.New("simulated: all retry attempts exhausted"),
	}

	processTick(ctx, sqlDB, redisClient, processor)

	var gotStatus string
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status FROM analysis_jobs WHERE id = ?`, pendingJob.ID,
	).Scan(&gotStatus); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if gotStatus != "failed" {
		t.Errorf("job status after one processTick call with a failing processor = %q, want %q", gotStatus, "failed")
	}

	if processor.calls != 1 {
		t.Fatalf("processor.calls = %d, want 1", processor.calls)
	}

	if got := countParticipants(t, sqlDB, pendingJob.ID); got != 0 {
		t.Errorf("countParticipants = %d, want 0 (zero SaveResults calls on failure)", got)
	}

	summary, exists := jobSummaryRow(t, sqlDB, pendingJob.ID)
	if !exists {
		t.Fatal("no job_summaries row found (proves SaveSummary was not called with a failure reason)")
	}
	if summary == "" {
		t.Error("job_summaries.summary is empty, want a human-readable failure reason")
	}
}

// TestProcessTick_DoesNotAdvancePreExistingProcessingJob asserts the new
// model's other half: a job that is already "processing" at the start of a
// processTick call (e.g. left behind by a crashed worker) is left
// completely untouched — no status change, no processor call, no summary —
// matching spec Non-scope's "crash/restart recovery ... is a future spec."
func TestProcessTick_DoesNotAdvancePreExistingProcessingJob(t *testing.T) {
	sqlDB := testDB(t)
	redisClient := testRedisClient(t)
	ctx := context.Background()

	userID := mustCreateUser(t, sqlDB, "+15559997003")

	stuckJob, err := job.Create(ctx, sqlDB, userID, "https://www.youtube.com/watch?v=process-tick-stuck-1")
	if err != nil {
		t.Fatalf("Create job: %v", err)
	}
	cleanupJob(t, sqlDB, stuckJob.ID)
	if _, err := sqlDB.ExecContext(ctx,
		`UPDATE analysis_jobs SET status = 'processing' WHERE id = ?`, stuckJob.ID,
	); err != nil {
		t.Fatalf("seed stuckJob to 'processing': %v", err)
	}

	processor := &fakeProcessor{result: video.Result{}}

	processTick(ctx, sqlDB, redisClient, processor)

	var gotStatus string
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT status FROM analysis_jobs WHERE id = ?`, stuckJob.ID,
	).Scan(&gotStatus); err != nil {
		t.Fatalf("query job status: %v", err)
	}
	if gotStatus != "processing" {
		t.Errorf("pre-existing processing job's status after processTick = %q, want unchanged %q", gotStatus, "processing")
	}
	if processor.calls != 0 {
		t.Errorf("processor.calls = %d, want 0 (a pre-existing processing job must not be picked up)", processor.calls)
	}
	if _, exists := jobSummaryRow(t, sqlDB, stuckJob.ID); exists {
		t.Error("job_summaries row exists for a pre-existing processing job that processTick should never have touched")
	}
}

// TestProcessTick_RedisUnavailableStillAdvancesDBStatus covers spec edge
// case 6 ("Redis temporarily unavailable"): a failed jobevents.Publish call
// must not stop processTick's DB writes from succeeding or crash the
// worker. This test is gated only on TEST_DSN (not TEST_REDIS_ADDR) — it
// deliberately builds a *redis.Client pointed at a port nothing listens on,
// so publish itself is exercised and fails, rather than being skipped.
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

	processor := &fakeProcessor{
		result: video.Result{Metadata: video.Metadata{Duration: 1, Width: 1, Height: 1, FPS: 1}},
	}

	done := make(chan struct{})
	go func() {
		processTick(ctx, sqlDB, unreachableRedis, processor)
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

	// The DB writes (both transitions, in the new synchronous-per-job model)
	// must succeed independently of whether either jobevents.Publish call
	// succeeds.
	if gotStatus != "done" {
		t.Errorf("pendingJob status after processTick with Redis unavailable = %q, want %q", gotStatus, "done")
	}
}

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/video-processing-improvements/plan.md step 9)
//
// The tests below drive three changes to cmd/worker/main.go that don't
// exist yet, so this package fails to compile until main.go is updated
// (expected, correct red state):
//
//   - The Processor interface's Process method gains an onStage func(string)
//     parameter: Process(ctx context.Context, youtubeURL string, onStage
//     func(string)) (video.Result, error). processTick must wire a closure
//     into it that publishes a jobevents.StageChanged event (via
//     jobevents.PublishStage) per stage, so a subscribed SSE client sees one
//     stage event per transition (spec acceptance criteria 1/2).
//   - failureSummary switches on new video-package error categories
//     (video.ErrCorruptDownload from plan.md step 3/4, plus whatever
//     stage-tagging mechanism step 10 introduces to distinguish "download
//     failed" from "analysis failed") instead of interpolating the raw
//     error text (spec acceptance criterion 11).
//   - A new loadAnalyzerConfig(getenv func(string) string) video.FFMPEGAnalyzer
//     helper parses MOTION_GRID_CELL_PX/MOTION_THRESHOLD_PER_PAIR/
//     MOTION_MIN_REGION_CELLS/MAX_PARTICIPANTS, mirroring main()'s existing
//     PROCESSING_TIMEOUT invalid-value handling.
// ----------------------------------------------------------------------------

// TestProcessTick_PublishesStageEventsInOrder covers plan.md step 9a: the
// Processor interface's Process gains an onStage func(string) parameter,
// and processTick must wire a closure into it that publishes a
// jobevents.StageChanged event (via jobevents.PublishStage) for each stage
// the Processor reports, in order, for a pending job.
func TestProcessTick_PublishesStageEventsInOrder(t *testing.T) {
	sqlDB := testDB(t)
	redisClient := testRedisClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	userID := mustCreateUser(t, sqlDB, "+15559997010")
	url := "https://www.youtube.com/watch?v=process-tick-stage-events-1"

	pendingJob, err := job.Create(ctx, sqlDB, userID, url)
	if err != nil {
		t.Fatalf("Create pending job: %v", err)
	}
	cleanupJob(t, sqlDB, pendingJob.ID)

	b := &jobevents.Broadcaster{}
	runDone := make(chan error, 1)
	go func() { runDone <- b.Run(ctx, redisClient) }()
	time.Sleep(200 * time.Millisecond)

	stageEvents, unsubscribe := b.Subscribe(userID)
	defer unsubscribe()

	processor := &fakeProcessor{
		stages: []string{"downloading", "probing", "analyzing"},
		result: video.Result{Metadata: video.Metadata{Duration: 1, Width: 1, Height: 1, FPS: 1}},
	}

	processTick(ctx, sqlDB, redisClient, processor)

	// b.Subscribe(userID) delivers every event for this user on one
	// undifferentiated channel: processTick first publishes a
	// StatusChanged{"processing"} event before Process's onStage closure
	// fires, so the leading message here decodes into a StageChanged with
	// an empty Stage field (its JSON has no "stage" key). Skip that one and
	// collect exactly the 3 real stage events.
	var gotStages []string
	for len(gotStages) < 3 {
		select {
		case raw := <-stageEvents:
			var got jobevents.StageChanged
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", raw, err)
			}
			if got.Stage == "" {
				continue
			}
			if got.JobID != pendingJob.ID {
				t.Errorf("stage event JobID = %d, want %d", got.JobID, pendingJob.ID)
			}
			if got.UserID != userID {
				t.Errorf("stage event UserID = %d, want %d", got.UserID, userID)
			}
			gotStages = append(gotStages, got.Stage)
		case <-time.After(2 * time.Second):
			t.Fatalf("did not receive stage event #%d within 2s (got so far: %v)", len(gotStages)+1, gotStages)
		}
	}

	want := []string{"downloading", "probing", "analyzing"}
	if !reflect.DeepEqual(gotStages, want) {
		t.Errorf("stage events received = %v, want %v", gotStages, want)
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(1 * time.Second):
	}
}

// TestFailureSummary_CategorizesWithoutLeakingRawErrorText covers spec
// acceptance criterion 11 / plan.md step 9b: failureSummary categorizes the
// pipeline error into one of a small fixed set of human-readable reasons,
// across all 5 categories, and the returned text never contains the
// original error's raw text (embedding a unique marker string, mirroring
// what a real internal temp path or subprocess stderr snippet would leak).
func TestFailureSummary_CategorizesWithoutLeakingRawErrorText(t *testing.T) {
	const marker = "SECRET_TMP_PATH_MARKER"

	cases := []struct {
		name       string
		err        error
		wantSubstr string
	}{
		{
			name:       "corrupt download",
			err:        fmt.Errorf("video pipeline: download: %w: %s", video.ErrCorruptDownload, marker),
			wantSubstr: "incomplete or corrupt",
		},
		{
			name:       "too short video",
			err:        fmt.Errorf("video pipeline: attempt 1 failed (non-retryable): %w: %s", video.ErrVideoTooShort, marker),
			wantSubstr: "too short or corrupt to analyze",
		},
		{
			name:       "analysis timed out",
			err:        fmt.Errorf("video pipeline: analyze: %w: %s", context.DeadlineExceeded, marker),
			wantSubstr: "timed out",
		},
		{
			name:       "generic download failure",
			err:        fmt.Errorf("video pipeline: download: %s: %w", marker, errors.New("connection reset")),
			wantSubstr: "download failed",
		},
		{
			name:       "generic analyze failure",
			err:        fmt.Errorf("video pipeline: analyze: %s: %w", marker, errors.New("ffmpeg exited 1")),
			wantSubstr: "analysis failed",
		},
		{
			name:       "generic probe failure",
			err:        fmt.Errorf("video pipeline: probe: %s: %w", marker, errors.New("malformed ffprobe output")),
			wantSubstr: "analysis failed",
		},
		{
			name:       "generic validate failure (not ErrCorruptDownload)",
			err:        fmt.Errorf("video pipeline: validate: %s: %w", marker, errors.New("ffprobe exited 1")),
			wantSubstr: "analysis failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := failureSummary(tc.err)

			if !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("failureSummary(%v) = %q, want it to contain %q", tc.err, got, tc.wantSubstr)
			}
			if strings.Contains(got, marker) {
				t.Errorf("failureSummary(%v) = %q, leaked the raw error text marker %q", tc.err, got, marker)
			}
		})
	}
}

// TestLoadAnalyzerConfig covers plan.md step 9c: loadAnalyzerConfig parses
// MOTION_GRID_CELL_PX/MOTION_THRESHOLD_PER_PAIR/MOTION_MIN_REGION_CELLS/
// MAX_PARTICIPANTS from an injected getenv func, falling back to
// video.FFMPEGAnalyzer's documented zero-value defaults when a var is unset
// or unparseable — mirroring main()'s existing PROCESSING_TIMEOUT
// invalid-value handling (spec edge case 6).
func TestLoadAnalyzerConfig(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want video.FFMPEGAnalyzer
	}{
		{
			name: "all unset uses zero-value defaults",
			env:  map[string]string{},
			want: video.FFMPEGAnalyzer{},
		},
		{
			name: "all set to valid non-default values",
			env: map[string]string{
				"MOTION_GRID_CELL_PX":       "48",
				"MOTION_THRESHOLD_PER_PAIR": "22.5",
				"MOTION_MIN_REGION_CELLS":   "3",
				"MAX_PARTICIPANTS":          "1",
			},
			want: video.FFMPEGAnalyzer{
				GridCellPx:             48,
				MotionThresholdPerPair: 22.5,
				MinRegionCells:         3,
				MaxParticipants:        1,
			},
		},
		{
			name: "all invalid/unparseable falls back to zero-value defaults",
			env: map[string]string{
				"MOTION_GRID_CELL_PX":       "not-an-int",
				"MOTION_THRESHOLD_PER_PAIR": "not-a-float",
				"MOTION_MIN_REGION_CELLS":   "not-an-int",
				"MAX_PARTICIPANTS":          "not-an-int",
			},
			want: video.FFMPEGAnalyzer{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }

			got := loadAnalyzerConfig(getenv)
			if got != tc.want {
				t.Errorf("loadAnalyzerConfig(%v) = %+v, want %+v", tc.env, got, tc.want)
			}
		})
	}
}
