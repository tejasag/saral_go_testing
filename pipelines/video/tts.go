package video

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type SarvamClient struct {
	APIKey string
}

func NewSarvamClient(apiKey string) *SarvamClient {
	return &SarvamClient{APIKey: apiKey}
}

func (s *SarvamClient) GenerateAudio(text, outputDir, filename, language string) (string, error) {
	// 1. Clean Text
	text = cleanTextForTTS(text)
	if text == "" {
		return "", fmt.Errorf("empty text after cleaning")
	}

	// 2. Chunk Text
	chunks := splitTextIntoChunks(text, 500)

	tempDir := filepath.Join(outputDir, "temp_chunks")
	os.MkdirAll(tempDir, 0755)

	var chunkFiles []string

	// 3. Process Chunks
	for i, chunk := range chunks {
		chunkPath := filepath.Join(tempDir, fmt.Sprintf("%s_chunk_%03d.wav", filename, i))
		err := s.synthesizeChunk(chunk, chunkPath, language)
		if err != nil {
			fmt.Printf("Error processing chunk %d: %v\n", i, err)
			continue
		}
		chunkFiles = append(chunkFiles, chunkPath)
	}

	if len(chunkFiles) == 0 {
		return "", fmt.Errorf("no audio chunks generated")
	}

	// 4. Concatenate
	finalPath := filepath.Join(outputDir, fmt.Sprintf("%s.wav", filename))

	if len(chunkFiles) == 1 {
		input, err := os.ReadFile(chunkFiles[0])
		if err != nil {
			return "", err
		}
		err = os.WriteFile(finalPath, input, 0644)
		return finalPath, err
	}

	// Use ffmpeg to concat
	listFileVal := ""
	for _, f := range chunkFiles {
		absPath, _ := filepath.Abs(f)
		listFileVal += fmt.Sprintf("file '%s'\n", absPath)
	}
	listPath := filepath.Join(tempDir, filename+"_list.txt")
	os.WriteFile(listPath, []byte(listFileVal), 0644)

	cmd := exec.Command("ffmpeg", "-y", "-f", "concat", "-safe", "0", "-i", listPath, "-c", "copy", finalPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("ffmpeg error: %s\n", string(output))
		// Fallback to first chunk
		input, _ := os.ReadFile(chunkFiles[0])
		os.WriteFile(finalPath, input, 0644)
		return finalPath, nil
	}

	return finalPath, nil
}

func (s *SarvamClient) synthesizeChunk(text, outputPath, language string) error {
	url := "https://api.sarvam.ai/text-to-speech"

	targetLang := "en-IN"
	voice := "vidya"
	if language == "Hindi" {
		targetLang = "hi-IN"
	}

	payload := map[string]interface{}{
		"inputs":               []string{text},
		"target_language_code": targetLang,
		"speaker":              voice,
		"speech_sample_rate":   22050,
		"enable_preprocessing": true,
		"model":                "bulbul:v2",
	}

	jsonPayload, _ := json.Marshal(payload)
	client := &http.Client{Timeout: 60 * time.Second}

	var resp *http.Response
	var err error

	// Retry loop
	for attempts := 0; attempts < 3; attempts++ {
		req, _ := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("api-subscription-key", s.APIKey)

		resp, err = client.Do(req)
		if err == nil && resp.StatusCode == 200 {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(2 * time.Second)
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

	// Strip header if present
	if idx := strings.Index(audioStr, ","); idx != -1 {
		audioStr = audioStr[idx+1:]
	}

	audioBytes, err := base64.StdEncoding.DecodeString(audioStr)
	if err != nil {
		return err
	}

	return os.WriteFile(outputPath, audioBytes, 0644)
}

func cleanTextForTTS(text string) string {
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`#+\s*`).ReplaceAllString(text, "")
	text = regexp.MustCompile(`[^\w\s.,!?;:\-()\"']`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

func splitTextIntoChunks(text string, maxLength int) []string {
	if len(text) <= maxLength {
		return []string{text}
	}

	var chunks []string
	sentences := regexp.MustCompile(`[.!?]+\s+`).Split(text, -1)

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
