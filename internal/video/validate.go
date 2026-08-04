package video

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ErrCorruptDownload indicates yt-dlp reported success but the resulting
// file is 0 bytes/missing or fails a basic ffprobe integrity check (spec
// Scope item 2, acceptance criterion 3) — a distinct category from both a
// generic download failure and ErrVideoTooShort. Unlike ErrVideoTooShort, a
// truncated/incomplete download is typically a transient network issue, so
// Pipeline.Run retries it under the normal 3-attempt/backoff policy instead
// of short-circuiting.
var ErrCorruptDownload = errors.New("video: downloaded file is missing, empty, or failed an integrity check")

// FFProbeValidator is the real Validator implementation: it rejects a
// 0-byte/missing file immediately via os.Stat (no subprocess needed), then
// runs a minimal ffprobe integrity check confirming the file is openable and
// reports at least one video stream.
type FFProbeValidator struct{}

// Validate confirms videoPath is a non-empty, ffprobe-openable file with at
// least one video stream. Either failure wraps ErrCorruptDownload.
func (FFProbeValidator) Validate(ctx context.Context, videoPath string) error {
	info, err := os.Stat(videoPath)
	if err != nil {
		return fmt.Errorf("video: validate: stat: %w: %w", ErrCorruptDownload, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("video: validate: %w: file is empty", ErrCorruptDownload)
	}

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type",
		videoPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("video: validate: ffprobe integrity check: %w: %s", ErrCorruptDownload, output)
	}
	if len(output) == 0 {
		return fmt.Errorf("video: validate: %w: no video stream found", ErrCorruptDownload)
	}

	return nil
}
