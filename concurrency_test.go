package main

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"saral_go_testing/common"
	"saral_go_testing/pipelines/video"
)

// TestConcurrentUsers simulates multiple users accessing the pipeline simultaneously
func TestConcurrentUsers(t *testing.T) {
	// Setup
	if err := common.LoadEnv(".env"); err != nil {
		t.Log("No .env file found, assuming env vars are set")
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	sarvamKey := os.Getenv("SARVAM_API_KEY")
	if apiKey == "" || sarvamKey == "" {
		t.Skip("Skipping concurrency test: API keys missing")
	}

	pdfPath := "semma.pdf"
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		t.Fatalf("Test PDF %s not found", pdfPath)
	}

	// Number of concurrent users to simulate
	concurrentUsers := 2

	var wg sync.WaitGroup
	errors := make(chan error, concurrentUsers)

	start := time.Now()

	fmt.Printf("Starting concurrency test with %d users...\n", concurrentUsers)

	for i := 0; i < concurrentUsers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Unique output dir for each user
			outputDir := fmt.Sprintf("test_output_user_%d_%d", id, time.Now().Unix())
			defer os.RemoveAll(outputDir) // Cleanup after test

			config := common.PipelineConfig{
				PDFPath:   pdfPath,
				OutputDir: outputDir,
				GeminiKey: apiKey,
				SarvamKey: sarvamKey,
			}

			fmt.Printf("[User %d] Starting pipeline...\n", id)
			if err := video.ProcessVideoPipeline(config); err != nil {
				fmt.Printf("[User %d] Failed: %v\n", id, err)
				errors <- fmt.Errorf("user %d error: %w", id, err)
			} else {
				fmt.Printf("[User %d] Success!\n", id)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	duration := time.Since(start)
	fmt.Printf("Concurrency test finished in %s\n", duration)

	failCount := 0
	for err := range errors {
		t.Errorf("%v", err)
		failCount++
	}

	if failCount == 0 {
		fmt.Printf("All %d users completed successfully.\n", concurrentUsers)
	} else {
		fmt.Printf("%d/%d users failed.\n", failCount, concurrentUsers)
	}
}
