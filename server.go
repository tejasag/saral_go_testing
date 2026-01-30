package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"saral_go_testing/common"
	"saral_go_testing/pipelines/poster"
	"saral_go_testing/pipelines/reel"
	"saral_go_testing/pipelines/video"
)

type JobStatus struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	Mode      string     `json:"mode"`
	OutputDir string     `json:"output_dir,omitempty"`
	Error     string     `json:"error,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
}

type WorkerPool struct {
	jobs       chan *Job
	results    map[string]*JobStatus
	mu         sync.RWMutex
	wg         sync.WaitGroup
	numWorkers int
}

type Job struct {
	ID        string
	PDFPath   string
	OutputDir string
	Mode      string
	Config    common.PipelineConfig
}

func NewWorkerPool(numWorkers int, bufferSize int) *WorkerPool {
	pool := &WorkerPool{
		jobs:       make(chan *Job, bufferSize),
		results:    make(map[string]*JobStatus),
		numWorkers: numWorkers,
	}
	pool.Start()
	return pool
}

func (p *WorkerPool) Start() {
	for i := 0; i < p.numWorkers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	log.Printf("Started %d workers", p.numWorkers)
}

func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	for job := range p.jobs {
		log.Printf("[Worker %d] Processing job %s (mode: %s)", id, job.ID, job.Mode)
		p.processJob(job)
	}
	log.Printf("[Worker %d] Shutting down", id)
}

func (p *WorkerPool) processJob(job *Job) {
	p.updateStatus(job.ID, "processing", "")

	var err error
	switch job.Mode {
	case "video":
		err = video.ProcessVideoPipeline(job.Config)
	case "poster":
		err = poster.ProcessPosterPipeline(job.Config)
	case "reel":
		err = reel.ProcessReelPipeline(job.Config)
	default:
		err = fmt.Errorf("unknown mode: %s", job.Mode)
	}

	if err != nil {
		p.updateStatus(job.ID, "failed", err.Error())
		log.Printf("[Job %s] Failed: %v", job.ID, err)
	} else {
		p.updateStatus(job.ID, "completed", "")
		log.Printf("[Job %s] Completed successfully", job.ID)
	}
}

func (p *WorkerPool) updateStatus(jobID, status, errMsg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if job, exists := p.results[jobID]; exists {
		job.Status = status
		job.Error = errMsg
		if status == "completed" || status == "failed" {
			now := time.Now()
			job.DoneAt = &now
		}
	}
}

func (p *WorkerPool) Submit(job *Job) {
	p.mu.Lock()
	p.results[job.ID] = &JobStatus{
		ID:        job.ID,
		Status:    "queued",
		Mode:      job.Mode,
		OutputDir: job.OutputDir,
		StartedAt: time.Now(),
	}
	p.mu.Unlock()

	p.jobs <- job
}

func (p *WorkerPool) GetStatus(jobID string) (*JobStatus, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	status, ok := p.results[jobID]
	return status, ok
}

func (p *WorkerPool) Shutdown() {
	close(p.jobs)
	p.wg.Wait()
}

type Server struct {
	pool      *WorkerPool
	geminiKey string
	sarvamKey string
	uploadDir string
}

func NewServer(numWorkers int) *Server {
	if err := common.LoadEnv(".env"); err != nil {
		log.Println("No .env file found")
	}

	geminiKey := os.Getenv("GEMINI_API_KEY")
	if geminiKey == "" {
		log.Fatal("GEMINI_API_KEY not set")
	}

	uploadDir := "./uploads"
	os.MkdirAll(uploadDir, 0755)

	return &Server{
		pool:      NewWorkerPool(numWorkers, 100),
		geminiKey: geminiKey,
		sarvamKey: os.Getenv("SARVAM_API_KEY"),
		uploadDir: uploadDir,
	}
}

func (s *Server) handlePDFUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "video"
	}
	if mode != "video" && mode != "poster" && mode != "reel" {
		http.Error(w, "Invalid mode. Use 'video', 'poster', or 'reel'", http.StatusBadRequest)
		return
	}

	if (mode == "video" || mode == "reel") && s.sarvamKey == "" {
		http.Error(w, "SARVAM_API_KEY not configured for "+mode+" mode", http.StatusInternalServerError)
		return
	}

	r.ParseMultipartForm(100 << 20)

	file, header, err := r.FormFile("pdf")
	if err != nil {
		http.Error(w, "Failed to get PDF file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	if filepath.Ext(header.Filename) != ".pdf" {
		http.Error(w, "Only PDF files are accepted", http.StatusBadRequest)
		return
	}

	jobID := fmt.Sprintf("%d", time.Now().UnixNano())
	pdfPath := filepath.Join(s.uploadDir, jobID+"_"+header.Filename)
	outputDir := "./output/output_" + jobID

	dst, err := os.Create(pdfPath)
	if err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		http.Error(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	job := &Job{
		ID:        jobID,
		PDFPath:   pdfPath,
		OutputDir: outputDir,
		Mode:      mode,
		Config: common.PipelineConfig{
			PDFPath:   pdfPath,
			OutputDir: outputDir,
			GeminiKey: s.geminiKey,
			SarvamKey: s.sarvamKey,
			Mode:      mode,
		},
	}

	s.pool.Submit(job)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"job_id":  jobID,
		"status":  "queued",
		"message": "PDF uploaded and queued for processing",
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.URL.Query().Get("id")
	if jobID == "" {
		http.Error(w, "Missing job id", http.StatusBadRequest)
		return
	}

	status, ok := s.pool.GetStatus(jobID)
	if !ok {
		http.Error(w, "Job not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "ok",
		"workers":     s.pool.numWorkers,
		"goroutines":  runtime.NumGoroutine(),
		"queued_jobs": len(s.pool.jobs),
	})
}

func (s *Server) catchAllHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handlePDFUpload(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "PDF Processing Server",
		"usage":   "POST any route with 'pdf' form field. Query params: ?mode=video|poster",
		"status":  "GET /status?id=<job_id>",
		"health":  "GET /health",
	})
}

func (s *Server) Shutdown(ctx context.Context) {
	s.pool.Shutdown()
}

func StartServer(addr string, numWorkers int) {
	server := NewServer(numWorkers)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/status", server.handleStatus)
	mux.HandleFunc("/", server.catchAllHandler)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
	}

	log.Printf("Server starting on %s with %d workers", addr, numWorkers)
	log.Printf("POST to any route with 'pdf' form field and ?mode=video|poster|reel to process")

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}
