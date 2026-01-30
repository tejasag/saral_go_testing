package reel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DialogueTurn represents a single dialogue exchange
type DialogueTurn struct {
	Character string `json:"character"` // "Person1" (female) or "Person2" (male)
	Dialogue  string `json:"dialogue"`
}

// ReelScript holds the parsed dialogue script
type ReelScript struct {
	OriginalDialogue string         `json:"original_dialogue"`
	ParsedScript     []DialogueTurn `json:"parsed_script"`
	EditedScript     []DialogueTurn `json:"edited_script,omitempty"`
}

// AvatarPair represents a pair of avatars for the reel
type AvatarPair struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MaleAvatar   string `json:"male_avatar"`   // Person2
	FemaleAvatar string `json:"female_avatar"` // Person1
	Description  string `json:"description"`
}

// AvatarSelection stores the selected avatar pair
type AvatarSelection struct {
	AvatarPairID string `json:"avatar_pair_id"`
	MaleAvatar   string `json:"male_avatar"`
	FemaleAvatar string `json:"female_avatar"`
}

// ReelJobStatus tracks the state of a reel generation job
type ReelJobStatus struct {
	PaperID         string           `json:"paper_id"`
	Status          string           `json:"status"` // processing, script_ready, script_edited, avatars_selected, completed, failed
	Stage           string           `json:"stage"`
	Language        string           `json:"language"`
	Filename        string           `json:"filename,omitempty"`
	SourceType      string           `json:"source_type,omitempty"` // pdf, arxiv, latex
	ScriptData      *ReelScript      `json:"script_data,omitempty"`
	AvatarSelection *AvatarSelection `json:"avatar_selection,omitempty"`
	Metadata        *PaperMetadata   `json:"metadata,omitempty"`
	VideoPath       string           `json:"video_path,omitempty"`
	ErrorMessage    string           `json:"error_message,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty"`
}

// PaperMetadata holds extracted paper information
type PaperMetadata struct {
	Title   string `json:"title"`
	Authors string `json:"authors"`
	Date    string `json:"date,omitempty"`
}

// ReelConfig holds configuration for the reel pipeline
type ReelConfig struct {
	PDFPath      string
	OutputDir    string
	Language     string
	GeminiKey    string
	SarvamKey    string
	AvatarPairID string
	AssetsDir    string
}

// Available avatar pairs
var AvailableAvatarPairs = []AvatarPair{
	{
		ID:           "male1_female1",
		Name:         "Male 1 & Female 1",
		MaleAvatar:   "prof1.png",
		FemaleAvatar: "student1.png",
		Description:  "Two person avatar pair",
	},
	{
		ID:           "male1_female2",
		Name:         "Male 1 & Female 2",
		MaleAvatar:   "prof1.png",
		FemaleAvatar: "student2.png",
		Description:  "Two person avatar pair",
	},
	{
		ID:           "male2_female1",
		Name:         "Male 2 & Female 1",
		MaleAvatar:   "prof2.png",
		FemaleAvatar: "student1.png",
		Description:  "Two person avatar pair",
	},
	{
		ID:           "male2_female2",
		Name:         "Male 2 & Female 2",
		MaleAvatar:   "prof2.png",
		FemaleAvatar: "student2.png",
		Description:  "Two person avatar pair",
	},
}

// GetAvatarPairByID finds an avatar pair by its ID
func GetAvatarPairByID(id string) *AvatarPair {
	for _, pair := range AvailableAvatarPairs {
		if pair.ID == id {
			return &pair
		}
	}
	return nil
}

// JobStatusManager handles reading/writing job status files
type JobStatusManager struct {
	StatusDir string
	mu        sync.RWMutex
}

// NewJobStatusManager creates a new status manager
func NewJobStatusManager(statusDir string) *JobStatusManager {
	os.MkdirAll(statusDir, 0755)
	return &JobStatusManager{StatusDir: statusDir}
}

// GetStatusFilePath returns the path to the status file for a paper
func (m *JobStatusManager) GetStatusFilePath(paperID string) string {
	return filepath.Join(m.StatusDir, paperID+".json")
}

// UpdateStatus updates the job status, preserving existing data
func (m *JobStatusManager) UpdateStatus(status *ReelJobStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	statusFile := m.GetStatusFilePath(status.PaperID)

	// Load existing data if file exists
	if existing, err := m.loadStatusUnsafe(status.PaperID); err == nil && existing != nil {
		// Merge: preserve existing fields that aren't set in new status
		if status.ScriptData == nil {
			status.ScriptData = existing.ScriptData
		}
		if status.AvatarSelection == nil {
			status.AvatarSelection = existing.AvatarSelection
		}
		if status.Metadata == nil {
			status.Metadata = existing.Metadata
		}
		if status.Language == "" {
			status.Language = existing.Language
		}
		if status.Filename == "" {
			status.Filename = existing.Filename
		}
		if status.SourceType == "" {
			status.SourceType = existing.SourceType
		}
		if status.CreatedAt.IsZero() {
			status.CreatedAt = existing.CreatedAt
		}
	}

	status.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(statusFile, data, 0644)
}

// GetStatus retrieves the job status
func (m *JobStatusManager) GetStatus(paperID string) (*ReelJobStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.loadStatusUnsafe(paperID)
}

// loadStatusUnsafe loads status without locking (caller must hold lock)
func (m *JobStatusManager) loadStatusUnsafe(paperID string) (*ReelJobStatus, error) {
	statusFile := m.GetStatusFilePath(paperID)

	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil, err
	}

	var status ReelJobStatus
	if err := json.Unmarshal(data, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// LanguageCodes maps language names to Sarvam TTS codes
var LanguageCodes = map[string]string{
	"english":   "en-IN",
	"hindi":     "hi-IN",
	"tamil":     "ta-IN",
	"bengali":   "bn-IN",
	"telugu":    "te-IN",
	"kannada":   "kn-IN",
	"malayalam": "ml-IN",
	"marathi":   "mr-IN",
	"gujarati":  "gu-IN",
	"punjabi":   "pa-IN",
	"odia":      "od-IN",
}

// GetLanguageCode returns the Sarvam TTS language code
func GetLanguageCode(language string) string {
	if code, ok := LanguageCodes[language]; ok {
		return code
	}
	return "en-IN" // Default to English
}
