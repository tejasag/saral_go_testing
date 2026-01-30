package reel

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ReelVideoGenerator handles video composition for reels
type ReelVideoGenerator struct {
	OutputDir string
	AssetsDir string
}

// NewReelVideoGenerator creates a new video generator
func NewReelVideoGenerator(outputDir, assetsDir string) *ReelVideoGenerator {
	os.MkdirAll(outputDir, 0755)
	return &ReelVideoGenerator{
		OutputDir: outputDir,
		AssetsDir: assetsDir,
	}
}

// GenerateTitleBackground creates a white background video with title and author
func (v *ReelVideoGenerator) GenerateTitleBackground(metadata *PaperMetadata, duration int) (string, error) {
	// Create title image
	imgPath := filepath.Join(v.OutputDir, "title_bg.png")
	videoPath := filepath.Join(v.OutputDir, "title_bg.mp4")

	if err := createTitleImage(metadata, imgPath, 480, 850); err != nil {
		return "", fmt.Errorf("failed to create title image: %w", err)
	}

	// Convert image to video using ffmpeg
	cmd := exec.Command("ffmpeg",
		"-y",
		"-loop", "1",
		"-i", imgPath,
		"-c:v", "libx264",
		"-t", strconv.Itoa(duration),
		"-pix_fmt", "yuv420p",
		"-vf", "scale=480:850",
		"-preset", "medium",
		"-r", "24",
		videoPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %s, output: %s", err, string(output))
	}

	// Clean up PNG
	os.Remove(imgPath)

	return videoPath, nil
}

// createTitleImage creates a white background image with title and author text
func createTitleImage(metadata *PaperMetadata, outputPath string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Fill with white
	white := color.RGBA{255, 255, 255, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{white}, image.Point{}, draw.Src)

	// For now, we create a simple white image
	// Text rendering requires additional dependencies (golang.org/x/image/font)
	// The title will be shown in the video overlay instead

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

// OverlayAvatarOnBackground overlays an avatar on the background video
func (v *ReelVideoGenerator) OverlayAvatarOnBackground(bgPath, avatarPath, position, outputPath string) error {
	// Determine overlay position
	var overlayFilter string
	switch position {
	case "bottom-left":
		overlayFilter = "[0:v][1:v] overlay=0:H-h:enable='between(t,0,60)'"
	case "bottom-right":
		overlayFilter = "[0:v][1:v] overlay=W-w:H-h:enable='between(t,0,60)'"
	default:
		overlayFilter = "[0:v][1:v] overlay=0:H-h:enable='between(t,0,60)'"
	}

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", bgPath,
		"-i", avatarPath,
		"-filter_complex", overlayFilter,
		"-pix_fmt", "yuv420p",
		"-c:a", "copy",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg overlay error: %s, output: %s", err, string(output))
	}

	return nil
}

// CreateAvatarVideos creates two videos with each avatar overlaid on background
func (v *ReelVideoGenerator) CreateAvatarVideos(bgPath string, avatarPair *AvatarPair) (person1Video, person2Video string, err error) {
	femaleAvatarPath := filepath.Join(v.AssetsDir, avatarPair.FemaleAvatar)
	maleAvatarPath := filepath.Join(v.AssetsDir, avatarPair.MaleAvatar)

	person1Video = filepath.Join(v.OutputDir, "Person1_video.mp4")
	person2Video = filepath.Join(v.OutputDir, "Person2_video.mp4")

	// Check avatar files exist
	if _, err := os.Stat(femaleAvatarPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("female avatar not found: %s", femaleAvatarPath)
	}
	if _, err := os.Stat(maleAvatarPath); os.IsNotExist(err) {
		return "", "", fmt.Errorf("male avatar not found: %s", maleAvatarPath)
	}

	// Create Person1 (female) video - bottom left
	log.Println("[VIDEO] Creating Person1 (female) avatar video...")
	if err := v.OverlayAvatarOnBackground(bgPath, femaleAvatarPath, "bottom-left", person1Video); err != nil {
		return "", "", fmt.Errorf("failed to create Person1 video: %w", err)
	}

	// Create Person2 (male) video - bottom right
	log.Println("[VIDEO] Creating Person2 (male) avatar video...")
	if err := v.OverlayAvatarOnBackground(bgPath, maleAvatarPath, "bottom-right", person2Video); err != nil {
		return "", "", fmt.Errorf("failed to create Person2 video: %w", err)
	}

	return person1Video, person2Video, nil
}

// CompositeReelVideo creates the final reel by combining avatar videos with audio
func (v *ReelVideoGenerator) CompositeReelVideo(
	person1Video, person2Video string,
	audioFiles map[int]string,
	dialogueTurns []DialogueTurn,
) (string, error) {

	if len(audioFiles) == 0 {
		return "", fmt.Errorf("no audio files provided")
	}

	// Create video clips for each dialogue turn
	var clipPaths []string

	for i, turn := range dialogueTurns {
		audioPath, ok := audioFiles[i]
		if !ok {
			log.Printf("[VIDEO] No audio for turn %d, skipping", i)
			continue
		}

		// Determine which avatar video to use
		var avatarVideo string
		if turn.Character == "Person1" {
			avatarVideo = person1Video
		} else {
			avatarVideo = person2Video
		}

		// Get audio duration
		duration, err := getAudioDuration(audioPath)
		if err != nil {
			log.Printf("[VIDEO] Error getting audio duration for turn %d: %v", i, err)
			continue
		}

		// Create clip with audio
		clipPath := filepath.Join(v.OutputDir, fmt.Sprintf("clip_%02d.mp4", i))
		if err := v.createClipWithAudio(avatarVideo, audioPath, duration, clipPath); err != nil {
			log.Printf("[VIDEO] Error creating clip for turn %d: %v", i, err)
			continue
		}

		clipPaths = append(clipPaths, clipPath)
		log.Printf("[VIDEO] ✓ Created clip %d: %s (%.2fs)", i, filepath.Base(clipPath), duration)
	}

	if len(clipPaths) == 0 {
		return "", fmt.Errorf("no video clips created")
	}

	// Concatenate all clips
	finalPath := filepath.Join(v.OutputDir, "reel_output.mp4")
	if err := v.concatenateClips(clipPaths, finalPath); err != nil {
		return "", fmt.Errorf("failed to concatenate clips: %w", err)
	}

	log.Printf("[VIDEO] ✓ Created final reel: %s", finalPath)
	return finalPath, nil
}

// createClipWithAudio creates a video clip from avatar video with synced audio
func (v *ReelVideoGenerator) createClipWithAudio(videoPath, audioPath string, duration float64, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-y",
		"-ss", "0",
		"-t", fmt.Sprintf("%.2f", duration),
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-pix_fmt", "yuv420p",
		"-preset", "ultrafast",
		"-threads", "8",
		"-shortest",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %s, output: %s", err, string(output))
	}

	return nil
}

// concatenateClips concatenates video clips into a final video
func (v *ReelVideoGenerator) concatenateClips(clipPaths []string, outputPath string) error {
	// Create concat list file
	listContent := ""
	for _, path := range clipPaths {
		absPath, _ := filepath.Abs(path)
		listContent += fmt.Sprintf("file '%s'\n", absPath)
	}

	listPath := filepath.Join(v.OutputDir, "concat_list.txt")
	if err := os.WriteFile(listPath, []byte(listContent), 0644); err != nil {
		return err
	}

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-c", "copy",
		outputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat error: %s, output: %s", err, string(output))
	}

	return nil
}

// getAudioDuration gets the duration of an audio file using ffprobe
func getAudioDuration(path string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffprobe error: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	return strconv.ParseFloat(durationStr, 64)
}

// CleanupTempFiles removes temporary files
func (v *ReelVideoGenerator) CleanupTempFiles() {
	// Remove clip files
	files, _ := filepath.Glob(filepath.Join(v.OutputDir, "clip_*.mp4"))
	for _, f := range files {
		os.Remove(f)
	}

	// Remove avatar videos
	os.Remove(filepath.Join(v.OutputDir, "Person1_video.mp4"))
	os.Remove(filepath.Join(v.OutputDir, "Person2_video.mp4"))

	// Remove title background
	os.Remove(filepath.Join(v.OutputDir, "title_bg.mp4"))

	// Remove concat list
	os.Remove(filepath.Join(v.OutputDir, "concat_list.txt"))
}
