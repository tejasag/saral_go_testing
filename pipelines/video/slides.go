package video

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"saral_go_testing/common"

	"github.com/gen2brain/go-fitz"
)

type SlideGenerator struct {
	OutputDir string
}

func NewSlideGenerator(outputDir string) *SlideGenerator {
	return &SlideGenerator{OutputDir: outputDir}
}

func (s *SlideGenerator) GenerateSlides(paperID string, title string, authors string, sections map[string]common.SectionData) (string, map[string][]string, string, error) {
	// 1. Generate LaTeX
	latexContent := s.generateLatex(title, authors, sections)

	// 2. Write to file
	if err := os.MkdirAll(s.OutputDir, 0755); err != nil {
		return "", nil, "", fmt.Errorf("error creating output dir: %w", err)
	}
	texFile := filepath.Join(s.OutputDir, fmt.Sprintf("%s_presentation.tex", paperID))
	err := os.WriteFile(texFile, []byte(latexContent), 0644)
	if err != nil {
		return "", nil, "", fmt.Errorf("error writing tex file: %w", err)
	}

	// 3. Compile
	pdfPath, err := s.compileLatex(texFile)
	if err != nil {
		return "", nil, "", err
	}

	// 4. Convert to Images
	titleSlide, sectionSlides, err := s.convertToImagesWithMapping(pdfPath, sections)
	return titleSlide, sectionSlides, pdfPath, err
}

func (s *SlideGenerator) generateLatex(title string, author string, sections map[string]common.SectionData) string {
	var sb strings.Builder

	// Header
	sb.WriteString(`\documentclass[aspectratio=169]{beamer}
\usetheme{Madrid}
\usecolortheme{whale}
\usepackage{graphicx}
\usepackage{ragged2e}

\title{` + common.EscapeLatex(title) + `}
\author{` + common.EscapeLatex(author) + `}
\date{\today}

\begin{document}

\begin{frame}
\titlepage
\end{frame}
`)

	// Sections - Order matters
	order := common.SectionOrder()

	for _, name := range order {
		data, ok := sections[name]
		if !ok {
			continue
		}

		sb.WriteString(fmt.Sprintf("\\section{%s}\n", name))

		sb.WriteString("\\begin{frame}{" + name + "}\n")
		sb.WriteString("\\begin{itemize}\n")
		for _, b := range data.Bullets {
			sb.WriteString("\\item " + common.EscapeLatex(b) + "\n")
		}
		sb.WriteString("\\end{itemize}\n")
		sb.WriteString("\\end{frame}\n")

		if data.Image != "" {
			sb.WriteString("\\begin{frame}{" + name + " - Visualization}\n")
			sb.WriteString("\\begin{center}\n")
			absImg, _ := filepath.Abs(data.Image)
			sb.WriteString(fmt.Sprintf("\\includegraphics[width=0.8\\textwidth,height=0.8\\textheight,keepaspectratio]{%s}\n", absImg))
			sb.WriteString("\\end{center}\n")
			sb.WriteString("\\end{frame}\n")
		}
	}

	sb.WriteString("\\end{document}")
	return sb.String()
}

func (s *SlideGenerator) compileLatex(texFile string) (string, error) {
	cmd := exec.Command("pdflatex", "-interaction=nonstopmode", "-output-directory", s.OutputDir, texFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("pdflatex output: %s\n", string(output))
		return "", fmt.Errorf("pdflatex failed: %w", err)
	}

	baseName := strings.TrimSuffix(filepath.Base(texFile), ".tex")
	pdfPath := filepath.Join(s.OutputDir, baseName+".pdf")
	if _, err := os.Stat(pdfPath); os.IsNotExist(err) {
		return "", fmt.Errorf("pdf not generated")
	}
	return pdfPath, nil
}

func (s *SlideGenerator) convertToImagesWithMapping(pdfPath string, sections map[string]common.SectionData) (string, map[string][]string, error) {
	doc, err := fitz.New(pdfPath)
	if err != nil {
		return "", nil, err
	}
	defer doc.Close()

	os.MkdirAll(s.OutputDir, 0755)

	var allImages []string
	for i := 0; i < doc.NumPage(); i++ {
		img, err := doc.ImagePNG(i, 300)
		if err != nil {
			return "", nil, err
		}

		imgPath := filepath.Join(s.OutputDir, fmt.Sprintf("slide_%03d.png", i))
		err = os.WriteFile(imgPath, img, 0644)
		if err != nil {
			return "", nil, err
		}
		allImages = append(allImages, imgPath)
	}

	if len(allImages) == 0 {
		return "", nil, fmt.Errorf("no slides generated")
	}

	titleSlide := allImages[0]
	sectionSlides := make(map[string][]string)

	currentIndex := 1
	order := common.SectionOrder()

	for _, name := range order {
		data, ok := sections[name]
		if !ok {
			continue
		}

		slidesCount := 1
		if data.Image != "" {
			slidesCount++
		}

		if currentIndex+slidesCount <= len(allImages) {
			sectionSlides[name] = allImages[currentIndex : currentIndex+slidesCount]
			currentIndex += slidesCount
		} else {
			fmt.Printf("Warning: slide count mismatch for section %s\n", name)
		}
	}

	return titleSlide, sectionSlides, nil
}
