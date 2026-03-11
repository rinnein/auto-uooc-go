package video

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/vansante/go-ffprobe.v2"
)

const ffprobePathEnv = "FFPROBE_PATH"

func ConfigureFFProbePath(configPath string) {
	resolved := strings.TrimSpace(configPath)
	if resolved == "" {
		resolved = strings.TrimSpace(os.Getenv(ffprobePathEnv))
	}
	if resolved == "" {
		resolved = findFFProbeInExecutableDir()
	}
	if resolved != "" {
		ffprobe.SetFFProbeBinPath(resolved)
	}
}

func findFFProbeInExecutableDir() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	exeDir := filepath.Dir(exePath)
	entries, err := os.ReadDir(exeDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.Contains(strings.ToLower(entry.Name()), "ffprobe") {
			continue
		}
		fullPath := filepath.Join(exeDir, entry.Name())
		if isExecutableFile(fullPath) {
			return fullPath
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		ext := strings.ToLower(filepath.Ext(path))
		return ext == ".exe" || ext == ".cmd" || ext == ".bat"
	}
	return info.Mode()&0o111 != 0
}

func FetchDurationSeconds(ctx context.Context, sourceURL string) (float64, error) {
	data, err := ffprobe.ProbeURL(ctx, sourceURL)
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	if data == nil || data.Format == nil {
		return 0, fmt.Errorf("no format info available")
	}

	duration := data.Format.Duration().Seconds()
	if duration <= 0 {
		return 0, fmt.Errorf("invalid duration: %.2f", duration)
	}
	return duration, nil
}
