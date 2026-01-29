package video

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"saral_go_testing/common"
)

// ProcessVideoPipeline executes the full PDF to Video workflow
func ProcessVideoPipeline(config common.PipelineConfig) error {
	// Ensure OutputDir exists
	if err := os.MkdirAll(config.OutputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}
	log.Printf("Starting video pipeline for %s -> %s", config.PDFPath, config.OutputDir)

	// 1. Processing PDF (Text & Images)
	log.Println("Step 1: Processing PDF...")
	pdfProc, err := common.NewPDFProcessor(config.PDFPath, config.OutputDir)
	if err != nil {
		return fmt.Errorf("failed to open PDF: %w", err)
	}
	defer pdfProc.Close()

	text, err := pdfProc.ExtractText()
	if err != nil {
		return fmt.Errorf("text extraction failed: %w", err)
	}
	log.Printf("Extracted %d chars of text", len(text))

	if text == "" {
		return fmt.Errorf("no text extracted")
	}

	// 2. Gemini: Script Generation
	log.Println("Step 2: Generating Script with Gemini...")
	gemini, err := common.NewGeminiClient(config.GeminiKey)
	if err != nil {
		return fmt.Errorf("gemini init failed: %w", err)
	}
	defer gemini.Close()

	fullScript, err := gemini.GenerateScript(text)
	if err != nil {
		return fmt.Errorf("script generation failed: %w", err)
	}
	os.WriteFile(filepath.Join(config.OutputDir, "script.txt"), []byte(fullScript), 0644)

	// Parse Script into Sections
	sections := common.ParseScriptToSections(fullScript)

	// 3. Generate Bullet Points (Parallelized)
	log.Println("Step 3: Generating Bullet Points (Parallel)...")
	var bulletWg sync.WaitGroup
	var sectionMutex sync.Mutex

	for name, data := range sections {
		bulletWg.Add(1)
		go func(n string, d common.SectionData) {
			defer bulletWg.Done()

			bullets, err := gemini.GenerateBulletPoints(d.Script)
			if err != nil {
				log.Printf("Bullet gen failed for %s: %v", n, err)
				bullets = []string{"Key points unavailable"}
			}

			sectionMutex.Lock()
			sections[n] = common.SectionData{
				Title:   n,
				Script:  d.Script,
				Bullets: bullets,
				Image:   "",
			}
			sectionMutex.Unlock()
		}(name, data)
	}
	bulletWg.Wait()

	// 4. Parallel Asset Generation (Slides & Audio)
	log.Println("Step 4: Generating Assets (Slides & Audio)...")

	slideGen := NewSlideGenerator(filepath.Join(config.OutputDir, "slides"))
	sarvam := NewSarvamClient(config.SarvamKey)
	videoGen := NewVideoGenerator(filepath.Join(config.OutputDir, "video"))
	os.MkdirAll(videoGen.OutputDir, 0755)

	type AssetResult struct {
		Name      string
		AudioPath string
		Err       error
	}

	var titleSlide string
	var sectionSlides map[string][]string

	var assetWg sync.WaitGroup

	// A. Slides
	assetWg.Add(1)
	go func() {
		defer assetWg.Done()
		var err error
		docTitle := strings.TrimSuffix(filepath.Base(config.PDFPath), filepath.Ext(config.PDFPath))
		titleSlide, sectionSlides, _, err = slideGen.GenerateSlides(docTitle, docTitle, sections)
		if err != nil {
			log.Printf("Slide generation failed: %v", err)
		} else {
			log.Println("Slides generated.")
		}
	}()

	// B. Audio (Parallel per section)
	sem := make(chan struct{}, 5)
	audioResults := make(chan AssetResult, len(sections)+1)
	var audioWg sync.WaitGroup

	order := append([]string{"Title"}, common.SectionOrder()...)

	for _, name := range order {
		if name == "Title" {
			continue
		}
		data, ok := sections[name]
		if !ok {
			continue
		}

		audioWg.Add(1)
		go func(n string, s string) {
			defer audioWg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			path, err := sarvam.GenerateAudio(s, filepath.Join(config.OutputDir, "audio"), n, "English")
			audioResults <- AssetResult{Name: n, AudioPath: path, Err: err}
		}(name, data.Script)
	}

	go func() {
		audioWg.Wait()
		close(audioResults)
	}()

	// Collect Audio
	audioMap := make(map[string]string)
	for res := range audioResults {
		if res.Err != nil {
			log.Printf("Audio gen failed for %s: %v", res.Name, res.Err)
		} else {
			audioMap[res.Name] = res.AudioPath
		}
	}

	// Wait for slides
	assetWg.Wait()

	if sectionSlides == nil {
		return fmt.Errorf("slides failed to generate, cannot proceed to video")
	}

	// 5. Combine into Segments (Parallel)
	log.Println("Step 5: Creating Video Segments...")

	segmentMap := make(map[int]string)
	var segMutex sync.Mutex
	var segWg sync.WaitGroup

	processSegment := func(index int, imgs []string, audio string, segName string) {
		defer segWg.Done()
		segPath, err := videoGen.CreateSegment(imgs, audio, segName)
		if err == nil {
			segMutex.Lock()
			segmentMap[index] = segPath
			segMutex.Unlock()
		} else {
			log.Printf("Failed to create segment %s: %v", segName, err)
		}
	}

	// Intro
	if introAudio, ok := audioMap["Introduction"]; ok {
		imgs := []string{titleSlide}
		if sSlides, ok := sectionSlides["Introduction"]; ok {
			imgs = append(imgs, sSlides...)
		}
		segWg.Add(1)
		go processSegment(0, imgs, introAudio, "01_intro_seg.mp4")
	}

	// Other sections
	sectionOrder := common.SectionOrder()
	for i, name := range sectionOrder {
		if name == "Introduction" {
			continue
		}
		audioPath, haveAudio := audioMap[name]
		slides, haveSlides := sectionSlides[name]

		if haveAudio && haveSlides {
			segName := fmt.Sprintf("%02d_%s_seg.mp4", i+1, strings.ToLower(name))
			segIdx := i
			segWg.Add(1)
			go processSegment(segIdx, slides, audioPath, segName)
		}
	}

	segWg.Wait()

	// 6. Final Concat
	log.Println("Step 6: Final Concatenation...")
	var segments []string
	for i := 0; i < len(sectionOrder); i++ {
		if path, ok := segmentMap[i]; ok {
			segments = append(segments, path)
		}
	}

	if len(segments) == 0 {
		return fmt.Errorf("no video segments created")
	}

	finalVideo, err := videoGen.ConcatSegments(segments, "final_video.mp4")
	if err != nil {
		return fmt.Errorf("final video creation failed: %w", err)
	}

	log.Printf("Video Pipeline Complete! Video: %s", finalVideo)
	return nil
}
