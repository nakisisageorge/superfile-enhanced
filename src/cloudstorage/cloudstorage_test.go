// Package cloudstorage provides tests for cloud storage providers.
package cloudstorage

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestProviderRegistry(t *testing.T) {
	registry := NewProviderRegistry()

	// Register a mock provider
	mockProvider := &mockProvider{name: "mock"}
	registry.Register(mockProvider)

	// Test Get
	provider, ok := registry.Get("mock")
	if !ok {
		t.Error("Failed to get registered provider")
	}
	if provider.Name() != "mock" {
		t.Errorf("Expected provider name 'mock', got %s", provider.Name())
	}

	// Test Get non-existent
	_, ok = registry.Get("nonexistent")
	if ok {
		t.Error("Expected non-existent provider to not be found")
	}

	// Test List
	providers := registry.List()
	if len(providers) != 1 || providers[0] != "mock" {
		t.Errorf("Expected [mock], got %v", providers)
	}
}

func TestGlobalRegistry(t *testing.T) {
	// Test global functions
	RegisterProvider(&mockProvider{name: "global-mock"})

	provider, ok := GetProvider("global-mock")
	if !ok {
		t.Error("Failed to get globally registered provider")
	}
	if provider.Name() != "global-mock" {
		t.Errorf("Expected global-mock, got %s", provider.Name())
	}

	providers := ListProviders()
	found := false
	for _, p := range providers {
		if p == "global-mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("global-mock not found in global registry list")
	}
}

func TestS3ProviderInit(t *testing.T) {
	provider := NewS3Provider()

	// Test with missing required config
	err := provider.Init(context.Background(), map[string]string{})
	if err == nil {
		t.Error("Expected error for missing bucket")
	}

	err = provider.Init(context.Background(), map[string]string{
		"bucket": "test-bucket",
	})
	if err != nil {
		t.Errorf("Unexpected error with minimal config: %v", err)
	}

	// Test config validation
	err = provider.ValidateConfig(map[string]string{})
	if err == nil {
		t.Error("Expected validation error for empty config")
	}

	err = provider.ValidateConfig(map[string]string{
		"bucket": "test",
		"region": "us-east-1",
	})
	if err != nil {
		t.Errorf("Unexpected validation error: %v", err)
	}
}

func TestAzureProviderInit(t *testing.T) {
	provider := NewAzureBlobProvider()

	err := provider.Init(context.Background(), map[string]string{})
	if err == nil {
		t.Error("Expected error for missing account_name")
	}

	err = provider.Init(context.Background(), map[string]string{
		"account_name":   "testaccount",
		"container_name": "testcontainer",
	})
	// This will fail because we don't have real credentials, but should not fail on validation
	if err != nil && !isCredentialError(err) {
		t.Errorf("Unexpected error type: %v", err)
	}
}

func TestGCSProviderInit(t *testing.T) {
	provider := NewGCSProvider()

	err := provider.Init(context.Background(), map[string]string{})
	if err == nil {
		t.Error("Expected error for missing bucket")
	}

	err = provider.Init(context.Background(), map[string]string{
		"bucket":     "test-bucket",
		"project_id": "test-project",
	})
	// Will fail without credentials, but not validation error
	if err != nil && !isCredentialError(err) {
		t.Errorf("Unexpected error type: %v", err)
	}
}

func TestWebDAVProviderInit(t *testing.T) {
	provider := NewWebDAVProvider()

	err := provider.Init(context.Background(), map[string]string{})
	if err == nil {
		t.Error("Expected error for missing url")
	}

	err = provider.Init(context.Background(), map[string]string{
		"url":      "https://example.com/webdav",
		"username": "user",
		"password": "pass",
	})
	// Will fail to connect, but not validation error
	if err != nil && !isConnectionError(err) {
		t.Errorf("Unexpected error type: %v", err)
	}
}

// Helper to check if error is credential-related
func isCredentialError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "credential") || contains(errStr, "auth") || contains(errStr, "Unauthorized")
}

// Helper to check if error is connection-related
func isConnectionError(err error) bool {
	errStr := err.Error()
	return contains(errStr, "connect") || contains(errStr, "dial") || contains(errStr, "network") || contains(errStr, "timeout")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockProvider implements CloudStorageProvider for testing
type mockProvider struct {
	name string
}

func (m *mockProvider) Name() string                                             { return m.name }
func (m *mockProvider) DisplayName() string                                      { return "Mock Provider" }
func (m *mockProvider) Init(ctx context.Context, config map[string]string) error { return nil }
func (m *mockProvider) ValidateConfig(config map[string]string) error            { return nil }
func (m *mockProvider) List(ctx context.Context, path string, opts ListOptions) ([]FileInfo, error) {
	return nil, nil
}
func (m *mockProvider) Stat(ctx context.Context, path string) (*FileInfo, error) { return nil, nil }
func (m *mockProvider) Get(ctx context.Context, path string, w io.Writer) error  { return nil }
func (m *mockProvider) Put(ctx context.Context, path string, r io.Reader, opts PutOptions) error {
	return nil
}
func (m *mockProvider) Delete(ctx context.Context, path string) error   { return nil }
func (m *mockProvider) Mkdir(ctx context.Context, path string) error    { return nil }
func (m *mockProvider) Copy(ctx context.Context, src, dst string) error { return nil }
func (m *mockProvider) Move(ctx context.Context, src, dst string) error { return nil }
func (m *mockProvider) PresignURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	return "", nil
}
func (m *mockProvider) Capabilities() ProviderCapabilities { return ProviderCapabilities{} }
