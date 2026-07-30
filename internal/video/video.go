// Package video implements the download/probe/analyze pipeline described in
// specs/video-processing/spec.md: given a job's youtube_url, it downloads
// the video, extracts technical metadata, and runs a lightweight
// motion-region detection pass to produce a per-participant activity score.
package video

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sethvargo/go-retry"
)

// Metadata holds the technical metadata extracted from a downloaded video
// file (spec Scope: "duration, resolution, and frame rate").
type Metadata struct {
	Duration float64 // seconds
	Width    int
	Height   int
	FPS      float64
}

// ParticipantResult is one detected participant's label and computed
// activity score (spec Scope: "a frame-difference-based 'activity score'").
type ParticipantResult struct {
	Label         string
	ActivityScore float64
}

// AnalysisResult is the outcome of a single Analyzer pass: every participant
// detected in the sampled frames.
type AnalysisResult struct {
	Participants []ParticipantResult
}

// Result is the outcome of a successful Pipeline.Run: the video's technical
// metadata plus every detected participant's activity score.
type Result struct {
	Metadata     Metadata
	Participants []ParticipantResult
}

// Downloader downloads the video at youtubeURL into destDir, returning the
// path to the downloaded file.
type Downloader interface {
	Download(ctx context.Context, youtubeURL, destDir string) (videoPath string, err error)
}

// Prober extracts technical metadata (duration/resolution/fps) from an
// already-downloaded video file.
type Prober interface {
	Probe(ctx context.Context, videoPath string) (Metadata, error)
}

// Analyzer samples frames from videoPath (using tmpDir for any extracted
// frame files) and detects participants/activity scores.
type Analyzer interface {
	Analyze(ctx context.Context, videoPath, tmpDir string) (AnalysisResult, error)
}

// maxAttempts is the total number of processing attempts per job: 1 initial
// attempt plus 2 retries (spec Scope: "retried up to 3 total attempts").
const maxAttempts = 3

// Pipeline orchestrates one job's download -> probe -> analyze attempt(s),
// retrying transient failures with exponential backoff, bounding each
// attempt with a timeout, and always cleaning up its per-attempt temp
// directory before returning from that attempt.
type Pipeline struct {
	Downloader Downloader
	Prober     Prober
	Analyzer   Analyzer

	// BackoffBase is the first retry's backoff duration; the second retry's
	// backoff is 2x this value (spec Scope: "1s, then 2s"). Production wires
	// 1 second; tests inject a sub-millisecond value to stay fast.
	BackoffBase time.Duration

	// Timeout bounds each individual attempt via context.WithTimeout.
	// Production default is 10 minutes (read from PROCESSING_TIMEOUT).
	Timeout time.Duration

	// Sleep is called with each computed backoff duration between attempts.
	// A nil Sleep defaults to the real time.Sleep; tests always inject a
	// recording fake so no real sleep ever happens.
	Sleep func(time.Duration)

	// TempDirRoot is the parent directory under which each attempt's
	// temporary directory is created. Empty defaults to os.TempDir().
	TempDirRoot string
}

// Run executes the download -> probe -> analyze pipeline for youtubeURL, up
// to maxAttempts times with exponential backoff between failed attempts
// (spec Scope: "Retry with exponential backoff"). It returns the first
// successful attempt's Result, or a wrapped error if every attempt fails.
func (p *Pipeline) Run(ctx context.Context, youtubeURL string) (Result, error) {
	sleep := p.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}

	backoff := retry.NewExponential(p.durationOrDefault())

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := p.runAttempt(ctx, youtubeURL)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if attempt == maxAttempts {
			break
		}

		d, _ := backoff.Next()
		sleep(d)
	}

	return Result{}, fmt.Errorf("video pipeline: all %d attempts failed: %w", maxAttempts, lastErr)
}

// durationOrDefault guards against retry.NewExponential's panic on a
// non-positive base (a zero-value Pipeline.BackoffBase would otherwise crash
// Run instead of returning an error).
func (p *Pipeline) durationOrDefault() time.Duration {
	if p.BackoffBase <= 0 {
		return time.Nanosecond
	}
	return p.BackoffBase
}

// runAttempt performs one full download -> probe -> analyze attempt, bounded
// by a per-attempt context.WithTimeout, inside a freshly created temp
// directory that is removed before this function returns on every exit path
// — success, failure, timeout, or panic (spec Constraints: "Resource
// management").
func (p *Pipeline) runAttempt(ctx context.Context, youtubeURL string) (result Result, err error) {
	tempDir, mkErr := os.MkdirTemp(p.TempDirRoot, "video-job-*")
	if mkErr != nil {
		return Result{}, fmt.Errorf("video pipeline: create temp dir: %w", mkErr)
	}
	defer func() {
		if rmErr := os.RemoveAll(tempDir); rmErr != nil && err == nil {
			err = fmt.Errorf("video pipeline: remove temp dir: %w", rmErr)
		}
	}()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("video pipeline: panic during attempt: %v", r)
		}
	}()

	attemptCtx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	videoPath, dlErr := p.Downloader.Download(attemptCtx, youtubeURL, tempDir)
	if dlErr != nil {
		return Result{}, fmt.Errorf("video pipeline: download: %w", dlErr)
	}

	metadata, probeErr := p.Prober.Probe(attemptCtx, videoPath)
	if probeErr != nil {
		return Result{}, fmt.Errorf("video pipeline: probe: %w", probeErr)
	}

	analysis, analyzeErr := p.Analyzer.Analyze(attemptCtx, videoPath, tempDir)
	if analyzeErr != nil {
		return Result{}, fmt.Errorf("video pipeline: analyze: %w", analyzeErr)
	}

	return Result{Metadata: metadata, Participants: analysis.Participants}, nil
}
