package reel

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"saral_go_testing/common"
)

// ProcessReelPipeline executes the full PDF to Reel workflow
// This follows the same pattern as video.ProcessVideoPipeline and poster.ProcessPosterPipeline
func ProcessReelPipeline(config common.PipelineConfig) error {
	// Ensure OutputDir exists
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}
	log.Printf("[REEL] Starting reel pipeline for %s -> %s", config.PDFPath, config.OutputDir)

	// 1. Process PDF (Extract Text)
	log.Println("[REEL] Step 1: Processing PDF...")
	pdfProc, err := common.NewPDFProcessor(config.PDFPath, config.OutputDir)
	if err != nil {
		return fmt.Errorf("failed to open PDF: %w", err)
	}
	defer pdfProc.Close()

	text, err := pdfProc.ExtractText()
	if err != nil {
		return fmt.Errorf("text extraction failed: %w", err)
	}
	log.Printf("[REEL] Extracted %d chars of text", len(text))

	if text == "" {
		return fmt.Errorf("no text extracted")
	}

	// 2. Generate Dialogue Script using common GeminiClient
	log.Println("[REEL] Step 2: Generating Dialogue Script with Gemini...")
	gemini, err := common.NewGeminiClient(config.GeminiKey)
	if err != nil {
		return fmt.Errorf("gemini init failed: %w", err)
	}
	defer gemini.Close()

	// Extract paper metadata (title and authors)
	log.Println("[REEL] Extracting paper metadata...")
	paperMetadata, err := gemini.ExtractMetadata(text)
	if err != nil {
		log.Printf("[REEL] Warning: metadata extraction failed: %v, using defaults", err)
	}
	log.Printf("[REEL] Paper Title: %s", paperMetadata.Title)
	log.Printf("[REEL] Paper Authors: %s", paperMetadata.Authors)

	dialogue, err := GenerateReelDialogue(gemini, text)
	if err != nil {
		return fmt.Errorf("dialogue generation failed: %w", err)
	}
	os.WriteFile(filepath.Join(config.OutputDir, "dialogue.txt"), []byte(dialogue), 0644)

	// Parse dialogue into turns
	dialogueTurns := ParseDialogueToScript(dialogue)
	if len(dialogueTurns) == 0 {
		return fmt.Errorf("failed to parse dialogue into script")
	}
	log.Printf("[REEL] Parsed %d dialogue turns", len(dialogueTurns))

	// 3. Generate Audio (Parallel) using existing TTS pattern
	log.Println("[REEL] Step 3: Generating Audio (Parallel)...")
	audioDir := filepath.Join(config.OutputDir, "audio")
	ttsClient := NewReelTTSClient(config.SarvamKey)

	audioFiles, err := ttsClient.GenerateDialogueAudio(dialogueTurns, audioDir, "english")
	if err != nil {
		return fmt.Errorf("audio generation failed: %w", err)
	}
	log.Printf("[REEL] Generated %d audio files", len(audioFiles))

	// 4. Generate Video (Title background + Avatar overlays)
	log.Println("[REEL] Step 4: Creating Video...")
	assetsDir := "./assets"
	videoDir := filepath.Join(config.OutputDir, "video")
	videoGen := NewReelVideoGenerator(videoDir, assetsDir)

	// Use extracted metadata for video title
	metadata := &PaperMetadata{
		Title:   paperMetadata.Title,
		Authors: paperMetadata.Authors,
	}

	// Generate title background
	bgPath, err := videoGen.GenerateTitleBackground(metadata, 120)
	if err != nil {
		// Try default background
		defaultBg := filepath.Join(assetsDir, "bg3.mp4")
		if _, statErr := os.Stat(defaultBg); statErr == nil {
			bgPath = defaultBg
			log.Printf("[REEL] Using default background: %s", defaultBg)
		} else {
			return fmt.Errorf("title background creation failed: %w", err)
		}
	}

	// Use default avatar pair
	avatarPair := &AvailableAvatarPairs[0]

	// Create avatar overlay videos
	person1Video, person2Video, err := videoGen.CreateAvatarVideos(bgPath, avatarPair)
	if err != nil {
		return fmt.Errorf("avatar video creation failed: %w", err)
	}

	// Composite final video
	finalPath, err := videoGen.CompositeReelVideo(person1Video, person2Video, audioFiles, dialogueTurns)
	if err != nil {
		return fmt.Errorf("video composition failed: %w", err)
	}

	log.Printf("[REEL] Reel Pipeline Complete! Video: %s", finalPath)
	return nil
}

// GenerateReelDialogue generates short-form dialogue using common GeminiClient
func GenerateReelDialogue(gemini *common.GeminiClient, text string) (string, error) {
	// Limit text to prevent token overflow
	if len(text) > 6000 {
		text = text[:6000]
	}

	prompt := fmt.Sprintf(`You are a skilled content creator specializing in short-form educational content for social media reels.

Your task is to generate a quick, engaging, and punchy dialogue between two speakers — 
Person1 and Person2 — as they discuss the key highlights of a research paper in a reel format.

Dialogue Requirements:
- Generate a SHORT dialogue with exactly 6-8 exchanges between speakers (perfect for 30-60 second reels)
- Each dialogue line should be 15-25 words maximum (for quick delivery)
- Use alternating lines with clear speaker tags (Person1:, Person2:)
- Make it conversational, energetic, and hook-focused
- Start with an attention-grabbing hook
- Focus on the most interesting/surprising finding from the paper
- End with a strong takeaway or call-to-action
- Use simple, accessible language - no jargon
- Make each line punchy and quotable

Output Example:
Person1: Did you know scientists just figured out how to make batteries charge in 10 seconds?
Person2: Wait, what? That's impossible!
Person1: Not anymore! They used a new material that changes everything.
Person2: So my phone could charge fully in seconds?
Person1: Exactly! And it could last 10 times longer too.
Person2: This is going to revolutionize everything we use!

Here is the research paper content:

%s

Generate a short, engaging reel dialogue between Person1 and Person2 about the most interesting aspect of this paper.
`, text)

	return gemini.GenerateText(prompt)
}

// ParseDialogueToScript converts raw dialogue text to structured DialogueTurns
func ParseDialogueToScript(dialogue string) []DialogueTurn {
	var script []DialogueTurn

	lines := strings.Split(dialogue, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			speaker := strings.TrimSpace(parts[0])

			var character string
			switch {
			case strings.Contains(strings.ToLower(speaker), "person1"):
				character = "Person1"
			case strings.Contains(strings.ToLower(speaker), "person2"):
				character = "Person2"
			default:
				continue
			}

			if len(parts) > 1 {
				dialogueText := strings.TrimSpace(parts[1])
				if dialogueText != "" {
					script = append(script, DialogueTurn{
						Character: character,
						Dialogue:  dialogueText,
					})
				}
			}
		}
	}

	return script
}

// DialogueAudioResult holds the result of generating audio for one dialogue turn
type DialogueAudioResult struct {
	Index     int
	Character string
	AudioPath string
	Error     error
}

// GenerateDialogueAudio generates audio for all dialogue turns concurrently
func (c *ReelTTSClient) GenerateDialogueAudio(dialogue []DialogueTurn, outputDir, language string) (map[int]string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	languageCode := GetLanguageCode(language)

	results := make(chan DialogueAudioResult, len(dialogue))
	var wg sync.WaitGroup

	for i, turn := range dialogue {
		if turn.Dialogue == "" {
			continue
		}

		wg.Add(1)
		go func(index int, t DialogueTurn) {
			defer wg.Done()

			// Determine voice based on character
			voice := "vidya" // Person1 = female
			if t.Character == "Person2" {
				voice = "karun" // Person2 = male
			}

			filename := fmt.Sprintf("%02d_%s.wav", index, t.Character)
			outputPath := filepath.Join(outputDir, filename)

			log.Printf("[TTS] Generating audio for turn %d: character=%s, voice=%s", index, t.Character, voice)

			err := c.synthesizeText(t.Dialogue, outputPath, languageCode, voice)
			if err != nil {
				log.Printf("[TTS] Error generating audio for turn %d: %v", index, err)
			}

			results <- DialogueAudioResult{
				Index:     index,
				Character: t.Character,
				AudioPath: outputPath,
				Error:     err,
			}
		}(i, turn)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	audioMap := make(map[int]string)
	var errors []string

	for res := range results {
		if res.Error != nil {
			errors = append(errors, fmt.Sprintf("turn %d: %v", res.Index, res.Error))
		} else {
			audioMap[res.Index] = res.AudioPath
			log.Printf("[TTS] ✓ Generated audio: %s", filepath.Base(res.AudioPath))
		}
	}

	if len(audioMap) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("all audio generation failed: %s", strings.Join(errors, "; "))
	}

	log.Printf("[TTS] Generated %d audio files", len(audioMap))
	return audioMap, nil
}
