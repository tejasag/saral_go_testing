package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type VideoGenerator struct {
	OutputDir string
}

func NewVideoGenerator(outputDir string) *VideoGenerator {
	return &VideoGenerator{OutputDir: outputDir}
}

// CreateSegment creates a video file from a list of images and one audio file.
func (v *VideoGenerator) CreateSegment(images []string, audioPath, outputName string) (string, error) {
	if len(images) == 0 {
		return "", fmt.Errorf("no images for segment")
	}

	outputPath := filepath.Join(v.OutputDir, outputName)

	// 1. Get Audio Duration
	duration, err := getAudioDuration(audioPath)
	if err != nil {
		return "", err
	}

	// 2. Calculate duration per image
	perImageDuration := duration / float64(len(images))

	// 3. Create a demuxer file for ffmpeg
	demuxerContent := ""
	for _, img := range images {
		absImg, _ := filepath.Abs(img)
		demuxerContent += fmt.Sprintf("file '%s'\n", absImg)
		demuxerContent += fmt.Sprintf("duration %.2f\n", perImageDuration)
	}

	lastImg, _ := filepath.Abs(images[len(images)-1])
	demuxerContent += fmt.Sprintf("file '%s'\n", lastImg)

	demuxerPath := filepath.Join(v.OutputDir, outputName+"_demux.txt")
	os.WriteFile(demuxerPath, []byte(demuxerContent), 0644)

	// 4. FFmpeg command
	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "concat", "-safe", "0", "-i", demuxerPath,
		"-i", audioPath,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-vf", "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2",
		"-c:a", "aac",
		"-shortest",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg video creation failed: %s, output: %s", err, string(output))
	}

	return outputPath, nil
}

func (v *VideoGenerator) ConcatSegments(segments []string, finalOutputName string) (string, error) {
	if len(segments) == 0 {
		return "", fmt.Errorf("no segments to concat")
	}

	outputPath := filepath.Join(v.OutputDir, finalOutputName)

	listContent := ""
	for _, seg := range segments {
		absPath, _ := filepath.Abs(seg)
		listContent += fmt.Sprintf("file '%s'\n", absPath)
	}

	listPath := filepath.Join(v.OutputDir, "concat_list.txt")
	os.WriteFile(listPath, []byte(listContent), 0644)

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "concat", "-safe", "0", "-i", listPath,
		"-c", "copy",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg concat failed: %s, output: %s", err, string(output))
	}

	return outputPath, nil
}

func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	durationStr := strings.TrimSpace(string(output))
	return strconv.ParseFloat(durationStr, 64)
}
