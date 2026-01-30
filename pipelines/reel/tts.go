package reel

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ReelTTSClient handles TTS generation for reel dialogues
type ReelTTSClient struct {
	APIKey string
	sem    chan struct{} // Semaphore to limit concurrent API calls
}

// Global semaphore to limit concurrent TTS API requests
var globalReelTTSSem = make(chan struct{}, 2)

// NewReelTTSClient creates a new TTS client for reel audio
func NewReelTTSClient(apiKey string) *ReelTTSClient {
	return &ReelTTSClient{
		APIKey: apiKey,
		sem:    globalReelTTSSem,
	}
}

// synthesizeText generates audio for a single text chunk
func (c *ReelTTSClient) synthesizeText(text, outputPath, languageCode, voice string) error {
	c.sem <- struct{}{}
	defer func() { <-c.sem }()

	text = cleanTextForTTS(text)
	if text == "" {
		return fmt.Errorf("empty text after cleaning")
	}

	chunks := splitTextIntoChunks(text, 500)

	if len(chunks) == 1 {
		return c.synthesizeChunk(chunks[0], outputPath, languageCode, voice)
	}

	tempDir := filepath.Join(filepath.Dir(outputPath), "temp_chunks")
	os.MkdirAll(tempDir, 0755)

	var chunkFiles []string
	baseName := strings.TrimSuffix(filepath.Base(outputPath), filepath.Ext(outputPath))

	for i, chunk := range chunks {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("%s_chunk_%03d.wav", baseName, i))
		if err := c.synthesizeChunk(chunk, chunkPath, languageCode, voice); err != nil {
			log.Printf("[TTS] Error on chunk %d: %v", i, err)
			continue
		}
		chunkFiles = append(chunkFiles, chunkPath)
	}

	if len(chunkFiles) == 0 {
		return fmt.Errorf("no audio chunks generated")
	}

	if len(chunkFiles) == 1 {
		data, err := os.ReadFile(chunkFiles[0])
		if err != nil {
			return err
		}
		return os.WriteFile(outputPath, data, 0644)
	}

	return concatenateAudioFiles(chunkFiles, outputPath, tempDir, baseName)
}

// synthesizeChunk makes the API call to generate audio for a text chunk
func (c *ReelTTSClient) synthesizeChunk(text, outputPath, languageCode, voice string) error {
	url := "https://api.sarvam.ai/text-to-speech"

	payload := map[string]interface{}{
		"inputs":               []string{text},
		"target_language_code": languageCode,
		"speaker":              voice,
		"speech_sample_rate":   22050,
		"enable_preprocessing": true,
		"model":                "bulbul:v2",
	}

	jsonPayload, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 60 * time.Second}

	var resp *http.Response
	var err error

	for attempts := 0; attempts < 3; attempts++ {
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("api-subscription-key", c.APIKey)

		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(time.Duration(attempts+1) * 2 * time.Second)
	}

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error: %d - %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	audios, ok := result["audios"].([]interface{})
	if !ok || len(audios) == 0 {
		return fmt.Errorf("no audio in response")
	}

	audioStr, ok := audios[0].(string)
	if !ok {
		return fmt.Errorf("invalid audio format")
	}

	if idx := strings.Index(audioStr, ","); idx != -1 {
		audioStr = audioStr[idx+1:]
	}

	audioBytes, err := base64.StdEncoding.DecodeString(audioStr)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, audioBytes, 0644)
}

func concatenateAudioFiles(files []string, outputPath, tempDir, baseName string) error {
	listContent := ""
	for _, f := range files {
		absPath, _ := filepath.Abs(f)
		listContent += fmt.Sprintf("file '%s'\n", absPath)
	}

	listPath := filepath.Join(tempDir, baseName+"_list.txt")
	if err := os.WriteFile(listPath, []byte(listContent), 0644); err != nil {
		return err
	}

	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", outputPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[TTS] ffmpeg error: %s", string(output))
		data, _ := os.ReadFile(files[0])
		os.WriteFile(outputPath, data, 0644)
		return nil
	}

	return nil
}

func cleanTextForTTS(text string) string {
	text = strings.ReplaceAll(text, "**", "")
	text = strings.ReplaceAll(text, "*", "")

	for strings.Contains(text, "#") {
		idx := strings.Index(text, "#")
		endIdx := strings.Index(text[idx:], " ")
		if endIdx == -1 {
			text = text[:idx]
		} else {
			text = text[:idx] + text[idx+endIdx:]
		}
	}

	var result strings.Builder
	for _, r := range text {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == ' ' || r == '.' || r == ',' || r == '!' || r == '?' || r == ';' ||
			r == ':' || r == '-' || r == '(' || r == ')' || r == '"' || r == '\'' ||
			r > 127 {
			result.WriteRune(r)
		} else {
			result.WriteRune(' ')
		}
	}

	text = result.String()
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return strings.TrimSpace(text)
}

func splitTextIntoChunks(text string, maxLength int) []string {
	if len(text) <= maxLength {
		return []string{text}
	}

	var chunks []string
	sentences := splitOnSentences(text)

	currentChunk := ""
	for _, sentence := range sentences {
		if len(currentChunk)+len(sentence)+1 <= maxLength {
			currentChunk += sentence + " "
		} else {
			if currentChunk != "" {
				chunks = append(chunks, strings.TrimSpace(currentChunk))
			}
			currentChunk = sentence + " "
		}
	}
	if currentChunk != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

func splitOnSentences(text string) []string {
	var sentences []string
	current := ""

	for _, r := range text {
		current += string(r)
		if r == '.' || r == '!' || r == '?' || r == '।' || r == '॥' {
			sentences = append(sentences, strings.TrimSpace(current))
			current = ""
		}
	}
	if current != "" {
		sentences = append(sentences, strings.TrimSpace(current))
	}

	return sentences
}
