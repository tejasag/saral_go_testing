package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

type GeminiClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

func NewGeminiClient(apiKey string) (*GeminiClient, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create gemini client: %w", err)
	}

	model := client.GenerativeModel("gemini-3-flash-preview")
	model.SetTemperature(0.7)

	return &GeminiClient{
		client: client,
		model:  model,
	}, nil
}

func (g *GeminiClient) Close() {
	g.client.Close()
}

// GenerateText generates text from a prompt (generic method for custom prompts)
func (g *GeminiClient) GenerateText(prompt string) (string, error) {
	ctx := context.Background()
	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("gemini generation error: %w", err)
	}
	return g.extractTextFromResponse(resp)
}

// PaperMetadata holds extracted paper information
type PaperMetadata struct {
	Title   string `json:"title"`
	Authors string `json:"authors"`
}

// ExtractMetadata extracts title and authors from paper text using Gemini
func (g *GeminiClient) ExtractMetadata(text string) (*PaperMetadata, error) {
	ctx := context.Background()

	// Limit text to first 2000 chars (metadata is usually at the start)
	if len(text) > 2000 {
		text = text[:2000]
	}

	prompt := fmt.Sprintf(`Extract the title and authors from this research paper text.

Return in exactly this format (no extra text):
TITLE: <paper title>
AUTHORS: <author names separated by commas>

If you cannot find the title, use "Research Paper".
If you cannot find authors, use "Authors".

Text:
%s`, text)

	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return &PaperMetadata{Title: "Research Paper", Authors: "Authors"}, err
	}

	response, err := g.extractTextFromResponse(resp)
	if err != nil {
		return &PaperMetadata{Title: "Research Paper", Authors: "Authors"}, err
	}

	// Parse the response
	metadata := &PaperMetadata{Title: "Research Paper", Authors: "Authors"}
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToUpper(line), "TITLE:") {
			metadata.Title = strings.TrimSpace(strings.TrimPrefix(line, "TITLE:"))
			metadata.Title = strings.TrimPrefix(metadata.Title, ":")
			metadata.Title = strings.TrimSpace(metadata.Title)
		} else if strings.HasPrefix(strings.ToUpper(line), "AUTHORS:") {
			metadata.Authors = strings.TrimSpace(strings.TrimPrefix(line, "AUTHORS:"))
			metadata.Authors = strings.TrimPrefix(metadata.Authors, ":")
			metadata.Authors = strings.TrimSpace(metadata.Authors)
		}
	}

	return metadata, nil
}

// GenerateScript generates a video script from text (for video pipeline)
func (g *GeminiClient) GenerateScript(text string) (string, error) {
	ctx := context.Background()
	prompt := fmt.Sprintf(`
You are an expert scriptwriter for educational videos. 
Convert the following research paper text into an engaging video script.
The script should be divided into clear sections: Introduction, Methodology, Results, Discussion, Conclusion.
Write in a conversational, easy-to-understand tone.
Do not include any visual cues or camera directions, just the spoken narration.
Make it engaging and flow well.

Text:
%s
	`, text)

	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("gemini generation error: %w", err)
	}

	return g.extractTextFromResponse(resp)
}

// GenerateBulletPoints generates bullet points for slides
func (g *GeminiClient) GenerateBulletPoints(sectionText string) ([]string, error) {
	ctx := context.Background()
	prompt := fmt.Sprintf(`
Summarize the following text into 3-5 concise bullet points suitable for a presentation slide.
Return ONLY the bullet points, one per line, starting with "- ".

Text:
%s
	`, sectionText)

	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generation error: %w", err)
	}

	text, err := g.extractTextFromResponse(resp)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(text, "\n")
	var bullets []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			bullets = append(bullets, strings.TrimPrefix(strings.TrimPrefix(trimmed, "- "), "* "))
		} else if len(trimmed) > 0 {
			bullets = append(bullets, trimmed)
		}
	}
	return bullets, nil
}

// GeneratePosterContent generates structured content for a poster
func (g *GeminiClient) GeneratePosterContent(text string) (*PosterContent, error) {
	ctx := context.Background()
	prompt := fmt.Sprintf(`
You are an expert at creating academic research posters. 
Analyze the following research paper text and generate content suitable for a large 3-column academic poster (120cm x 72cm).

IMPORTANT: The poster has significant space to fill. Generate DETAILED and COMPREHENSIVE content.

Return the content in the following format (use exactly these section headers):

TITLE: [Generate a concise, impactful title]

AUTHORS: [Extract or generate appropriate author names/affiliations]

ABSTRACT:
[Write a 4-6 sentence abstract summarizing the research problem, approach, and key findings. Be detailed.]

INTRODUCTION:
[Write 5-7 bullet points introducing the research problem, motivation, and background. Each point should be 1-2 sentences.]

METHODOLOGY:
[Write 5-7 bullet points describing the methods, architecture, and approach used. Be specific and technical.]

RESULTS:
[Write 6-8 bullet points highlighting the key findings, performance metrics, and comparisons. Include specific numbers where available.]

CONCLUSION:
[Write 4-5 bullet points summarizing conclusions, implications, limitations, and future work.]

REFERENCES:
[List 4-5 key references if identifiable from the text]

Each bullet point should be detailed and informative (1-2 sentences each).
Start each bullet point with "- ".
Fill the poster with substantive content - avoid being too brief.

Text:
%s
	`, text)

	resp, err := g.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generation error: %w", err)
	}

	text, err = g.extractTextFromResponse(resp)
	if err != nil {
		return nil, err
	}

	return parsePosterContent(text), nil
}

func (g *GeminiClient) extractTextFromResponse(resp *genai.GenerateContentResponse) (string, error) {
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("empty response from gemini")
	}

	var sb strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			sb.WriteString(string(txt))
		}
	}

	return sb.String(), nil
}

// PosterContent holds structured poster content
type PosterContent struct {
	Title        string
	Authors      string
	Abstract     string
	Introduction []string
	Methodology  []string
	Results      []string
	Conclusion   []string
	References   []string
}

// parsePosterContent parses the AI response into structured content
func parsePosterContent(text string) *PosterContent {
	content := &PosterContent{}
	lines := strings.Split(text, "\n")

	currentSection := ""
	var currentBuffer strings.Builder

	extractBullets := func(text string) []string {
		var bullets []string
		for _, line := range strings.Split(text, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- ") {
				bullets = append(bullets, strings.TrimPrefix(trimmed, "- "))
			} else if strings.HasPrefix(trimmed, "* ") {
				bullets = append(bullets, strings.TrimPrefix(trimmed, "* "))
			} else if len(trimmed) > 0 && !strings.Contains(strings.ToUpper(trimmed), ":") {
				bullets = append(bullets, trimmed)
			}
		}
		return bullets
	}

	saveSection := func() {
		bufText := strings.TrimSpace(currentBuffer.String())
		switch currentSection {
		case "TITLE":
			content.Title = bufText
		case "AUTHORS":
			content.Authors = bufText
		case "ABSTRACT":
			content.Abstract = bufText
		case "INTRODUCTION":
			content.Introduction = extractBullets(bufText)
		case "METHODOLOGY":
			content.Methodology = extractBullets(bufText)
		case "RESULTS":
			content.Results = extractBullets(bufText)
		case "CONCLUSION":
			content.Conclusion = extractBullets(bufText)
		case "REFERENCES":
			content.References = extractBullets(bufText)
		}
	}

	sectionHeaders := []string{"TITLE:", "AUTHORS:", "ABSTRACT:", "INTRODUCTION:", "METHODOLOGY:", "RESULTS:", "CONCLUSION:", "REFERENCES:"}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		foundHeader := false

		for _, header := range sectionHeaders {
			if strings.HasPrefix(strings.ToUpper(trimmed), header) {
				saveSection()
				currentSection = strings.TrimSuffix(header, ":")
				currentBuffer.Reset()
				// Check if there's content after the header on the same line
				remainder := strings.TrimSpace(strings.TrimPrefix(strings.ToUpper(trimmed), header))
				if remainder != "" {
					// Get the original case remainder
					idx := strings.Index(strings.ToUpper(trimmed), header)
					if idx >= 0 {
						actualRemainder := strings.TrimSpace(trimmed[idx+len(header):])
						currentBuffer.WriteString(actualRemainder)
						currentBuffer.WriteString("\n")
					}
				}
				foundHeader = true
				break
			}
		}

		if !foundHeader && currentSection != "" {
			currentBuffer.WriteString(line)
			currentBuffer.WriteString("\n")
		}
	}
	saveSection()

	return content
}
