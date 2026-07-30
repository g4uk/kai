package video

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

// ErrVideoTooShort indicates a downloaded video yielded fewer than 2 usable
// sampled frames after extraction — a deterministic, non-retryable outcome
// (spec edge case 3: "Video too short/corrupt to analyze"), distinct from
// acceptance criterion 4's "zero participants detected" case. Criterion 4
// only applies once at least 2 frames were successfully extracted and
// analyzed but no motion was found between them; ErrVideoTooShort covers the
// earlier, unrecoverable case where there weren't even enough frames to
// compare in the first place. Pipeline.Run recognizes this error (via
// errors.Is) and fails the job immediately on attempt 1 instead of retrying
// (retrying cannot change a too-short/corrupt video's outcome).
var ErrVideoTooShort = errors.New("video: too few usable frames to analyze (need at least 2)")

// FFMPEGAnalyzer is the real Analyzer implementation: it shells out to
// ffmpeg to extract frames at a fixed sample rate, decodes them with the
// pure-Go image/jpeg package (no CGO, no gocv/OpenCV bindings, per spec
// Non-scope), and runs a lightweight, non-ML motion-region detection pass
// (coarse grid + frame-to-frame pixel differencing) over the decoded frames
// to produce each detected participant's activity score.
type FFMPEGAnalyzer struct {
	// SampleFPS is the frame-extraction rate passed to ffmpeg's fps filter.
	// Zero defaults to 1 (one frame per second), bounding decode work
	// independent of video length/resolution (plan.md Risks #3).
	SampleFPS float64
}

const (
	// gridCell is the side length (in pixels) of each cell in the coarse grid
	// used for motion-region detection.
	gridCell = 32
	// motionThreshold is the minimum average per-pixel luma difference
	// (0-255 scale), summed across every consecutive frame pair, for a grid
	// cell to be considered part of a moving region.
	motionThresholdPerPair = 15.0
	// minRegionCells is the minimum number of connected moving cells for a
	// region to count as a distinct participant, filtering out single-cell
	// noise/flicker.
	minRegionCells = 2
)

// Analyze extracts frames from videoPath into tmpDir via ffmpeg, decodes
// them, and detects participants/activity scores via frame-to-frame
// pixel-diff motion detection.
func (a FFMPEGAnalyzer) Analyze(ctx context.Context, videoPath, tmpDir string) (AnalysisResult, error) {
	sampleFPS := a.SampleFPS
	if sampleFPS <= 0 {
		sampleFPS = 1
	}

	framesDir := filepath.Join(tmpDir, "frames")
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return AnalysisResult{}, fmt.Errorf("video: analyze: create frames dir: %w", err)
	}

	pattern := filepath.Join(framesDir, "frame-%04d.jpg")
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%g", sampleFPS),
		pattern,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return AnalysisResult{}, fmt.Errorf("video: analyze: ffmpeg frame extraction: %w: %s", err, output)
	}

	return analyzeFrames(framesDir)
}

// analyzeFrames decodes every frame file in framesDir and runs
// participant/motion detection over them. Extracted from Analyze so the
// decode/detect decision logic (in particular, the too-few-frames vs.
// zero-participants distinction) is directly unit-testable without shelling
// out to ffmpeg.
func analyzeFrames(framesDir string) (AnalysisResult, error) {
	frames, err := decodeFrames(framesDir)
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("video: analyze: decode frames: %w", err)
	}
	if len(frames) < 2 {
		// Deterministic, not transient (spec edge case 3): a video that
		// yields fewer than 2 usable frames cannot be fixed by retrying, so
		// this is a hard failure distinguishable from criterion 4's "zero
		// participants after analyzing >=2 frames" success case, and from a
		// generic network/timeout failure.
		return AnalysisResult{}, fmt.Errorf("video: analyze: %w", ErrVideoTooShort)
	}

	// >=2 frames were successfully extracted and analyzed; zero detected
	// motion regions here is criterion 4's valid "zero participants" case,
	// not an error — detectParticipants already returns an empty (non-nil)
	// slice in that case.
	return AnalysisResult{Participants: detectParticipants(frames)}, nil
}

// decodeFrames reads every file in dir, in name-sorted (i.e. extraction)
// order, decoding each as a JPEG into grayscale via the pure-Go image/jpeg
// package.
func decodeFrames(dir string) ([]*image.Gray, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	frames := make([]*image.Gray, 0, len(names))
	for _, name := range names {
		frame, err := decodeFrameFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("decode %s: %w", name, err)
		}
		frames = append(frames, frame)
	}
	return frames, nil
}

func decodeFrameFile(path string) (*image.Gray, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }()

	img, err := jpeg.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("jpeg decode: %w", err)
	}
	return toGray(img), nil
}

// toGray converts img to grayscale (a no-op copy if it already is one).
func toGray(img image.Image) *image.Gray {
	if g, ok := img.(*image.Gray); ok {
		return g
	}
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, img.At(x, y))
		}
	}
	return gray
}

// gridPoint identifies one cell in the coarse motion-detection grid.
type gridPoint struct{ row, col int }

// detectParticipants runs a coarse grid-based, frame-to-frame pixel-diff
// motion detection across frames, groups moving grid cells into
// 4-connected regions (a lightweight stand-in for real participant
// detection, per spec Non-scope: no ML/pose-estimation dependency), and
// computes each resulting region's activity score as the sum of its cells'
// accumulated frame-to-frame luma differences.
func detectParticipants(frames []*image.Gray) []ParticipantResult {
	bounds := frames[0].Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return []ParticipantResult{}
	}
	cols := (width + gridCell - 1) / gridCell
	rows := (height + gridCell - 1) / gridCell

	cellActivity := make([][]float64, rows)
	for i := range cellActivity {
		cellActivity[i] = make([]float64, cols)
	}

	pairs := 0
	for i := 1; i < len(frames); i++ {
		if frames[i].Bounds() != bounds {
			// A frame with mismatched dimensions can't be diffed cell-for-cell;
			// skip it rather than failing the whole analysis.
			continue
		}
		accumulateCellDiffs(frames[i-1], frames[i], cellActivity, rows, cols)
		pairs++
	}
	if pairs == 0 {
		return []ParticipantResult{}
	}

	threshold := motionThresholdPerPair * float64(pairs)
	moving := make([][]bool, rows)
	for r := 0; r < rows; r++ {
		moving[r] = make([]bool, cols)
		for c := 0; c < cols; c++ {
			moving[r][c] = cellActivity[r][c] >= threshold
		}
	}

	regions := connectedComponents(moving, rows, cols)

	participants := make([]ParticipantResult, 0, len(regions))
	for _, region := range regions {
		if len(region) < minRegionCells {
			continue
		}
		var score float64
		for _, p := range region {
			score += cellActivity[p.row][p.col]
		}
		participants = append(participants, ParticipantResult{ActivityScore: score})
	}
	for i := range participants {
		participants[i].Label = fmt.Sprintf("Participant %d", i+1)
	}
	return participants
}

// accumulateCellDiffs adds the average absolute luma difference between prev
// and next, per grid cell, into cellActivity.
func accumulateCellDiffs(prev, next *image.Gray, cellActivity [][]float64, rows, cols int) {
	bounds := prev.Bounds()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0 := bounds.Min.X + c*gridCell
			y0 := bounds.Min.Y + r*gridCell
			x1 := min(x0+gridCell, bounds.Max.X)
			y1 := min(y0+gridCell, bounds.Max.Y)

			var sum float64
			var count int
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					pv := prev.GrayAt(x, y).Y
					nv := next.GrayAt(x, y).Y
					diff := int(pv) - int(nv)
					if diff < 0 {
						diff = -diff
					}
					sum += float64(diff)
					count++
				}
			}
			if count > 0 {
				cellActivity[r][c] += sum / float64(count)
			}
		}
	}
}

// connectedComponents groups adjacent (4-connected) true cells in moving
// into distinct regions, returning one []gridPoint per region.
func connectedComponents(moving [][]bool, rows, cols int) [][]gridPoint {
	visited := make([][]bool, rows)
	for i := range visited {
		visited[i] = make([]bool, cols)
	}

	var regions [][]gridPoint
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if !moving[r][c] || visited[r][c] {
				continue
			}
			regions = append(regions, floodFill(moving, visited, rows, cols, r, c))
		}
	}
	return regions
}

// floodFill collects the 4-connected region of moving cells starting at
// (startRow, startCol), marking each visited.
func floodFill(moving, visited [][]bool, rows, cols, startRow, startCol int) []gridPoint {
	var region []gridPoint
	queue := []gridPoint{{startRow, startCol}}
	visited[startRow][startCol] = true

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		region = append(region, p)

		for _, d := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
			nr, nc := p.row+d[0], p.col+d[1]
			if nr < 0 || nr >= rows || nc < 0 || nc >= cols {
				continue
			}
			if !moving[nr][nc] || visited[nr][nc] {
				continue
			}
			visited[nr][nc] = true
			queue = append(queue, gridPoint{nr, nc})
		}
	}
	return region
}
