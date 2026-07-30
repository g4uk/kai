// Package video will hold the download/probe/analyze pipeline described in
// specs/video-processing/spec.md.
package video

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/video-processing/plan.md step 1)
//
// internal/video/video.go does not exist yet. This test file references the
// following production identifiers that video.go must define; until it
// does, this package fails to compile (expected, correct red state):
//
//	type Metadata struct {
//	    Duration float64 // seconds
//	    Width    int
//	    Height   int
//	    FPS      float64
//	}
//
//	type ParticipantResult struct {
//	    Label         string
//	    ActivityScore float64
//	}
//
//	type AnalysisResult struct {
//	    Participants []ParticipantResult
//	}
//
//	type Result struct {
//	    Metadata     Metadata
//	    Participants []ParticipantResult
//	}
//
//	type Downloader interface {
//	    Download(ctx context.Context, youtubeURL, destDir string) (videoPath string, err error)
//	}
//
//	type Prober interface {
//	    Probe(ctx context.Context, videoPath string) (Metadata, error)
//	}
//
//	type Analyzer interface {
//	    Analyze(ctx context.Context, videoPath, tmpDir string) (AnalysisResult, error)
//	}
//
//	type Pipeline struct {
//	    Downloader  Downloader
//	    Prober      Prober
//	    Analyzer    Analyzer
//	    BackoffBase time.Duration          // production: 1s; tests inject a tiny value
//	    Timeout     time.Duration          // per-attempt timeout; production default 10m
//	    Sleep       func(time.Duration)    // backoff hook; nil => real time.Sleep. Tests
//	                                       // always inject a recording fake so no real
//	                                       // sleep ever happens, per plan.md step 1's
//	                                       // "fake clock/sleep func" note.
//	    TempDirRoot string                 // parent dir for per-attempt temp dirs;
//	                                       // empty => os.TempDir(). Tests inject
//	                                       // t.TempDir() so cleanup is verifiable.
//	}
//
//	func (p *Pipeline) Run(ctx context.Context, youtubeURL string) (Result, error)
//
// Retry policy (spec Scope): up to 3 total attempts (1 initial + 2 retries).
// Each attempt: Download -> Probe -> Analyze, bounded by a per-attempt
// context.WithTimeout(ctx, p.Timeout). Between a failed attempt N and
// attempt N+1, Pipeline calls p.Sleep(backoff) where backoff is
// p.BackoffBase * 2^(N-1) (1x, then 2x, matching spec's "1s, then 2s").
// Pipeline creates a fresh per-attempt temp directory (inside TempDirRoot)
// for Download/Analyze to use and removes it before returning from that
// attempt (success, failure, or timeout) — so a later retry never inherits a
// prior attempt's files (edge case 7), and nothing survives after Run
// returns (criterion 7), verified below via os.ReadDir on TempDirRoot.
// ----------------------------------------------------------------------------

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// ---- fakes ----------------------------------------------------------------

// fakeDownloader scripts one result per call, indexed by call order. If more
// calls happen than scripted results, it returns an error naming the
// over-call so a test failure is easy to diagnose instead of panicking.
type fakeDownloader struct {
	calls int
	paths []string
	errs  []error
}

func (f *fakeDownloader) Download(_ context.Context, _, _ string) (string, error) {
	i := f.calls
	f.calls++
	if i >= len(f.errs) {
		return "", errors.New("fakeDownloader: no scripted result for this call")
	}
	if f.errs[i] != nil {
		return "", f.errs[i]
	}
	return f.paths[i], nil
}

// fakeProber always returns the same Metadata on success; it is only ever
// expected to be called on attempts that got past Download.
type fakeProber struct {
	calls    int
	metadata Metadata
	err      error
}

func (f *fakeProber) Probe(_ context.Context, _ string) (Metadata, error) {
	f.calls++
	if f.err != nil {
		return Metadata{}, f.err
	}
	return f.metadata, nil
}

// fakeAnalyzer always returns the same AnalysisResult on success.
type fakeAnalyzer struct {
	calls  int
	result AnalysisResult
	err    error
}

func (f *fakeAnalyzer) Analyze(_ context.Context, _, _ string) (AnalysisResult, error) {
	f.calls++
	if f.err != nil {
		return AnalysisResult{}, f.err
	}
	return f.result, nil
}

// blockingAnalyzer simulates an attempt that takes longer than the
// per-attempt timeout: it is a well-behaved fake that honors ctx
// cancellation (mirroring how the real ffmpeg-subprocess-backed Analyzer
// will be driven via exec.CommandContext), so Pipeline's
// context.WithTimeout is what ends the attempt, not any special-casing in
// the fake itself.
type blockingAnalyzer struct {
	calls    int
	blockFor time.Duration
}

func (f *blockingAnalyzer) Analyze(ctx context.Context, _, _ string) (AnalysisResult, error) {
	f.calls++
	select {
	case <-time.After(f.blockFor):
		return AnalysisResult{}, nil
	case <-ctx.Done():
		return AnalysisResult{}, ctx.Err()
	}
}

// recordingSleep returns a Sleep hook that appends every requested duration
// to *record without ever actually sleeping, so backoff sequencing can be
// asserted without slowing the test suite down (plan.md step 1: "never real
// time.Sleep").
func recordingSleep(record *[]time.Duration) func(time.Duration) {
	return func(d time.Duration) {
		*record = append(*record, d)
	}
}

// assertTempDirCleanedUp fails the test if root still contains any entries,
// proving Pipeline removed every per-attempt temp dir it created inside root
// (criterion 7 / edge case 7).
func assertTempDirCleanedUp(t *testing.T, root string) {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("os.ReadDir(%q): %v", root, err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("temp dir root %q has %d leftover entries after Run returned, want 0: %v", root, len(entries), names)
	}
}

// ---- table-driven: success / transient-failure / all-fail ------------------

func TestPipeline_Run(t *testing.T) {
	const base = 1 * time.Millisecond

	successMetadata := Metadata{Duration: 65.5, Width: 1920, Height: 1080, FPS: 29.97}
	successAnalysis := AnalysisResult{Participants: []ParticipantResult{
		{Label: "Participant 1", ActivityScore: 12.3},
		{Label: "Participant 2", ActivityScore: 7.1},
	}}

	transientErr := errors.New("simulated transient network error")

	cases := []struct {
		name string

		downloader *fakeDownloader
		prober     *fakeProber
		analyzer   *fakeAnalyzer

		wantErr           bool
		wantDownloadCalls int
		wantProbeCalls    int
		wantAnalyzeCalls  int
		wantBackoffs      []time.Duration
		wantParticipants  int
	}{
		{
			name: "success on first attempt",
			downloader: &fakeDownloader{
				paths: []string{"/tmp/attempt-0/video.mp4"},
				errs:  []error{nil},
			},
			prober:            &fakeProber{metadata: successMetadata},
			analyzer:          &fakeAnalyzer{result: successAnalysis},
			wantErr:           false,
			wantDownloadCalls: 1,
			wantProbeCalls:    1,
			wantAnalyzeCalls:  1,
			wantBackoffs:      nil,
			wantParticipants:  2,
		},
		{
			name: "transient failure then success",
			downloader: &fakeDownloader{
				paths: []string{"", "/tmp/attempt-1/video.mp4"},
				errs:  []error{transientErr, nil},
			},
			prober:            &fakeProber{metadata: successMetadata},
			analyzer:          &fakeAnalyzer{result: successAnalysis},
			wantErr:           false,
			wantDownloadCalls: 2,
			wantProbeCalls:    1,
			wantAnalyzeCalls:  1,
			wantBackoffs:      []time.Duration{base},
			wantParticipants:  2,
		},
		{
			name: "all 3 attempts fail",
			downloader: &fakeDownloader{
				paths: []string{"", "", ""},
				errs:  []error{transientErr, transientErr, transientErr},
			},
			prober:            &fakeProber{metadata: successMetadata},
			analyzer:          &fakeAnalyzer{result: successAnalysis},
			wantErr:           true,
			wantDownloadCalls: 3,
			wantProbeCalls:    0,
			wantAnalyzeCalls:  0,
			wantBackoffs:      []time.Duration{base, 2 * base},
			wantParticipants:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tempRoot := t.TempDir()
			var sleeps []time.Duration

			p := &Pipeline{
				Downloader:  tc.downloader,
				Prober:      tc.prober,
				Analyzer:    tc.analyzer,
				BackoffBase: base,
				Timeout:     1 * time.Second,
				Sleep:       recordingSleep(&sleeps),
				TempDirRoot: tempRoot,
			}

			result, err := p.Run(context.Background(), "https://www.youtube.com/watch?v=pipeline-test")

			if tc.wantErr {
				if err == nil {
					t.Error("Run: got nil error, want non-nil (all attempts should have failed)")
				}
				if result.Metadata != (Metadata{}) {
					t.Errorf("Run on all-attempts-fail: result.Metadata = %+v, want zero value (no partial writes)", result.Metadata)
				}
				if len(result.Participants) != 0 {
					t.Errorf("Run on all-attempts-fail: len(result.Participants) = %d, want 0 (no partial writes)", len(result.Participants))
				}
			} else if err != nil {
				t.Fatalf("Run: unexpected error: %v", err)
			}

			if tc.downloader.calls != tc.wantDownloadCalls {
				t.Errorf("downloader.calls = %d, want %d", tc.downloader.calls, tc.wantDownloadCalls)
			}
			if tc.prober.calls != tc.wantProbeCalls {
				t.Errorf("prober.calls = %d, want %d", tc.prober.calls, tc.wantProbeCalls)
			}
			if tc.analyzer.calls != tc.wantAnalyzeCalls {
				t.Errorf("analyzer.calls = %d, want %d", tc.analyzer.calls, tc.wantAnalyzeCalls)
			}

			if len(sleeps) != len(tc.wantBackoffs) {
				t.Fatalf("len(sleeps) = %d (%v), want %d (%v)", len(sleeps), sleeps, len(tc.wantBackoffs), tc.wantBackoffs)
			}
			for i, want := range tc.wantBackoffs {
				if sleeps[i] != want {
					t.Errorf("sleeps[%d] = %v, want %v", i, sleeps[i], want)
				}
			}

			if !tc.wantErr {
				if len(result.Participants) != tc.wantParticipants {
					t.Errorf("len(result.Participants) = %d, want %d", len(result.Participants), tc.wantParticipants)
				}
				if result.Metadata != successMetadata {
					t.Errorf("result.Metadata = %+v, want %+v", result.Metadata, successMetadata)
				}
			}

			assertTempDirCleanedUp(t, tempRoot)
		})
	}
}

// ---- per-attempt timeout ----------------------------------------------------

func TestPipeline_Run_PerAttemptTimeoutExceeded(t *testing.T) {
	const base = 1 * time.Millisecond
	const timeout = 5 * time.Millisecond
	const blockFor = 200 * time.Millisecond // far longer than timeout, every attempt

	tempRoot := t.TempDir()
	var sleeps []time.Duration

	downloader := &fakeDownloader{
		paths: []string{"/tmp/a0/video.mp4", "/tmp/a1/video.mp4", "/tmp/a2/video.mp4"},
		errs:  []error{nil, nil, nil},
	}
	prober := &fakeProber{metadata: Metadata{Duration: 10, Width: 640, Height: 480, FPS: 24}}
	analyzer := &blockingAnalyzer{blockFor: blockFor}

	p := &Pipeline{
		Downloader:  downloader,
		Prober:      prober,
		Analyzer:    analyzer,
		BackoffBase: base,
		Timeout:     timeout,
		Sleep:       recordingSleep(&sleeps),
		TempDirRoot: tempRoot,
	}

	start := time.Now()
	result, err := p.Run(context.Background(), "https://www.youtube.com/watch?v=pipeline-timeout")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Run: got nil error, want non-nil (every attempt should have timed out)")
	}
	if len(result.Participants) != 0 {
		t.Errorf("len(result.Participants) = %d, want 0 on a fully-timed-out run", len(result.Participants))
	}

	if downloader.calls != 3 {
		t.Errorf("downloader.calls = %d, want 3 (a per-attempt timeout is retried like any other failure)", downloader.calls)
	}
	if analyzer.calls != 3 {
		t.Errorf("analyzer.calls = %d, want 3", analyzer.calls)
	}

	wantBackoffs := []time.Duration{base, 2 * base}
	if len(sleeps) != len(wantBackoffs) {
		t.Fatalf("len(sleeps) = %d (%v), want %d (%v)", len(sleeps), sleeps, len(wantBackoffs), wantBackoffs)
	}
	for i, want := range wantBackoffs {
		if sleeps[i] != want {
			t.Errorf("sleeps[%d] = %v, want %v", i, sleeps[i], want)
		}
	}

	// Each attempt must be bounded by Timeout, not by blockFor: 3 attempts at
	// ~timeout each, plus the (recorded-but-not-real) backoffs, should finish
	// in well under blockFor. This guards against a Pipeline implementation
	// that ignores the per-attempt context.WithTimeout and just waits out the
	// fake's full blockFor on every attempt.
	if elapsed >= blockFor {
		t.Errorf("Run took %v, want well under blockFor (%v) — per-attempt timeout was not enforced", elapsed, blockFor)
	}

	assertTempDirCleanedUp(t, tempRoot)
}
