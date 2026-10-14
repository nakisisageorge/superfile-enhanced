// Package categorize provides AI-powered file categorization capabilities.
// Uses rule-based classification with extensible design for future ML integration.
package categorize

import (
	"bufio"
	"context"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// FileCategory represents a file category with metadata.
type FileCategory struct {
	ID          string
	Name        string
	Description string
	Icon        string
	Color       string
	Priority    int // Higher priority categories are matched first
}

// CategorizationResult represents the result of categorizing a file.
type CategorizationResult struct {
	FilePath    string
	Category    FileCategory
	Confidence  float64
	MatchedRule string
	Metadata    map[string]string
	Timestamp   time.Time
}

// CategorizationRule defines a rule for categorizing files.
type CategorizationRule struct {
	ID         string
	Category   FileCategory
	Patterns   []string // Glob patterns
	Regexes    []*regexp.Regexp
	MIMETypes  []string
	Extensions []string
	Keywords   []string                     // For content-based matching
	CustomFunc func(string) (bool, float64) // Custom logic with confidence
	Priority   int
}

// Categorizer performs file categorization.
type Categorizer struct {
	rules      []CategorizationRule
	categories map[string]FileCategory
}

// NewCategorizer creates a new categorizer with default rules.
func NewCategorizer() *Categorizer {
	c := &Categorizer{
		categories: make(map[string]FileCategory),
	}
	c.registerDefaultCategories()
	c.registerDefaultRules()
	return c
}

// registerDefaultCategories defines the built-in categories.
func (c *Categorizer) registerDefaultCategories() {
	categories := []FileCategory{
		{ID: "documents", Name: "Documents", Description: "Text documents, PDFs, spreadsheets", Icon: "📄", Color: "#3b82f6", Priority: 10},
		{ID: "images", Name: "Images", Description: "Image files", Icon: "🖼️", Color: "#8b5cf6", Priority: 10},
		{ID: "videos", Name: "Videos", Description: "Video files", Icon: "🎬", Color: "#ec4899", Priority: 10},
		{ID: "audio", Name: "Audio", Description: "Audio files", Icon: "🎵", Color: "#f59e0b", Priority: 10},
		{ID: "archives", Name: "Archives", Description: "Compressed archives", Icon: "📦", Color: "#6b7280", Priority: 10},
		{ID: "code", Name: "Code", Description: "Source code files", Icon: "💻", Color: "#10b981", Priority: 15},
		{ID: "config", Name: "Configuration", Description: "Config files", Icon: "⚙️", Color: "#6366f1", Priority: 15},
		{ID: "data", Name: "Data", Description: "Data files (JSON, CSV, XML)", Icon: "📊", Color: "#06b6d4", Priority: 12},
		{ID: "scripts", Name: "Scripts", Description: "Executable scripts", Icon: "📜", Color: "#ef4444", Priority: 15},
		{ID: "mobile_money", Name: "Mobile Money", Description: "African mobile money receipts", Icon: "📱", Color: "#16a34a", Priority: 20},
		{ID: "finance", Name: "Finance", Description: "Financial documents", Icon: "💰", Color: "#84cc16", Priority: 12},
		{ID: "other", Name: "Other", Description: "Uncategorized files", Icon: "📁", Color: "#9ca3af", Priority: 1},
	}

	for _, cat := range categories {
		c.categories[cat.ID] = cat
	}
}

// registerDefaultRules defines the built-in categorization rules.
func (c *Categorizer) registerDefaultRules() {
	now := time.Now()
	c.rules = []CategorizationRule{
		// Mobile Money Receipts (African fintech focus)
		{
			ID:       "mpesa_receipt",
			Category: c.categories["mobile_money"],
			Patterns: []string{"*mpesa*", "*m-pesa*", "*m_pesa*"},
			Regexes: []*regexp.Regexp{
				regexp.MustCompile(`(?i)mpesa`),
				regexp.MustCompile(`(?i)m-pesa`),
				regexp.MustCompile(`(?i)M-PESA`),
			},
			Keywords: []string{"MPESA", "M-PESA", "Safaricom", "Till Number", "Paybill"},
			Priority: 100,
		},
		{
			ID:       "momo_receipt",
			Category: c.categories["mobile_money"],
			Patterns: []string{"*momo*", "*mo-mo*", "*mo_mo*"},
			Regexes: []*regexp.Regexp{
				regexp.MustCompile(`(?i)momo`),
				regexp.MustCompile(`(?i)mo-mo`),
				regexp.MustCompile(`(?i)MTN MoMo`),
			},
			Keywords: []string{"MoMo", "MTN MoMo", "Mobile Money", "Reference:"},
			Priority: 100,
		},
		{
			ID:       "airtel_money_receipt",
			Category: c.categories["mobile_money"],
			Patterns: []string{"*airtel*", "*airtel_money*"},
			Regexes: []*regexp.Regexp{
				regexp.MustCompile(`(?i)airtel\s*money`),
				regexp.MustCompile(`(?i)airtel`),
			},
			Keywords: []string{"Airtel Money", "Airtel", "Transaction ID"},
			Priority: 100,
		},
		{
			ID:       "orange_money_receipt",
			Category: c.categories["mobile_money"],
			Patterns: []string{"*orange*", "*orange_money*"},
			Regexes: []*regexp.Regexp{
				regexp.MustCompile(`(?i)orange\s*money`),
				regexp.MustCompile(`(?i)orange`),
			},
			Keywords: []string{"Orange Money", "OM", "Transaction"},
			Priority: 100,
		},

		// Images
		{
			ID:         "image_files",
			Category:   c.categories["images"],
			MIMETypes:  []string{"image/jpeg", "image/png", "image/gif", "image/webp", "image/bmp", "image/tiff", "image/svg+xml", "image/heic", "image/avif"},
			Extensions: []string{".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tiff", ".tif", ".svg", ".heic", ".avif", ".ico"},
			Priority:   90,
		},

		// Videos
		{
			ID:         "video_files",
			Category:   c.categories["videos"],
			MIMETypes:  []string{"video/mp4", "video/webm", "video/quicktime", "video/x-msvideo", "video/x-matroska", "video/mpeg"},
			Extensions: []string{".mp4", ".webm", ".mov", ".avi", ".mkv", ".mpeg", ".mpg", ".flv", ".wmv", ".m4v"},
			Priority:   90,
		},

		// Audio
		{
			ID:         "audio_files",
			Category:   c.categories["audio"],
			MIMETypes:  []string{"audio/mpeg", "audio/wav", "audio/ogg", "audio/flac", "audio/aac", "audio/mp4"},
			Extensions: []string{".mp3", ".wav", ".ogg", ".flac", ".aac", ".m4a", ".opus", ".wma"},
			Priority:   90,
		},

		// Archives
		{
			ID:         "archive_files",
			Category:   c.categories["archives"],
			MIMETypes:  []string{"application/zip", "application/x-tar", "application/gzip", "application/x-rar", "application/x-7z-compressed"},
			Extensions: []string{".zip", ".tar", ".gz", ".tgz", ".rar", ".7z", ".bz2", ".xz", ".zst"},
			Priority:   90,
		},

		// Code files
		{
			ID:         "code_go",
			Category:   c.categories["code"],
			Extensions: []string{".go"},
			MIMETypes:  []string{"text/x-go"},
			Priority:   95,
		},
		{
			ID:         "code_python",
			Category:   c.categories["code"],
			Extensions: []string{".py", ".pyw", ".pyi"},
			MIMETypes:  []string{"text/x-python"},
			Priority:   95,
		},
		{
			ID:         "code_js_ts",
			Category:   c.categories["code"],
			Extensions: []string{".js", ".ts", ".jsx", ".tsx", ".mjs", ".cjs"},
			MIMETypes:  []string{"application/javascript", "text/typescript"},
			Priority:   95,
		},
		{
			ID:         "code_rust",
			Category:   c.categories["code"],
			Extensions: []string{".rs"},
			MIMETypes:  []string{"text/x-rust"},
			Priority:   95,
		},
		{
			ID:         "code_generic",
			Category:   c.categories["code"],
			Extensions: []string{".java", ".kt", ".scala", ".cs", ".cpp", ".cc", ".cxx", ".hpp", ".h", ".c", ".rb", ".php", ".swift", ".dart", ".lua", ".r", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".sql"},
			Priority:   90,
		},

		// Config files
		{
			ID:         "config_files",
			Category:   c.categories["config"],
			Patterns:   []string{"*.conf", "*.config", "*.ini", "*.toml", "*.yaml", "*.yml", "*.json", "*.xml", ".env*", "*.properties"},
			Extensions: []string{".conf", ".config", ".ini", ".toml", ".yaml", ".yml", ".json", ".xml", ".properties"},
			Priority:   95,
		},

		// Data files
		{
			ID:         "data_json",
			Category:   c.categories["data"],
			Extensions: []string{".json", ".jsonl", ".ndjson"},
			MIMETypes:  []string{"application/json"},
			Priority:   90,
		},
		{
			ID:         "data_csv",
			Category:   c.categories["data"],
			Extensions: []string{".csv", ".tsv"},
			MIMETypes:  []string{"text/csv", "text/tab-separated-values"},
			Priority:   90,
		},
		{
			ID:         "data_xml",
			Category:   c.categories["data"],
			Extensions: []string{".xml", ".xsd", ".xsl"},
			MIMETypes:  []string{"application/xml", "text/xml"},
			Priority:   90,
		},
		{
			ID:         "data_parquet",
			Category:   c.categories["data"],
			Extensions: []string{".parquet", ".pq"},
			MIMETypes:  []string{"application/parquet"},
			Priority:   90,
		},

		// Scripts
		{
			ID:         "shell_scripts",
			Category:   c.categories["scripts"],
			Extensions: []string{".sh", ".bash", ".zsh", ".fish"},
			MIMETypes:  []string{"application/x-shellscript"},
			Priority:   95,
		},
		{
			ID:       "python_scripts",
			Category: c.categories["scripts"],
			Patterns: []string{"*.py"},
			CustomFunc: func(path string) (bool, float64) {
				// Confirm a Python script by reading its first line for a shebang
				// (a .py extension alone is already covered by the pattern above).
				f, err := os.Open(path)
				if err != nil {
					return false, 0
				}
				defer f.Close()
				scanner := bufio.NewScanner(f)
				if !scanner.Scan() {
					return false, 0
				}
				firstLine := strings.ToLower(strings.TrimSpace(scanner.Text()))
				if strings.HasPrefix(firstLine, "#!") && strings.Contains(firstLine, "python") {
					return true, 0.95
				}
				return false, 0
			},
			Priority: 95,
		},

		// Documents
		{
			ID:         "doc_pdf",
			Category:   c.categories["documents"],
			Extensions: []string{".pdf"},
			MIMETypes:  []string{"application/pdf"},
			Priority:   95,
		},
		{
			ID:         "doc_office",
			Category:   c.categories["documents"],
			Extensions: []string{".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods", ".odp"},
			MIMETypes: []string{
				"application/msword",
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.ms-excel",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"application/vnd.ms-powerpoint",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			},
			Priority: 95,
		},
		{
			ID:         "doc_text",
			Category:   c.categories["documents"],
			Extensions: []string{".txt", ".md", ".markdown", ".rst", ".tex", ".rtf"},
			MIMETypes:  []string{"text/plain", "text/markdown", "text/x-rst"},
			Priority:   85,
		},

		// Finance
		{
			ID:       "finance_receipts",
			Category: c.categories["finance"],
			Patterns: []string{"*receipt*", "*invoice*", "*bill*", "*payment*"},
			Regexes: []*regexp.Regexp{
				regexp.MustCompile(`(?i)receipt`),
				regexp.MustCompile(`(?i)invoice`),
				regexp.MustCompile(`(?i)payment`),
			},
			Keywords: []string{"Total", "Amount", "VAT", "Tax", "Subtotal", "Invoice #", "Receipt #"},
			Priority: 80,
		},
	}

	_ = now // Suppress unused warning
}

// AddRule adds a custom categorization rule.
func (c *Categorizer) AddRule(rule CategorizationRule) {
	c.rules = append(c.rules, rule)
}

// AddCategory adds a custom category.
func (c *Categorizer) AddCategory(category FileCategory) {
	c.categories[category.ID] = category
}

// Categorize categorizes a single file based on path and optional content.
func (c *Categorizer) Categorize(ctx context.Context, filePath string, content []byte) CategorizationResult {
	// Determine MIME type
	mimeType := mime.TypeByExtension(filepath.Ext(filePath))
	if mimeType == "" && len(content) > 0 {
		mimeType = http.DetectContentType(content)
	}

	// Get filename
	fileName := filepath.Base(filePath)
	fileNameLower := strings.ToLower(fileName)

	bestMatch := CategorizationResult{
		FilePath:    filePath,
		Category:    c.categories["other"],
		Confidence:  0.1,
		MatchedRule: "default",
		Metadata:    make(map[string]string),
		Timestamp:   time.Now(),
	}

	// Sort rules by priority (higher first)
	sortedRules := make([]CategorizationRule, len(c.rules))
	copy(sortedRules, c.rules)
	// Simple bubble sort by priority
	for i := 0; i < len(sortedRules)-1; i++ {
		for j := 0; j < len(sortedRules)-i-1; j++ {
			if sortedRules[j].Priority < sortedRules[j+1].Priority {
				sortedRules[j], sortedRules[j+1] = sortedRules[j+1], sortedRules[j]
			}
		}
	}

	// Rule priority is the primary signal: a higher-priority rule wins even when a
	// lower-priority one scores a higher raw confidence (e.g. a MIME-type hit at
	// 0.95 must not outrank a config rule evaluated first). Confidence only breaks
	// ties between rules of the same priority.
	bestRule := -1
	bestConf := 0.0
	for idx, rule := range sortedRules {
		confidence := c.matchRule(rule, filePath, fileName, fileNameLower, mimeType, content)
		if confidence <= 0 {
			continue
		}
		if bestRule == -1 {
			bestRule, bestConf = idx, confidence
		} else if rule.Priority == sortedRules[bestRule].Priority && confidence > bestConf {
			bestRule, bestConf = idx, confidence
		}
		// Very high confidence within the top matching priority band lets us stop.
		if bestRule >= 0 && rule.Priority == sortedRules[bestRule].Priority && confidence >= 0.95 {
			break
		}
	}

	if bestRule >= 0 {
		bestMatch = CategorizationResult{
			FilePath:    filePath,
			Category:    sortedRules[bestRule].Category,
			Confidence:  bestConf,
			MatchedRule: sortedRules[bestRule].ID,
			Metadata:    make(map[string]string),
			Timestamp:   time.Now(),
		}
	}

	return bestMatch
}

// matchRule evaluates a rule against a file.
func (c *Categorizer) matchRule(rule CategorizationRule, filePath, fileName, fileNameLower, mimeType string, content []byte) float64 {
	maxConfidence := 0.0

	// Check extensions
	for _, ext := range rule.Extensions {
		if strings.HasSuffix(fileNameLower, strings.ToLower(ext)) {
			maxConfidence = max(maxConfidence, 0.9)
		}
	}

	// Check MIME types
	for _, mt := range rule.MIMETypes {
		if strings.EqualFold(mimeType, mt) {
			maxConfidence = max(maxConfidence, 0.95)
		}
	}

	// Check glob patterns
	for _, pattern := range rule.Patterns {
		matched, _ := filepath.Match(pattern, fileName)
		if !matched {
			matched, _ = filepath.Match(pattern, fileNameLower)
		}
		if matched {
			maxConfidence = max(maxConfidence, 0.85)
		}
	}

	// Check regexes
	for _, re := range rule.Regexes {
		if re.MatchString(fileName) || re.MatchString(fileNameLower) {
			maxConfidence = max(maxConfidence, 0.9)
		}
	}

	// Check keywords in content
	if len(content) > 0 && len(rule.Keywords) > 0 {
		contentStr := string(content)
		contentLower := strings.ToLower(contentStr)
		matchedKeywords := 0
		for _, keyword := range rule.Keywords {
			if strings.Contains(contentLower, strings.ToLower(keyword)) {
				matchedKeywords++
			}
		}
		if matchedKeywords > 0 {
			keywordConfidence := 0.5 + (0.4 * float64(matchedKeywords) / float64(len(rule.Keywords)))
			maxConfidence = max(maxConfidence, keywordConfidence)
		}
	}

	// Check custom function
	if rule.CustomFunc != nil {
		matched, customConf := rule.CustomFunc(filePath)
		if matched {
			maxConfidence = max(maxConfidence, customConf)
		}
	}

	return maxConfidence
}

// CategorizeBatch categorizes multiple files.
func (c *Categorizer) CategorizeBatch(ctx context.Context, files []string, contents map[string][]byte) []CategorizationResult {
	results := make([]CategorizationResult, len(files))
	for i, file := range files {
		var content []byte
		if contents != nil {
			content = contents[file]
		}
		results[i] = c.Categorize(ctx, file, content)
	}
	return results
}

// GetCategoryStats returns statistics about categorization results.
func (c *Categorizer) GetCategoryStats(results []CategorizationResult) map[string]int {
	stats := make(map[string]int)
	for _, r := range results {
		stats[r.Category.ID]++
	}
	return stats
}

// GetCategory returns a category by ID.
func (c *Categorizer) GetCategory(id string) (FileCategory, bool) {
	cat, ok := c.categories[id]
	return cat, ok
}

// ListCategories returns all registered categories.
func (c *Categorizer) ListCategories() []FileCategory {
	cats := make([]FileCategory, 0, len(c.categories))
	for _, cat := range c.categories {
		cats = append(cats, cat)
	}
	return cats
}

// ListRules returns all registered rules.
func (c *Categorizer) ListRules() []CategorizationRule {
	return c.rules
}
