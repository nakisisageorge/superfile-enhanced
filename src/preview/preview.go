// Package preview provides enhanced file preview capabilities.
// Supports syntax highlighting, image previews, PDF thumbnails, and more.
package preview

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/disintegration/imaging"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// PreviewGenerator generates previews for files.
type PreviewGenerator struct {
	syntaxHighlighter chroma.Formatter
	syntaxStyle       *chroma.Style
	markdownParser    goldmark.Markdown
	maxWidth          int
	maxHeight         int
}

// PreviewResult contains the preview data and metadata.
type PreviewResult struct {
	Data      []byte
	MimeType  string
	Width     int
	Height    int
	Truncated bool
	Error     string
}

// PreviewOptions configures preview generation.
type PreviewOptions struct {
	MaxWidth        int
	MaxHeight       int
	SyntaxTheme     string
	LineNumbers     bool
	ShowLineNumbers bool
	BackgroundColor string
}

// NewPreviewGenerator creates a new preview generator.
func NewPreviewGenerator(opts PreviewOptions) *PreviewGenerator {
	if opts.MaxWidth <= 0 {
		opts.MaxWidth = 800
	}
	if opts.MaxHeight <= 0 {
		opts.MaxHeight = 600
	}
	if opts.SyntaxTheme == "" {
		opts.SyntaxTheme = "dracula"
	}

	// chroma v2 resolves the style at Format() time; keep the resolved style on the
	// generator so every preview uses the same theme.
	style := styles.Get(opts.SyntaxTheme)
	if style == nil {
		style = styles.Get("dracula")
	}
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Get("terminal")
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
		),
	)

	return &PreviewGenerator{
		syntaxHighlighter: formatter,
		syntaxStyle:       style,
		markdownParser:    md,
		maxWidth:          opts.MaxWidth,
		maxHeight:         opts.MaxHeight,
	}
}

// GeneratePreview generates a preview for a file.
func (g *PreviewGenerator) GeneratePreview(ctx context.Context, filePath string, opts PreviewOptions) PreviewResult {
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" {
		// Try to detect from content
		data, err := os.ReadFile(filePath)
		if err == nil && len(data) > 0 {
			mimeType = http.DetectContentType(data)
		}
	}

	// Check if it's an image
	if strings.HasPrefix(mimeType, "image/") {
		return g.generateImagePreview(filePath, opts)
	}

	// Check if it's a PDF
	if mimeType == "application/pdf" {
		return g.generatePDFPreview(filePath, opts)
	}

	// Check if it's a text/code file
	if strings.HasPrefix(mimeType, "text/") ||
		strings.Contains(mimeType, "json") ||
		strings.Contains(mimeType, "xml") ||
		strings.Contains(mimeType, "yaml") ||
		strings.Contains(mimeType, "javascript") ||
		strings.Contains(mimeType, "typescript") {
		return g.generateTextPreview(filePath, opts)
	}

	// Check if it's markdown
	if strings.HasSuffix(strings.ToLower(filePath), ".md") ||
		strings.HasSuffix(strings.ToLower(filePath), ".markdown") {
		return g.generateMarkdownPreview(filePath, opts)
	}

	// Default: return file info
	return PreviewResult{
		Data:     []byte(fmt.Sprintf("Preview not available for %s (%s)", filepath.Base(filePath), mimeType)),
		MimeType: "text/plain",
		Error:    "Unsupported file type for preview",
	}
}

// generateImagePreview generates a preview for image files.
func (g *PreviewGenerator) generateImagePreview(filePath string, opts PreviewOptions) PreviewResult {
	img, err := imaging.Open(filePath)
	if err != nil {
		return PreviewResult{
			Error:    fmt.Sprintf("Failed to open image: %v", err),
			MimeType: "text/plain",
		}
	}

	// Resize if needed
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width > opts.MaxWidth || height > opts.MaxHeight {
		img = imaging.Fit(img, opts.MaxWidth, opts.MaxHeight, imaging.Lanczos)
		width = img.Bounds().Dx()
		height = img.Bounds().Dy()
	}

	// Encode to PNG for preview
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return PreviewResult{
			Error:    fmt.Sprintf("Failed to encode preview: %v", err),
			MimeType: "text/plain",
		}
	}

	return PreviewResult{
		Data:     buf.Bytes(),
		MimeType: "image/png",
		Width:    width,
		Height:   height,
	}
}

// generatePDFPreview generates a preview for PDF files.
// Note: This is a placeholder - real implementation would use a PDF library.
func (g *PreviewGenerator) generatePDFPreview(filePath string, opts PreviewOptions) PreviewResult {
	// For now, return a placeholder
	// Real implementation would use github.com/pdfium-go or similar
	return PreviewResult{
		Data:     []byte("PDF preview requires additional dependencies (pdfium/go-pdfium)"),
		MimeType: "text/plain",
		Error:    "PDF preview not fully implemented - requires CGO/pdfium",
	}
}

// generateTextPreview generates a syntax-highlighted preview for text/code files.
func (g *PreviewGenerator) generateTextPreview(filePath string, opts PreviewOptions) PreviewResult {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return PreviewResult{
			Error:    fmt.Sprintf("Failed to read file: %v", err),
			MimeType: "text/plain",
		}
	}

	// Detect lexer
	lexer := chroma.Coalesce(lexers.Match(filePath))
	if lexer == nil {
		lexer = lexers.Fallback
	}

	// Tokenize
	iterator, err := lexer.Tokenise(nil, string(data))
	if err != nil {
		return PreviewResult{
			Data:     data,
			MimeType: "text/plain",
			Error:    fmt.Sprintf("Failed to tokenize: %v", err),
		}
	}

	// Format with syntax highlighting
	var buf bytes.Buffer
	if err := g.syntaxHighlighter.Format(&buf, g.syntaxStyle, iterator); err != nil {
		return PreviewResult{
			Data:     data,
			MimeType: "text/plain",
			Error:    fmt.Sprintf("Failed to format: %v", err),
		}
	}

	// Truncate if too large
	maxSize := 1024 * 1024 // 1MB
	if buf.Len() > maxSize {
		truncated := buf.Bytes()[:maxSize]
		return PreviewResult{
			Data:      truncated,
			MimeType:  "text/plain",
			Truncated: true,
		}
	}

	return PreviewResult{
		Data:     buf.Bytes(),
		MimeType: "text/plain",
	}
}

// generateMarkdownPreview generates an HTML preview for markdown files.
func (g *PreviewGenerator) generateMarkdownPreview(filePath string, opts PreviewOptions) PreviewResult {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return PreviewResult{
			Error:    fmt.Sprintf("Failed to read file: %v", err),
			MimeType: "text/plain",
		}
	}

	var buf bytes.Buffer
	if err := g.markdownParser.Convert(data, &buf); err != nil {
		return PreviewResult{
			Data:     data,
			MimeType: "text/plain",
			Error:    fmt.Sprintf("Failed to render markdown: %v", err),
		}
	}

	return PreviewResult{
		Data:     buf.Bytes(),
		MimeType: "text/html",
	}
}

// CanPreview returns true if the generator can preview the file.
func (g *PreviewGenerator) CanPreview(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))
	mimeType := mime.TypeByExtension(ext)

	// Images
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}

	// PDF
	if mimeType == "application/pdf" {
		return true
	}

	// Text/code
	if strings.HasPrefix(mimeType, "text/") {
		return true
	}

	// Specific extensions
	previewableExts := map[string]bool{
		".json": true, ".xml": true, ".yaml": true, ".yml": true,
		".toml": true, ".ini": true, ".conf": true, ".config": true,
		".js": true, ".ts": true, ".jsx": true, ".tsx": true,
		".py": true, ".go": true, ".rs": true, ".java": true,
		".cpp": true, ".c": true, ".h": true, ".hpp": true,
		".rb": true, ".php": true, ".swift": true, ".kt": true,
		".sh": true, ".bash": true, ".zsh": true, ".fish": true,
		".sql": true, ".html": true, ".css": true, ".scss": true,
		".md": true, ".markdown": true, ".rst": true, ".tex": true,
		".dockerfile": true, ".makefile": true, ".gradle": true,
	}

	if previewableExts[ext] {
		return true
	}

	// Check for common filenames without extensions
	previewableNames := map[string]bool{
		"dockerfile": true, "makefile": true, "rakefile": true,
		"readme": true, "license": true, "changelog": true,
		"contributing": true, "authors": true, "todo": true,
	}

	base := strings.ToLower(filepath.Base(filePath))
	if previewableNames[base] {
		return true
	}

	return false
}

// GetSupportedMimeTypes returns the list of supported MIME types.
func (g *PreviewGenerator) GetSupportedMimeTypes() []string {
	return []string{
		"image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff",
		"application/pdf",
		"text/plain", "text/markdown", "text/x-go", "text/x-python",
		"application/javascript", "application/json", "application/xml",
		"text/x-yaml", "text/x-toml", "text/x-shellscript",
	}
}
