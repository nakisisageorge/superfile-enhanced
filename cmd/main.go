// Package main demonstrates the superfile-enhanced capabilities.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	categorize "github.com/nakisisageorge/superfile-enhanced/src/categorize"
	cloudstorage "github.com/nakisisageorge/superfile-enhanced/src/cloudstorage"
	preview "github.com/nakisisageorge/superfile-enhanced/src/preview"
	search "github.com/nakisisageorge/superfile-enhanced/src/search"
)

func main() {
	ctx := context.Background()

	fmt.Println("=== superfile-enhanced Demo ===")
	fmt.Println()

	// 1. Cloud Storage Demo
	fmt.Println("1. Cloud Storage Providers:")
	providers := cloudstorage.ListProviders()
	for _, p := range providers {
		provider, _ := cloudstorage.GetProvider(p)
		caps := provider.Capabilities()
		fmt.Printf("  - %s (%s)\n", provider.DisplayName(), provider.Name())
		fmt.Printf("    Directories: %v, Presign: %v, Multipart: %v\n",
			caps.SupportsDirectories, caps.SupportsPresignURLs, caps.SupportsMultipart)
	}
	fmt.Println()

	// 2. File Categorization Demo
	fmt.Println("2. AI-Powered File Categorization:")
	cat := categorize.NewCategorizer()

	testFiles := []string{
		"receipt_mpesa_12345.txt",
		"transaction_momo_67890.pdf",
		"airtel_money_receipt.jpg",
		"orange_money_payment.png",
		"main.go",
		"config.yaml",
		"data.json",
		"script.sh",
		"image.png",
		"video.mp4",
		"archive.zip",
		"document.pdf",
		"spreadsheet.xlsx",
		"README.md",
		"Dockerfile",
		"invoice_2024.pdf",
		"random_file.xyz",
	}

	// Create test files with content for better categorization
	tmpDir, _ := os.MkdirTemp("", "superfile-cat-test-")
	defer os.RemoveAll(tmpDir)

	contents := map[string][]byte{
		"receipt_mpesa_12345.txt":    []byte("MPESA Transaction Confirmed\nAmount: KES 5000\nTill Number: 123456\nDate: 2024-01-15\nReference: ABC123"),
		"transaction_momo_67890.pdf": []byte("MTN MoMo Payment\nAmount: UGX 25000\nReference: TXN789\nMobile Money"),
		"main.go":                    []byte("package main\n\nfunc main() {\n\tprintln(\"Hello, World!\")\n}"),
		"config.yaml":                []byte("database:\n  host: localhost\n  port: 5432\n  name: mydb\n"),
		"data.json":                  []byte(`{"users": [{"id": 1, "name": "John"}]}`),
		"script.sh":                  []byte("#!/bin/bash\necho 'Hello from script'\n"),
		"invoice_2024.pdf":           []byte("Invoice #INV-2024-001\nTotal: $1,250.00\nVAT: $250.00\nPayment Due: 2024-02-15"),
	}

	// Write test files
	for name, content := range contents {
		os.WriteFile(filepath.Join(tmpDir, name), content, 0644)
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(tmpDir, file)
		content := contents[file]
		if content == nil {
			// Create empty file for testing
			os.WriteFile(fullPath, []byte(""), 0644)
			content = []byte("")
		}

		result := cat.Categorize(ctx, fullPath, content)
		fmt.Printf("  %-30s -> %-15s (%.0f%%) [%s]\n",
			file, result.Category.Name, result.Confidence*100, result.MatchedRule)
	}
	fmt.Println()

	// 3. Preview Demo
	fmt.Println("3. Enhanced File Previews:")
	previewGen := preview.NewPreviewGenerator(preview.PreviewOptions{
		MaxWidth:    800,
		MaxHeight:   600,
		SyntaxTheme: "dracula",
		LineNumbers: true,
	})

	previewFiles := []string{
		filepath.Join(tmpDir, "main.go"),
		filepath.Join(tmpDir, "config.yaml"),
		filepath.Join(tmpDir, "data.json"),
		filepath.Join(tmpDir, "README.md"),
	}

	for _, file := range previewFiles {
		if _, err := os.Stat(file); err == nil {
			result := previewGen.GeneratePreview(ctx, file, preview.PreviewOptions{
				MaxWidth:  800,
				MaxHeight: 600,
			})
			fmt.Printf("  %-20s -> %s (%d bytes, %s)\n",
				filepath.Base(file), result.MimeType, len(result.Data), result.Error)
		}
	}
	fmt.Println()

	// 4. Search Demo
	fmt.Println("4. Semantic Search:")
	searchEngine := search.NewSearchEngine()

	// Index the temp directory
	_ = searchEngine.IndexDirectory(ctx, tmpDir)

	results, _ := searchEngine.Search(ctx, search.SearchOptions{
		Query:         "MPESA",
		Path:          tmpDir,
		CaseSensitive: false,
		MaxResults:    10,
	})

	for _, r := range results {
		fmt.Printf("  %s (line %d): %s\n", r.Path, r.LineNumber, r.Line)
	}

	results, _ = searchEngine.Search(ctx, search.SearchOptions{
		Query:         "func",
		Path:          tmpDir,
		CaseSensitive: false,
		MaxResults:    10,
	})

	for _, r := range results {
		fmt.Printf("  %s (line %d): %s\n", r.Path, r.LineNumber, r.Line)
	}
	fmt.Println()

	// 5. Category Statistics
	fmt.Println("5. Category Statistics:")
	categories := cat.ListCategories()
	for _, c := range categories {
		fmt.Printf("  - %s (%s): %s\n", c.Name, c.ID, c.Description)
	}
	fmt.Println()

	fmt.Println("=== Demo Complete ===")
}
