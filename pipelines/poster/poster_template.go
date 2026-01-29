package poster

import (
	"fmt"
	"path/filepath"
	"strings"

	"saral_go_testing/common"
)

// PosterTemplate generates LaTeX content for academic posters
type PosterTemplate struct {
	Width      int    // Poster width in cm
	Height     int    // Poster height in cm
	NumColumns int    // Number of columns
	ColorTheme string // Color theme name
}

// NewPosterTemplate creates a new poster template with default settings
func NewPosterTemplate() *PosterTemplate {
	return &PosterTemplate{
		Width:      120,
		Height:     72,
		NumColumns: 3,
		ColorTheme: "default",
	}
}

// GenerateLatex generates the complete LaTeX document for the poster
func (t *PosterTemplate) GenerateLatex(content *common.PosterContent, imagePaths []string) string {
	var sb strings.Builder

	// Preamble
	sb.WriteString(t.generatePreamble())

	// Title and Authors
	sb.WriteString(t.generateTitleBlock(content))

	// Document body
	sb.WriteString("\\begin{document}\n")
	sb.WriteString("\\begin{frame}[t]\n")
	sb.WriteString("\\begin{columns}[t]\n")
	sb.WriteString("\\separatorcolumn\n\n")

	// Column content distribution
	switch t.NumColumns {
	case 3:
		sb.WriteString(t.generateThreeColumnLayout(content, imagePaths))
	case 2:
		sb.WriteString(t.generateTwoColumnLayout(content, imagePaths))
	default:
		sb.WriteString(t.generateThreeColumnLayout(content, imagePaths))
	}

	sb.WriteString("\\separatorcolumn\n")
	sb.WriteString("\\end{columns}\n")
	sb.WriteString("\\end{frame}\n")
	sb.WriteString("\\end{document}\n")

	return sb.String()
}

func (t *PosterTemplate) generatePreamble() string {
	colWidth := (100 - (float64(t.NumColumns+1) * 2.5)) / float64(t.NumColumns) / 100

	return fmt.Sprintf(`\documentclass[final]{beamer}

%%%% Packages %%%%
\usepackage[T1]{fontenc}
\usepackage{lmodern}
\usepackage[size=custom,width=%d,height=%d,scale=1.2]{beamerposter}
\usetheme{gemini}
\usecolortheme{gemini}
\usepackage{graphicx}
\usepackage{booktabs}
\usepackage{tikz}
\usepackage{pgfplots}
\pgfplotsset{compat=1.14}
\usepackage{anyfontsize}
\usepackage{ragged2e}

%%%% Lengths %%%%
\newlength{\sepwidth}
\newlength{\colwidth}
\setlength{\sepwidth}{0.025\paperwidth}
\setlength{\colwidth}{%.3f\paperwidth}

\newcommand{\separatorcolumn}{\begin{column}{\sepwidth}\end{column}}

`, t.Width, t.Height, colWidth)
}

func (t *PosterTemplate) generateTitleBlock(content *common.PosterContent) string {
	title := common.EscapeLatex(content.Title)
	if title == "" {
		title = "Research Poster"
	}

	authors := common.EscapeLatex(content.Authors)
	if authors == "" {
		authors = "Anonymous"
	}

	return fmt.Sprintf(`%%%% Title %%%%
\title{%s}
\author{%s}
\institute[]{}

`, title, authors)
}

func (t *PosterTemplate) generateThreeColumnLayout(content *common.PosterContent, imagePaths []string) string {
	var sb strings.Builder

	// Column 1: Abstract, Introduction, Methodology
	sb.WriteString("\\begin{column}{\\colwidth}\n\n")

	// Abstract block
	if content.Abstract != "" {
		sb.WriteString(t.generateBlock("Abstract", content.Abstract, false))
	}

	// Introduction block
	if len(content.Introduction) > 0 {
		sb.WriteString(t.generateBulletBlock("Introduction", content.Introduction))
	}

	// Methodology block
	if len(content.Methodology) > 0 {
		sb.WriteString(t.generateBulletBlock("Methodology", content.Methodology))
	}

	sb.WriteString("\\end{column}\n\n")
	sb.WriteString("\\separatorcolumn\n\n")

	// Column 2: Results (main findings with potential images)
	sb.WriteString("\\begin{column}{\\colwidth}\n\n")

	if len(content.Results) > 0 {
		sb.WriteString(t.generateResultsBlock(content.Results, imagePaths))
	}

	sb.WriteString("\\end{column}\n\n")
	sb.WriteString("\\separatorcolumn\n\n")

	// Column 3: Conclusion, References
	sb.WriteString("\\begin{column}{\\colwidth}\n\n")

	if len(content.Conclusion) > 0 {
		sb.WriteString(t.generateBulletBlock("Conclusion", content.Conclusion))
	}

	if len(content.References) > 0 {
		sb.WriteString(t.generateReferencesBlock(content.References))
	}

	sb.WriteString("\\end{column}\n\n")

	return sb.String()
}

func (t *PosterTemplate) generateTwoColumnLayout(content *common.PosterContent, imagePaths []string) string {
	var sb strings.Builder

	// Column 1: Abstract, Introduction, Methodology
	sb.WriteString("\\begin{column}{\\colwidth}\n\n")

	if content.Abstract != "" {
		sb.WriteString(t.generateBlock("Abstract", content.Abstract, false))
	}

	if len(content.Introduction) > 0 {
		sb.WriteString(t.generateBulletBlock("Introduction", content.Introduction))
	}

	if len(content.Methodology) > 0 {
		sb.WriteString(t.generateBulletBlock("Methodology", content.Methodology))
	}

	sb.WriteString("\\end{column}\n\n")
	sb.WriteString("\\separatorcolumn\n\n")

	// Column 2: Results, Conclusion, References
	sb.WriteString("\\begin{column}{\\colwidth}\n\n")

	if len(content.Results) > 0 {
		sb.WriteString(t.generateResultsBlock(content.Results, imagePaths))
	}

	if len(content.Conclusion) > 0 {
		sb.WriteString(t.generateBulletBlock("Conclusion", content.Conclusion))
	}

	if len(content.References) > 0 {
		sb.WriteString(t.generateReferencesBlock(content.References))
	}

	sb.WriteString("\\end{column}\n\n")

	return sb.String()
}

func (t *PosterTemplate) generateBlock(title, content string, isAlert bool) string {
	blockType := "block"
	if isAlert {
		blockType = "alertblock"
	}

	return fmt.Sprintf(`\begin{%s}{%s}
%s
\end{%s}

`, blockType, title, common.EscapeLatex(content), blockType)
}

func (t *PosterTemplate) generateBulletBlock(title string, bullets []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("\\begin{block}{%s}\n", title))
	sb.WriteString("\\begin{itemize}\n")

	for _, bullet := range bullets {
		sb.WriteString(fmt.Sprintf("  \\item %s\n", common.EscapeLatex(bullet)))
	}

	sb.WriteString("\\end{itemize}\n")
	sb.WriteString("\\end{block}\n\n")

	return sb.String()
}

func (t *PosterTemplate) generateResultsBlock(results []string, imagePaths []string) string {
	var sb strings.Builder

	sb.WriteString("\\begin{block}{Results}\n")

	// Add bullet points first
	sb.WriteString("\\begin{itemize}\n")
	for _, result := range results {
		sb.WriteString(fmt.Sprintf("  \\item %s\n", common.EscapeLatex(result)))
	}
	sb.WriteString("\\end{itemize}\n")

	// Add images if available (limit to 2 for space)
	if len(imagePaths) > 0 {
		sb.WriteString("\n\\vspace{1em}\n")
		maxImages := 2
		if len(imagePaths) < maxImages {
			maxImages = len(imagePaths)
		}

		for i := 0; i < maxImages; i++ {
			// Use absolute path for the image
			absPath, err := filepath.Abs(imagePaths[i])
			if err != nil {
				continue
			}
			sb.WriteString("\\begin{figure}\n")
			sb.WriteString("\\centering\n")
			sb.WriteString(fmt.Sprintf("\\includegraphics[width=0.9\\textwidth,height=0.25\\textheight,keepaspectratio]{%s}\n", absPath))
			sb.WriteString(fmt.Sprintf("\\caption{Figure %d}\n", i+1))
			sb.WriteString("\\end{figure}\n")
		}
	}

	sb.WriteString("\\end{block}\n\n")

	return sb.String()
}

func (t *PosterTemplate) generateReferencesBlock(refs []string) string {
	var sb strings.Builder

	sb.WriteString("\\begin{block}{References}\n")
	sb.WriteString("\\footnotesize\n")
	sb.WriteString("\\begin{enumerate}\n")

	for _, ref := range refs {
		sb.WriteString(fmt.Sprintf("  \\item %s\n", common.EscapeLatex(ref)))
	}

	sb.WriteString("\\end{enumerate}\n")
	sb.WriteString("\\end{block}\n\n")

	return sb.String()
}
