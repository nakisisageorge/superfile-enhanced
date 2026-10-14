# superfile-enhanced Makefile

.PHONY: build test test-coverage lint fmt tidy clean install demo

# Build the demo application
build:
	go build -o bin/superfile-enhanced-demo ./cmd

# Run all tests
test:
	go test ./...

# Run tests with coverage
test-coverage:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run linter
lint:
	golangci-lint run

# Format code
fmt:
	go fmt ./...

# Tidy dependencies
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -rf bin/ coverage.out coverage.html

# Install demo binary
install: build
	cp bin/superfile-enhanced-demo $(GOPATH)/bin/superfile-enhanced-demo

# Run demo
demo: build
	./bin/superfile-enhanced-demo

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o bin/superfile-enhanced-demo-linux-amd64 ./cmd
	GOOS=darwin GOARCH=amd64 go build -o bin/superfile-enhanced-demo-darwin-amd64 ./cmd
	GOOS=darwin GOARCH=arm64 go build -o bin/superfile-enhanced-demo-darwin-arm64 ./cmd
	GOOS=windows GOARCH=amd64 go build -o bin/superfile-enhanced-demo-windows-amd64.exe ./cmd

# Generate documentation
docs:
	godoc -http=:6060

# Check for vulnerabilities
vuln:
	govulncheck ./...

# Run benchmarks
bench:
	go test -bench=. -benchmem ./...

# Default target
default: tidy fmt test build