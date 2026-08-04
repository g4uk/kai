package video

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestSmoke_RealPipeline exercises the real Downloader+Prober+Analyzer
// end to end against a short, fixed public YouTube test video. It is gated
// on TEST_YTDLP=1 (same opt-in convention as TEST_DSN/TEST_REDIS_ADDR) and
// never runs as part of the default `go test ./...` pass — it invokes real
// yt-dlp/ffmpeg subprocesses and makes a real network call.
func TestSmoke_RealPipeline(t *testing.T) {
	if os.Getenv("TEST_YTDLP") != "1" {
		t.Skip("TEST_YTDLP not set to 1; skipping real yt-dlp/ffmpeg smoke test")
	}

	p := &Pipeline{
		Downloader:  YTDLPDownloader{},
		Prober:      FFProbeProber{},
		Analyzer:    FFMPEGAnalyzer{},
		BackoffBase: 1 * time.Second,
		Timeout:     2 * time.Minute,
		TempDirRoot: t.TempDir(),
	}

	// "Me at the zoo" (jNQXAC9IVRw) — YouTube's first-ever upload, ~19s,
	// stable and unlikely to be taken down; a good fixed short test video.
	result, err := p.Run(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw", nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Metadata.Duration <= 0 {
		t.Errorf("result.Metadata.Duration = %v, want > 0", result.Metadata.Duration)
	}
	t.Logf("smoke result: %+v", result)
}
