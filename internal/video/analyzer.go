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

	// GridCellPx is the side length (in pixels) of each cell in the coarse
	// grid used for motion-region detection. Zero/unset defaults to today's
	// hardcoded 32px (spec criterion 6), configurable via MOTION_GRID_CELL_PX
	// (spec criterion 5) so an operator can retune sensitivity without a
	// rebuild.
	GridCellPx int

	// MotionThresholdPerPair is the minimum average per-pixel luma
	// difference (0-255 scale) a grid cell must exceed, per single
	// frame-to-frame pair, to be considered active in that pair. Zero/unset
	// defaults to today's hardcoded 15.0, configurable via
	// MOTION_THRESHOLD_PER_PAIR.
	MotionThresholdPerPair float64

	// MinRegionCells is the minimum number of connected moving cells for a
	// region to count as a distinct participant, filtering out single-cell
	// noise/flicker. Zero/unset defaults to today's hardcoded 2, configurable
	// via MOTION_MIN_REGION_CELLS.
	MinRegionCells int

	// MaxParticipants caps how many top-scoring (by persistence, spec
	// criterion 7) detected regions are kept as participants. Zero/unset
	// defaults to 2 (spec criterion 10 — an intentional behavior change from
	// today's "keep every detected region"), configurable via
	// MAX_PARTICIPANTS for non-standard footage.
	MaxParticipants int
}

const (
	// gridCell is the side length (in pixels) of each cell in the coarse grid
	// used for motion-region detection, when FFMPEGAnalyzer.GridCellPx is
	// zero/unset.
	gridCell = 32
	// motionThresholdPerPair is the minimum average per-pixel luma
	// difference (0-255 scale) a grid cell must exceed, per single
	// frame-to-frame pair, to be considered active in that pair, when
	// FFMPEGAnalyzer.MotionThresholdPerPair is zero/unset.
	motionThresholdPerPair = 15.0
	// minRegionCells is the minimum number of connected moving cells for a
	// region to count as a distinct participant, when
	// FFMPEGAnalyzer.MinRegionCells is zero/unset.
	minRegionCells = 2
	// defaultMaxParticipants is the number of top-persistence regions kept
	// when FFMPEGAnalyzer.MaxParticipants is zero/unset (spec criterion 10:
	// "2 fighters" is the default, not "keep every detected region").
	defaultMaxParticipants = 2
)

// Analyze extracts frames from videoPath into tmpDir via ffmpeg, decodes
// them, and detects participants/activity scores via frame-to-frame
// pixel-diff motion detection, using a's configured (or defaulted)
// thresholds.
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

	return decodeAndDetect(framesDir, a)
}

// analyzeFrames decodes every frame file in framesDir and runs
// participant/motion detection over them using the default (zero-value)
// FFMPEGAnalyzer configuration. Extracted from Analyze so the decode/detect
// decision logic (in particular, the too-few-frames vs. zero-participants
// distinction) is directly unit-testable without shelling out to a real
// ffmpeg subprocess or threading a config through every test.
func analyzeFrames(framesDir string) (AnalysisResult, error) {
	return decodeAndDetect(framesDir, FFMPEGAnalyzer{})
}

// decodeAndDetect is the shared decode+detect seam behind both analyzeFrames
// (always using default thresholds) and FFMPEGAnalyzer.Analyze (using its
// own configured thresholds), so the too-few-frames vs. zero-participants
// distinction is defined exactly once.
func decodeAndDetect(framesDir string, cfg FFMPEGAnalyzer) (AnalysisResult, error) {
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
	return AnalysisResult{Participants: detectParticipants(frames, cfg)}, nil
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

// scoredRegion is one connected moving region's computed scores, before
// top-N selection.
type scoredRegion struct {
	// score is the region's total accumulated activity (sum, across every
	// cell and every frame pair, of that cell's average luma diff) — today's
	// original ranking signal, kept as acceptance criterion 7/8's tie-break.
	score float64
	// persistence is the fraction of frame-pairs in which at least one of
	// the region's cells was active in that specific pair (spec criterion
	// 7/edge case 1): fighters move near-continuously, so this favors
	// sustained motion over a single high-magnitude spike (e.g. a referee's
	// point call).
	persistence float64
	cells       []gridPoint
}

// detectParticipants runs a coarse grid-based, frame-to-frame pixel-diff
// motion detection across frames (grid size and per-pair threshold from
// cfg, defaulting to today's 32px/15.0 when zero), groups moving grid cells
// into 4-connected regions (a lightweight stand-in for real participant
// detection, per spec Non-scope: no ML/pose-estimation dependency), scores
// each region by both its accumulated activity and its persistence (the
// fraction of frame-pairs it was active in), and keeps only the top
// cfg.MaxParticipants regions ranked by persistence descending, ties broken
// by accumulated activity descending (spec criterion 7, edge case 8).
func detectParticipants(frames []*image.Gray, cfg FFMPEGAnalyzer) []ParticipantResult {
	gridCellPx := cfg.GridCellPx
	if gridCellPx <= 0 {
		gridCellPx = gridCell
	}
	thresholdPerPair := cfg.MotionThresholdPerPair
	if thresholdPerPair <= 0 {
		thresholdPerPair = motionThresholdPerPair
	}
	minCells := cfg.MinRegionCells
	if minCells <= 0 {
		minCells = minRegionCells
	}
	maxParticipants := cfg.MaxParticipants
	if maxParticipants <= 0 {
		maxParticipants = defaultMaxParticipants
	}

	bounds := frames[0].Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width == 0 || height == 0 {
		return []ParticipantResult{}
	}
	cols := (width + gridCellPx - 1) / gridCellPx
	rows := (height + gridCellPx - 1) / gridCellPx

	cellActivity := make([][]float64, rows)
	// cellPairActive[r][c] holds one bool per frame pair processed so far,
	// recording whether that specific cell was active (diff >=
	// thresholdPerPair) in that specific pair — the per-pair bookkeeping
	// persistence scoring needs, distinct from cellActivity's running total
	// (plan.md Risks #6).
	cellPairActive := make([][][]bool, rows)
	for i := range cellActivity {
		cellActivity[i] = make([]float64, cols)
		cellPairActive[i] = make([][]bool, cols)
	}

	pairs := 0
	for i := 1; i < len(frames); i++ {
		if frames[i].Bounds() != bounds {
			// A frame with mismatched dimensions can't be diffed cell-for-cell;
			// skip it rather than failing the whole analysis.
			continue
		}
		accumulateCellDiffs(frames[i-1], frames[i], cellActivity, cellPairActive, rows, cols, gridCellPx, thresholdPerPair)
		pairs++
	}
	if pairs == 0 {
		return []ParticipantResult{}
	}

	threshold := thresholdPerPair * float64(pairs)
	moving := make([][]bool, rows)
	for r := 0; r < rows; r++ {
		moving[r] = make([]bool, cols)
		for c := 0; c < cols; c++ {
			moving[r][c] = cellActivity[r][c] >= threshold
		}
	}

	regions := connectedComponents(moving, rows, cols)

	scored := make([]scoredRegion, 0, len(regions))
	for _, region := range regions {
		if len(region) < minCells {
			continue
		}
		var score float64
		activePairs := make([]bool, pairs)
		for _, p := range region {
			score += cellActivity[p.row][p.col]
			for pairIdx, active := range cellPairActive[p.row][p.col] {
				if active {
					activePairs[pairIdx] = true
				}
			}
		}
		activeCount := 0
		for _, active := range activePairs {
			if active {
				activeCount++
			}
		}
		scored = append(scored, scoredRegion{
			score:       score,
			persistence: float64(activeCount) / float64(pairs),
			cells:       region,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].persistence != scored[j].persistence {
			return scored[i].persistence > scored[j].persistence
		}
		// Tie-break by accumulated activity score, higher wins (spec edge
		// case 8: keeps region selection deterministic given identical
		// input frames).
		return scored[i].score > scored[j].score
	})

	if len(scored) > maxParticipants {
		scored = scored[:maxParticipants]
	}

	participants := make([]ParticipantResult, 0, len(scored))
	for _, s := range scored {
		participants = append(participants, ParticipantResult{ActivityScore: s.score})
	}
	for i := range participants {
		participants[i].Label = fmt.Sprintf("Participant %d", i+1)
	}
	return participants
}

// accumulateCellDiffs adds the average absolute luma difference between prev
// and next, per grid cell, into cellActivity, and appends one bool per cell
// to cellPairActive recording whether that cell's diff for this specific
// pair met thresholdPerPair — the per-pair signal detectParticipants uses to
// compute each region's persistence score.
func accumulateCellDiffs(prev, next *image.Gray, cellActivity [][]float64, cellPairActive [][][]bool, rows, cols, gridCellPx int, thresholdPerPair float64) {
	bounds := prev.Bounds()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x0 := bounds.Min.X + c*gridCellPx
			y0 := bounds.Min.Y + r*gridCellPx
			x1 := min(x0+gridCellPx, bounds.Max.X)
			y1 := min(y0+gridCellPx, bounds.Max.Y)

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

			var avg float64
			if count > 0 {
				avg = sum / float64(count)
				cellActivity[r][c] += avg
			}
			cellPairActive[r][c] = append(cellPairActive[r][c], avg >= thresholdPerPair)
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
