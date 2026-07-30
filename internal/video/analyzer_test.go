package video

// These tests exercise analyzeFrames — the decode+detect logic FFMPEGAnalyzer
// runs after ffmpeg has already extracted frames — directly against
// hand-written JPEG frame files, so the too-few-frames vs.
// zero-participants distinction can be verified without shelling out to a
// real ffmpeg subprocess (per plan.md step 3: the real Analyzer's I/O has no
// dedicated unit test of its own beyond this decode/detect seam).
//
// Reviewer finding (video-processing REQUEST_CHANGES): the two scenarios
// below must NOT be conflated:
//   - edge case 3 (spec.md): fewer than 2 usable frames after extraction is
//     a deterministic, non-retryable failure (ErrVideoTooShort).
//   - acceptance criterion 4 (spec.md): >=2 frames were extracted and
//     analyzed but zero motion regions were found — a valid success with
//     participants: [].

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// writeGrayJPEG writes a solid-fill width x height grayscale JPEG to path.
func writeGrayJPEG(t *testing.T, path string, width, height int, fill uint8) {
	t.Helper()

	img := image.NewGray(image.Rect(0, 0, width, height))
	for i := range img.Pix {
		img.Pix[i] = fill
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("jpeg encode %s: %v", path, err)
	}
}

// TestAnalyzeFrames_TooFewFramesIsNonRetryableError covers spec edge case 3:
// fewer than 2 usable frames after extraction must fail with a
// distinguishable, non-retryable error — not the empty-success result
// criterion 4 describes.
func TestAnalyzeFrames_TooFewFramesIsNonRetryableError(t *testing.T) {
	cases := []struct {
		name       string
		frameCount int
	}{
		{"zero frames", 0},
		{"one frame", 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for i := 0; i < tc.frameCount; i++ {
				writeGrayJPEG(t, filepath.Join(dir, fmt.Sprintf("frame-%04d.jpg", i)), 32, 32, 128)
			}

			result, err := analyzeFrames(dir)

			if err == nil {
				t.Fatalf("analyzeFrames with %d frame(s): got nil error, want ErrVideoTooShort", tc.frameCount)
			}
			if !errors.Is(err, ErrVideoTooShort) {
				t.Errorf("analyzeFrames with %d frame(s): err = %v, want errors.Is(err, ErrVideoTooShort)", tc.frameCount, err)
			}
			if len(result.Participants) != 0 {
				t.Errorf("result.Participants = %v, want empty on error", result.Participants)
			}
		})
	}
}

// TestAnalyzeFrames_NoMotionYieldsSuccessWithZeroParticipants covers
// acceptance criterion 4: once at least 2 frames were successfully extracted
// and analyzed, finding zero distinct moving regions is a valid success
// outcome (participants: []), not an error — must not be confused with the
// too-few-frames failure above.
func TestAnalyzeFrames_NoMotionYieldsSuccessWithZeroParticipants(t *testing.T) {
	dir := t.TempDir()

	// 3 identical frames (>=2, clearing the too-few-frames floor): zero
	// frame-to-frame pixel difference anywhere, so zero motion regions
	// should be detected.
	for i := 0; i < 3; i++ {
		writeGrayJPEG(t, filepath.Join(dir, fmt.Sprintf("frame-%04d.jpg", i)), 64, 64, 100)
	}

	result, err := analyzeFrames(dir)

	if err != nil {
		t.Fatalf("analyzeFrames: unexpected error: %v", err)
	}
	if result.Participants == nil {
		t.Error("result.Participants = nil, want a non-nil empty slice (criterion 4: participants: [])")
	}
	if len(result.Participants) != 0 {
		t.Errorf("len(result.Participants) = %d, want 0 (no motion between identical frames)", len(result.Participants))
	}
}
