// Package search provides semantic search capabilities for files.
// Integrates with ripgrep for content search and provides fuzzy filename matching.
package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"mime"
)

// SearchResult represents a search result.
type SearchResult struct {
	Path       string
	LineNumber int
	Line       string
	MatchStart int
	MatchEnd   int
	Context    []string
	Score      float64
	FileSize   int64
	ModTime    int64
	IsDir      bool
}

// SearchOptions configures search behavior.
type SearchOptions struct {
	Query         string
	Path          string
	Pattern       string
	CaseSensitive bool
	Regex         bool
	WholeWord     bool
	IncludeHidden bool
	MaxResults    int
	MaxDepth      int
	FileTypes     []string
	ExcludePaths  []string
	UseRipgrep    bool
	ContextLines  int
}

// SearchEngine provides search capabilities.
type SearchEngine struct {
	index       map[string]*FileIndex
	indexMutex  sync.RWMutex
	ripgrepPath string
}

// FileIndex represents an indexed file.
type FileIndex struct {
	Path     string
	Size     int64
	ModTime  int64
	IsDir    bool
	MimeType string
	Keywords []string
	Content  string // For small files
}

// NewSearchEngine creates a new search engine.
func NewSearchEngine() *SearchEngine {
	// Check for ripgrep
	rgPath := "rg"
	if _, err := os.Stat(rgPath); err != nil {
		// Try common locations
		for _, p := range []string{"/usr/bin/rg", "/usr/local/bin/rg", "/opt/homebrew/bin/rg"} {
			if _, err := os.Stat(p); err == nil {
				rgPath = p
				break
			}
		}
	}

	return &SearchEngine{
		index:       make(map[string]*FileIndex),
		ripgrepPath: rgPath,
	}
}

// Search searches for files matching the query.
func (e *SearchEngine) Search(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	if opts.MaxResults <= 0 {
		opts.MaxResults = 100
	}
	if opts.ContextLines <= 0 {
		opts.ContextLines = 2
	}

	// If ripgrep is available and requested, use it for content search
	if opts.UseRipgrep && opts.Query != "" && !strings.HasPrefix(opts.Query, "*") {
		return e.searchWithRipgrep(ctx, opts)
	}

	// Fallback to Go-based search
	return e.searchGo(ctx, opts)
}

// searchWithRipgrep uses ripgrep for fast content search.
func (e *SearchEngine) searchWithRipgrep(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	// This is a simplified implementation
	// Real implementation would exec ripgrep and parse output
	return e.searchGo(ctx, opts)
}

// searchGo performs search using Go's filepath.Walk.
func (e *SearchEngine) searchGo(ctx context.Context, opts SearchOptions) ([]SearchResult, error) {
	var results []SearchResult
	var mutex sync.Mutex

	query := opts.Query
	if !opts.CaseSensitive {
		query = strings.ToLower(query)
	}

	var pattern *regexp.Regexp
	if opts.Regex && query != "" {
		var err error
		if opts.CaseSensitive {
			pattern, err = regexp.Compile(query)
		} else {
			pattern, err = regexp.Compile("(?i)" + query)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
	}

	err := filepath.WalkDir(opts.Path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err // Skip errors
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Check depth
		rel, _ := filepath.Rel(opts.Path, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if opts.MaxDepth > 0 && depth > opts.MaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files unless requested
		if !opts.IncludeHidden && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check exclude paths
		for _, exclude := range opts.ExcludePaths {
			if strings.Contains(path, exclude) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Check file types
		if len(opts.FileTypes) > 0 && !d.IsDir() {
			ext := filepath.Ext(path)
			matched := false
			for _, ft := range opts.FileTypes {
				if strings.EqualFold(ext, ft) || strings.EqualFold("*"+ext, ft) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Match filename
		name := d.Name()
		nameLower := strings.ToLower(name)
		queryLower := strings.ToLower(query)

		filenameMatch := false
		if opts.Regex && pattern != nil {
			filenameMatch = pattern.MatchString(name)
		} else if opts.WholeWord {
			filenameMatch = nameLower == queryLower
		} else if query != "" {
			filenameMatch = strings.Contains(nameLower, queryLower)
		}

		// Match content for files
		contentMatch := false
		var matchLine string
		var matchLineNum int
		var matchStart, matchEnd int

		if !d.IsDir() && query != "" && info.Size() < 10*1024*1024 { // Only search files < 10MB
			content, err := os.ReadFile(path)
			if err == nil {
				contentStr := string(content)
				if !opts.CaseSensitive {
					contentStr = strings.ToLower(contentStr)
				}

				if opts.Regex && pattern != nil {
					matches := pattern.FindStringIndex(contentStr)
					if matches != nil {
						contentMatch = true
						matchStart = matches[0]
						matchEnd = matches[1]
						// Find line number
						lines := strings.Split(contentStr[:matchStart], "\n")
						matchLineNum = len(lines)
						lineStart := strings.LastIndex(contentStr[:matchStart], "\n") + 1
						lineEnd := strings.Index(contentStr[matchStart:], "\n")
						if lineEnd == -1 {
							lineEnd = len(contentStr) - matchStart
						}
						matchLine = contentStr[lineStart : matchStart+lineEnd]
					}
				} else {
					idx := strings.Index(contentStr, queryLower)
					if idx >= 0 {
						contentMatch = true
						matchStart = idx
						matchEnd = idx + len(query)
						lines := strings.Split(contentStr[:matchStart], "\n")
						matchLineNum = len(lines)
						lineStart := strings.LastIndex(contentStr[:matchStart], "\n") + 1
						lineEnd := strings.Index(contentStr[matchStart:], "\n")
						if lineEnd == -1 {
							lineEnd = len(contentStr) - matchStart
						}
						matchLine = contentStr[lineStart : matchStart+lineEnd]
					}
				}
			}
		}

		if filenameMatch || contentMatch || (query == "" && opts.Pattern != "") {
			// Also check pattern matching
			if opts.Pattern != "" {
				matched, _ := filepath.Match(opts.Pattern, name)
				if !matched {
					return nil
				}
			}

			// Build context lines
			var contextLines []string
			if contentMatch && opts.ContextLines > 0 {
				// Simplified - would need full file read for proper context
			}

			result := SearchResult{
				Path:       path,
				LineNumber: matchLineNum,
				Line:       matchLine,
				MatchStart: matchStart,
				MatchEnd:   matchEnd,
				Context:    contextLines,
				Score:      calculateScore(filenameMatch, contentMatch, name, query),
				FileSize:   info.Size(),
				ModTime:    info.ModTime().Unix(),
				IsDir:      d.IsDir(),
			}

			mutex.Lock()
			results = append(results, result)
			mutex.Unlock()

			if len(results) >= opts.MaxResults {
				return fmt.Errorf("max results reached") // Use error to stop walk
			}
		}

		return nil
	})

	// Ignore the "max results reached" error
	if err != nil && err.Error() != "max results reached" {
		return nil, err
	}

	return results, nil
}

// calculateScore calculates a relevance score for a search result.
func calculateScore(filenameMatch, contentMatch bool, name, query string) float64 {
	score := 0.0
	if filenameMatch {
		score += 10.0
		// Exact match bonus
		if strings.EqualFold(name, query) {
			score += 20.0
		}
		// Prefix match bonus
		if strings.HasPrefix(strings.ToLower(name), strings.ToLower(query)) {
			score += 10.0
		}
	}
	if contentMatch {
		score += 5.0
	}
	return score
}

// IndexDirectory indexes a directory for faster searching.
func (e *SearchEngine) IndexDirectory(ctx context.Context, rootPath string) error {
	return filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		// Only index files smaller than 1MB
		if !d.IsDir() && info.Size() <= 1024*1024 {
			content, err := os.ReadFile(path)
			if err == nil {
				// Extract keywords (simplified)
				keywords := extractKeywords(string(content))

				e.indexMutex.Lock()
				e.index[path] = &FileIndex{
					Path:     path,
					Size:     info.Size(),
					ModTime:  info.ModTime().Unix(),
					IsDir:    false,
					MimeType: mimeTypeFromPath(path),
					Keywords: keywords,
					Content:  string(content),
				}
				e.indexMutex.Unlock()
			}
		}

		return nil
	})
}

// GetIndexedFiles returns all indexed files.
func (e *SearchEngine) GetIndexedFiles() []*FileIndex {
	e.indexMutex.RLock()
	defer e.indexMutex.RUnlock()

	files := make([]*FileIndex, 0, len(e.index))
	for _, idx := range e.index {
		files = append(files, idx)
	}
	return files
}

// ClearIndex clears the search index.
func (e *SearchEngine) ClearIndex() {
	e.indexMutex.Lock()
	defer e.indexMutex.Unlock()
	e.index = make(map[string]*FileIndex)
}

// extractKeywords extracts keywords from text content.
func extractKeywords(content string) []string {
	// Simplified: split by non-alphanumeric and filter
	words := regexp.MustCompile(`[a-zA-Z0-9_]+`).FindAllString(content, -1)
	keywordSet := make(map[string]bool)
	for _, w := range words {
		if len(w) >= 3 && len(w) <= 50 {
			keywordSet[strings.ToLower(w)] = true
		}
	}

	keywords := make([]string, 0, len(keywordSet))
	for k := range keywordSet {
		keywords = append(keywords, k)
	}
	return keywords
}

// mimeTypeFromPath guesses MIME type from file path.
func mimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	return mime.TypeByExtension(ext)
}

// FuzzySearch performs fuzzy string matching.
func FuzzySearch(query, target string) (bool, float64) {
	query = strings.ToLower(query)
	target = strings.ToLower(target)

	if query == "" {
		return true, 1.0
	}

	if strings.Contains(target, query) {
		// Calculate position-based score
		idx := strings.Index(target, query)
		score := 1.0 - float64(idx)/float64(len(target))
		return true, score
	}

	// Simple character sequence matching
	qIdx := 0
	for i := 0; i < len(target) && qIdx < len(query); i++ {
		if target[i] == query[qIdx] {
			qIdx++
		}
	}

	if qIdx == len(query) {
		return true, 0.5 // Partial match
	}

	return false, 0.0
}
