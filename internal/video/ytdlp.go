package video

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// YTDLPDownloader is the real Downloader implementation: it shells out to
// yt-dlp with the URL passed as a discrete argv element, never via shell
// string interpolation/concatenation (spec Constraints: command-injection
// defense in depth).
type YTDLPDownloader struct{}

// Download runs `yt-dlp <youtubeURL> -o <destDir>/video.mp4` via
// exec.CommandContext, returning the path to the downloaded file.
func (YTDLPDownloader) Download(ctx context.Context, youtubeURL, destDir string) (string, error) {
	destPath := filepath.Join(destDir, "video.mp4")

	cmd := exec.CommandContext(ctx, "yt-dlp", youtubeURL, "-o", destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("video: yt-dlp download: %w: %s", err, output)
	}

	return destPath, nil
}
