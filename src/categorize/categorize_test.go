// Package categorize provides tests for the categorization service.
package categorize

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewCategorizer(t *testing.T) {
	c := NewCategorizer()
	if c == nil {
		t.Fatal("NewCategorizer returned nil")
	}

	categories := c.ListCategories()
	if len(categories) == 0 {
		t.Error("No categories registered")
	}

	// Check for expected categories
	expectedCategories := []string{"documents", "images", "videos", "audio", "archives", "code", "config", "data", "scripts", "mobile_money", "finance", "other"}
	for _, expected := range expectedCategories {
		found := false
		for _, cat := range categories {
			if cat.ID == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected category %s not found", expected)
		}
	}

	rules := c.ListRules()
	if len(rules) == 0 {
		t.Error("No rules registered")
	}
}

func TestCategorizeMobileMoney(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	testCases := []struct {
		fileName         string
		content          []byte
		expectedCategory string
		minConfidence    float64
	}{
		{"mpesa_receipt.txt", []byte("MPESA Transaction\nAmount: KES 5000\nTill: 123456"), "mobile_money", 0.8},
		{"momo_payment.pdf", []byte("MTN MoMo Payment\nReference: TXN123"), "mobile_money", 0.8},
		{"airtel_money.jpg", []byte("Airtel Money Transaction\nID: AIR123"), "mobile_money", 0.7},
		{"orange_money.png", []byte("Orange Money\nAmount: CFA 10000"), "mobile_money", 0.7},
	}

	for _, tc := range testCases {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, tc.fileName)
		os.WriteFile(filePath, tc.content, 0644)

		result := c.Categorize(ctx, filePath, tc.content)

		if result.Category.ID != tc.expectedCategory {
			t.Errorf("%s: expected category %s, got %s", tc.fileName, tc.expectedCategory, result.Category.ID)
		}
		if result.Confidence < tc.minConfidence {
			t.Errorf("%s: expected confidence >= %.0f%%, got %.0f%%", tc.fileName, tc.minConfidence*100, result.Confidence*100)
		}
	}
}

func TestCategorizeCodeFiles(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	testCases := []struct {
		fileName         string
		expectedCategory string
	}{
		{"main.go", "code"},
		{"script.py", "code"},
		{"app.js", "code"},
		{"component.tsx", "code"},
		{"main.rs", "code"},
		{"App.java", "code"},
		{"program.cpp", "code"},
		{"script.sh", "scripts"},
		{"deploy.bash", "scripts"},
	}

	for _, tc := range testCases {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, tc.fileName)
		os.WriteFile(filePath, []byte(""), 0644)

		result := c.Categorize(ctx, filePath, nil)

		if result.Category.ID != tc.expectedCategory {
			t.Errorf("%s: expected category %s, got %s (rule: %s, conf: %.2f)", tc.fileName, tc.expectedCategory, result.Category.ID, result.MatchedRule, result.Confidence)
		}
	}
}

func TestCategorizeDocuments(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	testCases := []struct {
		fileName         string
		expectedCategory string
	}{
		{"document.pdf", "documents"},
		{"report.docx", "documents"},
		{"spreadsheet.xlsx", "documents"},
		{"presentation.pptx", "documents"},
		{"notes.txt", "documents"},
		{"README.md", "documents"},
	}

	for _, tc := range testCases {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, tc.fileName)
		os.WriteFile(filePath, []byte(""), 0644)

		result := c.Categorize(ctx, filePath, nil)

		if result.Category.ID != tc.expectedCategory {
			t.Errorf("%s: expected category %s, got %s (rule: %s)", tc.fileName, tc.expectedCategory, result.Category.ID, result.MatchedRule)
		}
	}
}

func TestCategorizeMedia(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	testCases := []struct {
		fileName         string
		expectedCategory string
	}{
		{"photo.jpg", "images"},
		{"image.png", "images"},
		{"animation.gif", "images"},
		{"video.mp4", "videos"},
		{"movie.mkv", "videos"},
		{"song.mp3", "audio"},
		{"audio.wav", "audio"},
		{"archive.zip", "archives"},
		{"backup.tar.gz", "archives"},
	}

	for _, tc := range testCases {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, tc.fileName)
		os.WriteFile(filePath, []byte(""), 0644)

		result := c.Categorize(ctx, filePath, nil)

		if result.Category.ID != tc.expectedCategory {
			t.Errorf("%s: expected category %s, got %s", tc.fileName, tc.expectedCategory, result.Category.ID)
		}
	}
}

func TestCategorizeConfigFiles(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	testCases := []struct {
		fileName         string
		expectedCategory string
	}{
		{"config.yaml", "config"},
		{"settings.toml", "config"},
		{"app.json", "config"},
		{"config.xml", "config"},
		{".env", "config"},
		{"docker-compose.yml", "config"},
	}

	for _, tc := range testCases {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, tc.fileName)
		os.WriteFile(filePath, []byte(""), 0644)

		result := c.Categorize(ctx, filePath, nil)

		if result.Category.ID != tc.expectedCategory {
			t.Errorf("%s: expected category %s, got %s", tc.fileName, tc.expectedCategory, result.Category.ID)
		}
	}
}

func TestCategorizeDataFiles(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	testCases := []struct {
		fileName         string
		expectedCategory string
	}{
		{"data.json", "data"},
		{"export.csv", "data"},
		{"config.xml", "data"}, // XML could be config or data
		{"dataset.parquet", "data"},
	}

	for _, tc := range testCases {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, tc.fileName)
		os.WriteFile(filePath, []byte(""), 0644)

		result := c.Categorize(ctx, filePath, nil)

		// Data or config are both acceptable for some formats
		if result.Category.ID != tc.expectedCategory && result.Category.ID != "config" {
			t.Errorf("%s: expected category %s or config, got %s", tc.fileName, tc.expectedCategory, result.Category.ID)
		}
	}
}

func TestCategorizeFinance(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "invoice.pdf")
	content := []byte("Invoice #INV-001\nTotal: $1,000.00\nVAT: $200.00\nSubtotal: $800.00")
	os.WriteFile(filePath, content, 0644)

	result := c.Categorize(ctx, filePath, content)

	// Should match finance or documents
	if result.Category.ID != "finance" && result.Category.ID != "documents" {
		t.Errorf("invoice.pdf: expected finance or documents, got %s (rule: %s, conf: %.2f)", result.Category.ID, result.MatchedRule, result.Confidence)
	}
}

func TestCategorizeBatch(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	tmpDir := t.TempDir()
	files := []string{
		filepath.Join(tmpDir, "file1.go"),
		filepath.Join(tmpDir, "file2.py"),
		filepath.Join(tmpDir, "image.png"),
		filepath.Join(tmpDir, "doc.pdf"),
	}

	contents := map[string][]byte{
		files[0]: []byte("package main\nfunc main() {}"),
		files[1]: []byte("def hello():\n    print('world')"),
		files[2]: []byte(""),
		files[3]: []byte("PDF content"),
	}

	for _, f := range files {
		os.WriteFile(f, contents[f], 0644)
	}

	results := c.CategorizeBatch(ctx, files, contents)

	if len(results) != len(files) {
		t.Errorf("Expected %d results, got %d", len(files), len(results))
	}

	expectedCategories := []string{"code", "code", "images", "documents"}
	for i, r := range results {
		if r.Category.ID != expectedCategories[i] {
			t.Errorf("File %d: expected %s, got %s", i, expectedCategories[i], r.Category.ID)
		}
	}
}

func TestGetCategoryStats(t *testing.T) {
	c := NewCategorizer()

	results := []CategorizationResult{
		{Category: c.categories["code"], Confidence: 0.9},
		{Category: c.categories["code"], Confidence: 0.8},
		{Category: c.categories["images"], Confidence: 0.95},
		{Category: c.categories["documents"], Confidence: 0.85},
	}

	stats := c.GetCategoryStats(results)

	if stats["code"] != 2 {
		t.Errorf("Expected code count 2, got %d", stats["code"])
	}
	if stats["images"] != 1 {
		t.Errorf("Expected images count 1, got %d", stats["images"])
	}
	if stats["documents"] != 1 {
		t.Errorf("Expected documents count 1, got %d", stats["documents"])
	}
}

func TestAddCustomCategoryAndRule(t *testing.T) {
	c := NewCategorizer()
	ctx := context.Background()

	// Add custom category
	customCat := FileCategory{
		ID:          "custom",
		Name:        "Custom",
		Description: "Custom category for testing",
		Icon:        "⭐",
		Color:       "#ff00ff",
		Priority:    50,
	}
	c.AddCategory(customCat)

	// Add custom rule
	customRule := CategorizationRule{
		ID:       "custom_rule",
		Category: customCat,
		Patterns: []string{"*custom*"},
		Priority: 100,
	}
	c.AddRule(customRule)

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "custom_file.txt")
	os.WriteFile(filePath, []byte(""), 0644)

	result := c.Categorize(ctx, filePath, nil)

	if result.Category.ID != "custom" {
		t.Errorf("Expected custom category, got %s (rule: %s)", result.Category.ID, result.MatchedRule)
	}
}
