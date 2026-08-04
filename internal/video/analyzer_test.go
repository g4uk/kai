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
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"sort"
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

// ----------------------------------------------------------------------------
// TDD RED PHASE NOTE (specs/video-processing-improvements/plan.md step 1)
//
// The tests below drive two changes to internal/video/analyzer.go that don't
// exist yet, so this package fails to compile until analyzer.go is updated
// (expected, correct red state):
//
//   - FFMPEGAnalyzer gains three new fields -- GridCellPx int,
//     MotionThresholdPerPair float64, MinRegionCells int -- each defaulting
//     to today's hardcoded constants (32/15.0/2) when zero, plus
//     MaxParticipants int (default 2, spec criterion 10).
//   - detectParticipants's signature changes from
//     detectParticipants(frames []*image.Gray) []ParticipantResult to
//     detectParticipants(frames []*image.Gray, cfg FFMPEGAnalyzer) []ParticipantResult,
//     using cfg's four fields above to drive grid size, per-pair motion
//     threshold, minimum region size, and (new) a persistence-score-ranked
//     top-N cap.
// ----------------------------------------------------------------------------

// newStripedFrame builds a width x height grayscale image split into
// vertical stripes, each stripeWidth px wide, with the i-th stripe filled
// uniformly with vals[i]. len(vals) must equal width/stripeWidth.
func newStripedFrame(width, height, stripeWidth int, vals []uint8) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			col := x / stripeWidth
			img.SetGray(x, y, color.Gray{Y: vals[col]})
		}
	}
	return img
}

// scoreSet extracts and sorts every participant's ActivityScore, for
// order-independent comparison -- detectParticipants's label/order for
// selected regions is not part of what these tests pin down.
func scoreSet(participants []ParticipantResult) []float64 {
	scores := make([]float64, len(participants))
	for i, p := range participants {
		scores[i] = p.ActivityScore
	}
	sort.Float64s(scores)
	return scores
}

func assertScoreSet(t *testing.T, got []ParticipantResult, want []float64) {
	t.Helper()

	wantSorted := append([]float64(nil), want...)
	sort.Float64s(wantSorted)
	gotScores := scoreSet(got)

	if len(gotScores) != len(wantSorted) {
		t.Fatalf("participant scores = %v, want %v", gotScores, wantSorted)
	}
	for i := range wantSorted {
		if gotScores[i] != wantSorted[i] {
			t.Fatalf("participant scores = %v, want %v", gotScores, wantSorted)
		}
	}
}

// TestFFMPEGAnalyzer_CustomThresholdsChangeDetectedCells covers spec
// criterion 5 / plan.md step 1a: FFMPEGAnalyzer{GridCellPx,
// MotionThresholdPerPair, MinRegionCells} set to non-default values changes
// which grid cells (and therefore which regions) are flagged as moving,
// exercising each of the three fields independently.
func TestFFMPEGAnalyzer_CustomThresholdsChangeDetectedCells(t *testing.T) {
	t.Run("custom GridCellPx enables finer-grained region detection", func(t *testing.T) {
		// 1 frame pair, 64x16px: logical 16px-wide columns 0,1 jump by 20
		// (>= today's default per-pair threshold of 15.0); columns 2,3 stay
		// still. Under the default 32px grid, columns 0+1 fall inside a
		// single 32px cell -- a lone moving cell, filtered out by the
		// default MinRegionCells=2. Under a custom 16px grid, they become
		// two separate, adjacent moving cells -- a 2-cell region, kept.
		frames := []*image.Gray{
			newStripedFrame(64, 16, 16, []uint8{100, 100, 100, 100}),
			newStripedFrame(64, 16, 16, []uint8{120, 120, 100, 100}),
		}

		defaultResult := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 2})
		if len(defaultResult) != 0 {
			t.Errorf("default GridCellPx (32): got %d participants, want 0", len(defaultResult))
		}

		customResult := detectParticipants(frames, FFMPEGAnalyzer{GridCellPx: 16, MaxParticipants: 2})
		if len(customResult) != 1 {
			t.Fatalf("custom GridCellPx=16: got %d participants, want 1", len(customResult))
		}
		if customResult[0].ActivityScore != 40 {
			t.Errorf("custom GridCellPx=16: ActivityScore = %v, want 40", customResult[0].ActivityScore)
		}
	})

	t.Run("lowered MotionThresholdPerPair flags a subtler movement", func(t *testing.T) {
		// 1 frame pair, 64x32px (2 default-size 32px columns): both columns
		// jump by 10 -- below today's default 15.0 per-pair threshold, so
		// nothing is flagged by default. A lowered custom threshold flags
		// both (now-adjacent, 2-cell) columns as moving.
		frames := []*image.Gray{
			newStripedFrame(64, 32, 32, []uint8{100, 100}),
			newStripedFrame(64, 32, 32, []uint8{110, 110}),
		}

		defaultResult := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 2})
		if len(defaultResult) != 0 {
			t.Errorf("default MotionThresholdPerPair (15.0): got %d participants, want 0", len(defaultResult))
		}

		customResult := detectParticipants(frames, FFMPEGAnalyzer{MotionThresholdPerPair: 5.0, MaxParticipants: 2})
		if len(customResult) != 1 {
			t.Fatalf("custom MotionThresholdPerPair=5.0: got %d participants, want 1", len(customResult))
		}
		if customResult[0].ActivityScore != 20 {
			t.Errorf("custom MotionThresholdPerPair=5.0: ActivityScore = %v, want 20", customResult[0].ActivityScore)
		}
	})

	t.Run("lowered MinRegionCells keeps a single-cell region", func(t *testing.T) {
		// 1 frame pair, 64x32px (2 default-size 32px columns): only column 0
		// jumps by 20 (>= default threshold); column 1 stays still, so at
		// most a single moving cell is ever detected -- filtered out by the
		// default MinRegionCells=2, kept when lowered to 1.
		frames := []*image.Gray{
			newStripedFrame(64, 32, 32, []uint8{100, 100}),
			newStripedFrame(64, 32, 32, []uint8{120, 100}),
		}

		defaultResult := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 2})
		if len(defaultResult) != 0 {
			t.Errorf("default MinRegionCells (2): got %d participants, want 0", len(defaultResult))
		}

		customResult := detectParticipants(frames, FFMPEGAnalyzer{MinRegionCells: 1, MaxParticipants: 2})
		if len(customResult) != 1 {
			t.Fatalf("custom MinRegionCells=1: got %d participants, want 1", len(customResult))
		}
		if customResult[0].ActivityScore != 20 {
			t.Errorf("custom MinRegionCells=1: ActivityScore = %v, want 20", customResult[0].ActivityScore)
		}
	})
}

// TestFFMPEGAnalyzer_ZeroValueFieldsMatchTodaysDefaults covers spec
// criterion 6 / plan.md step 1b: leaving GridCellPx/MotionThresholdPerPair/
// MinRegionCells at their zero values must reproduce today's exact behavior
// (32px/15.0/2) -- proven by asserting the zero-value result is identical to
// an explicit 32/15.0/2 config's result, not merely "also non-empty."
func TestFFMPEGAnalyzer_ZeroValueFieldsMatchTodaysDefaults(t *testing.T) {
	frames := []*image.Gray{
		newStripedFrame(64, 32, 32, []uint8{100, 100}),
		newStripedFrame(64, 32, 32, []uint8{120, 120}),
	}

	zeroValue := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 2})
	explicit := detectParticipants(frames, FFMPEGAnalyzer{
		GridCellPx:             32,
		MotionThresholdPerPair: 15.0,
		MinRegionCells:         2,
		MaxParticipants:        2,
	})

	if !reflect.DeepEqual(zeroValue, explicit) {
		t.Fatalf("zero-value fields produced %+v, want identical result to explicit 32/15.0/2: %+v", zeroValue, explicit)
	}
	if len(zeroValue) != 1 || zeroValue[0].ActivityScore != 40 {
		t.Fatalf("zero-value result = %+v, want exactly 1 participant with ActivityScore 40", zeroValue)
	}
}

// buildThreeRegionFrames returns 4 frames (3 frame-pairs) laid out as 8
// default-size (32px) grid columns x 1 row: region A (cols 0-1) is active in
// every pair, region B (cols 3-4) is active in 2 of 3 pairs, and region C
// (cols 6-7) is still except for one huge spike in the middle pair --
// carefully tuned so C's raw accumulated activity sum (400) is higher than
// *both* sustained regions' sums (A=120, B=100), proving a pure
// accumulated-sum ranking would get top-N selection wrong (it would keep C
// over A and/or B), while a persistence-score ranking (A=1.0, B=0.667,
// C=0.333) correctly favors the sustained regions and demotes the bursty
// one. Columns 2 and 5 are constant gaps so the three regions never
// 4-connect into one blob.
func buildThreeRegionFrames() []*image.Gray {
	// col:       0    1    2    3    4    5    6    7
	//         [--A--][gap][--B--][gap][--C--]
	frame0 := []uint8{100, 100, 100, 100, 100, 100, 10, 10}
	frame1 := []uint8{120, 120, 100, 125, 125, 100, 10, 10}
	frame2 := []uint8{100, 100, 100, 150, 150, 100, 210, 210}
	frame3 := []uint8{120, 120, 100, 150, 150, 100, 210, 210}

	return []*image.Gray{
		newStripedFrame(256, 32, 32, frame0),
		newStripedFrame(256, 32, 32, frame1),
		newStripedFrame(256, 32, 32, frame2),
		newStripedFrame(256, 32, 32, frame3),
	}
}

// TestDetectParticipants_TopNByPersistenceBeatsRawAccumulatedSum covers spec
// criterion 7 and the primary motivating case (spec edge case 1) / plan.md
// step 1c: with MaxParticipants=2, detectParticipants must keep the two
// *sustained* regions (A, B) and drop the bursty one (C), even though C's
// raw accumulated activity sum is higher than a sustained region's sum --
// the case a pure accumulated-sum ranking gets wrong.
func TestDetectParticipants_TopNByPersistenceBeatsRawAccumulatedSum(t *testing.T) {
	frames := buildThreeRegionFrames()

	// Sanity check on the test data itself (plan.md: "construct the pixel
	// data carefully so this is actually true, not just asserted"): with no
	// effective cap, all 3 regions are detected, and the bursty region's
	// accumulated sum (400) really is higher than a sustained region's sum
	// (B=100).
	all := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 3})
	assertScoreSet(t, all, []float64{120, 100, 400})
	const burstySum, sustainedBSum = 400.0, 100.0
	if burstySum <= sustainedBSum {
		t.Fatalf("test data invariant violated: bursty region's sum (%v) must be higher than a sustained region's sum (%v)", burstySum, sustainedBSum)
	}

	got := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 2})
	assertScoreSet(t, got, []float64{120, 100})
	for _, p := range got {
		if p.ActivityScore == 400 {
			t.Errorf("kept the bursty region (score 400, persistence 0.333) over a sustained region (persistence 1.0/0.667); got %+v", got)
		}
	}
}

// TestDetectParticipants_SingleRegionUnderCapReturnedUnchanged covers spec
// criterion 8 / edge case 3 / plan.md step 1d: with only 1 region detected
// and MaxParticipants=2, that region is returned unchanged -- the cap never
// invents participants that weren't detected.
func TestDetectParticipants_SingleRegionUnderCapReturnedUnchanged(t *testing.T) {
	frames := []*image.Gray{
		newStripedFrame(64, 32, 32, []uint8{100, 100}),
		newStripedFrame(64, 32, 32, []uint8{120, 120}),
		newStripedFrame(64, 32, 32, []uint8{100, 100}),
		newStripedFrame(64, 32, 32, []uint8{120, 120}),
	}

	got := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 2})
	if len(got) != 1 {
		t.Fatalf("len(participants) = %d, want 1 (cap must never invent participants)", len(got))
	}
	if got[0].ActivityScore != 120 {
		t.Errorf("ActivityScore = %v, want 120", got[0].ActivityScore)
	}
}

// TestDetectParticipants_MaxParticipantsOneKeepsOnlyTopPersistence covers
// spec criterion 9 / plan.md step 1e: with 3 detected regions and
// MaxParticipants=1, exactly 1 region -- the top-scoring one by persistence
// -- is kept.
func TestDetectParticipants_MaxParticipantsOneKeepsOnlyTopPersistence(t *testing.T) {
	frames := buildThreeRegionFrames()

	got := detectParticipants(frames, FFMPEGAnalyzer{MaxParticipants: 1})
	if len(got) != 1 {
		t.Fatalf("len(participants) = %d, want exactly 1", len(got))
	}
	// Region A (persistence 1.0, active every pair) unambiguously outranks
	// region B (persistence 0.667) and region C (persistence 0.333) -- no
	// tie-break ambiguity.
	if got[0].ActivityScore != 120 {
		t.Errorf("kept participant's ActivityScore = %v, want 120 (region A, the highest-persistence region)", got[0].ActivityScore)
	}
}

// TestDetectParticipants_MaxParticipantsZeroDefaultsToTwo covers spec
// criterion 10 / plan.md step 1f: MaxParticipants left at its zero value
// defaults to 2, keeping today's "2 fighters" default cap instead of the
// old "keep every detected region" behavior.
func TestDetectParticipants_MaxParticipantsZeroDefaultsToTwo(t *testing.T) {
	frames := buildThreeRegionFrames()

	got := detectParticipants(frames, FFMPEGAnalyzer{}) // MaxParticipants zero value
	assertScoreSet(t, got, []float64{120, 100})
}
