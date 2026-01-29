package main

import (
	"flag"
	"log"
	"os"
	"runtime"
	"time"

	"saral_go_testing/common"
	"saral_go_testing/pipelines/poster"
	"saral_go_testing/pipelines/video"
)

func main() {
	mode := flag.String("mode", "video", "Pipeline mode: 'video' or 'poster'")
	serverMode := flag.Bool("server", false, "Run as HTTP server")
	port := flag.String("port", ":8080", "Server port (only with --server)")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of worker goroutines (only with --server)")
	flag.Parse()

	if *serverMode {
		StartServer(*port, *workers)
		return
	}

	args := flag.Args()
	if len(args) < 1 {
		log.Fatal("Usage: go run . [--mode=video|poster] <pdf_path>\n       go run . --server [--port=:8080] [--workers=4]")
	}
	pdfPath := args[0]

	if err := common.LoadEnv(".env"); err != nil {
		log.Println("No .env file found or error reading it")
	}

	config := common.PipelineConfig{
		PDFPath:   pdfPath,
		OutputDir: "./output/output_" + time.Now().Format("20060102_150405"),
		GeminiKey: os.Getenv("GEMINI_API_KEY"),
		SarvamKey: os.Getenv("SARVAM_API_KEY"),
		Mode:      *mode,
	}

	if config.GeminiKey == "" {
		log.Fatal("Please set GEMINI_API_KEY environment variable")
	}

	if *mode == "video" && config.SarvamKey == "" {
		log.Fatal("Please set SARVAM_API_KEY environment variable for video mode")
	}

	var err error
	switch *mode {
	case "video":
		log.Println("Running Video Pipeline...")
		err = video.ProcessVideoPipeline(config)
	case "poster":
		log.Println("Running Poster Pipeline...")
		err = poster.ProcessPosterPipeline(config)
	default:
		log.Fatalf("Unknown mode: %s. Use 'video' or 'poster'", *mode)
	}

	if err != nil {
		log.Fatalf("Pipeline failed: %v", err)
	}

	log.Println("Pipeline completed successfully!")
}
