package video

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// FFProbeProber is the real Prober implementation: it shells out to ffprobe
// and parses duration/width/height/fps from its JSON output.
type FFProbeProber struct{}

// ffprobeOutput mirrors the subset of ffprobe's `-print_format json
// -show_format -show_streams` output this package needs.
type ffprobeOutput struct {
	Format struct {
		Duration string `json:"duration"`
	} `json:"format"`
	Streams []struct {
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
}

// Probe runs ffprobe against videoPath and extracts duration, resolution,
// and frame rate from the first video stream.
func (FFProbeProber) Probe(ctx context.Context, videoPath string) (Metadata, error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		videoPath,
	)
	out, err := cmd.Output()
	if err != nil {
		return Metadata{}, fmt.Errorf("video: ffprobe: %w", err)
	}

	var parsed ffprobeOutput
	if err := json.Unmarshal(out, &parsed); err != nil {
		return Metadata{}, fmt.Errorf("video: ffprobe: parse json: %w", err)
	}

	duration, err := strconv.ParseFloat(parsed.Format.Duration, 64)
	if err != nil {
		return Metadata{}, fmt.Errorf("video: ffprobe: parse duration %q: %w", parsed.Format.Duration, err)
	}

	var width, height int
	var fps float64
	for _, s := range parsed.Streams {
		if s.CodecType != "video" {
			continue
		}
		width = s.Width
		height = s.Height
		fps = parseFrameRate(s.RFrameRate)
		break
	}

	return Metadata{Duration: duration, Width: width, Height: height, FPS: fps}, nil
}

// parseFrameRate parses ffprobe's r_frame_rate, which is either a plain
// number or a "num/den" fraction (e.g. "30000/1001").
func parseFrameRate(s string) float64 {
	num, den, found := strings.Cut(s, "/")
	if !found {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}

	numVal, err1 := strconv.ParseFloat(num, 64)
	denVal, err2 := strconv.ParseFloat(den, 64)
	if err1 != nil || err2 != nil || denVal == 0 {
		return 0
	}
	return numVal / denVal
}
