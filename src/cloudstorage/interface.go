// Package cloudstorage provides a unified interface for cloud storage providers.
// This allows superfile to interact with various cloud storage backends
// (S3, Azure Blob, GCS, WebDAV) through a common API.
package cloudstorage

import (
	"context"
	"io"
	"time"
)

// FileInfo represents metadata about a file or directory in cloud storage.
type FileInfo struct {
	Name         string
	Path         string
	Size         int64
	ModTime      time.Time
	IsDir        bool
	ContentType  string
	ETag         string
	StorageClass string
	Metadata     map[string]string
}

// ListOptions configures directory listing behavior.
type ListOptions struct {
	Prefix    string
	Delimiter string
	MaxKeys   int
	Marker    string
	Recursive bool
}

// PutOptions configures file upload behavior.
type PutOptions struct {
	ContentType  string
	Metadata     map[string]string
	StorageClass string
	ACL          string
}

// CloudStorageProvider defines the interface that all cloud storage backends must implement.
type CloudStorageProvider interface {
	// Name returns the provider's unique identifier (e.g., "s3", "azure", "gcs", "webdav").
	Name() string

	// DisplayName returns a human-readable name for the provider.
	DisplayName() string

	// Init initializes the provider with configuration.
	// Config is a provider-specific configuration map.
	Init(ctx context.Context, config map[string]string) error

	// ValidateConfig checks if the configuration is valid.
	ValidateConfig(config map[string]string) error

	// List lists files and directories at the given path.
	List(ctx context.Context, path string, opts ListOptions) ([]FileInfo, error)

	// Stat returns metadata for a single file or directory.
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// Get downloads a file to the provided writer.
	Get(ctx context.Context, path string, w io.Writer) error

	// Put uploads data from the provided reader to the given path.
	Put(ctx context.Context, path string, r io.Reader, opts PutOptions) error

	// Delete removes a file or directory.
	Delete(ctx context.Context, path string) error

	// Mkdir creates a directory (if the provider supports it).
	Mkdir(ctx context.Context, path string) error

	// Copy copies a file within the same provider.
	Copy(ctx context.Context, srcPath, dstPath string) error

	// Move moves/renames a file within the same provider.
	Move(ctx context.Context, srcPath, dstPath string) error

	// PresignURL generates a presigned URL for temporary access.
	PresignURL(ctx context.Context, path string, expiry time.Duration) (string, error)

	// Capabilities returns the provider's capabilities.
	Capabilities() ProviderCapabilities
}

// ProviderCapabilities describes what features a provider supports.
type ProviderCapabilities struct {
	SupportsDirectories     bool
	SupportsPresignURLs     bool
	SupportsMultipart       bool
	SupportsVersioning      bool
	SupportsEncryption      bool
	SupportsMetadata        bool
	MaxFileSize             int64
	SupportedStorageClasses []string
}

// ProviderRegistry manages registered cloud storage providers.
type ProviderRegistry struct {
	providers map[string]CloudStorageProvider
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]CloudStorageProvider),
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(provider CloudStorageProvider) {
	r.providers[provider.Name()] = provider
}

// Get retrieves a provider by name.
func (r *ProviderRegistry) Get(name string) (CloudStorageProvider, bool) {
	provider, ok := r.providers[name]
	return provider, ok
}

// List returns all registered provider names.
func (r *ProviderRegistry) List() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Global registry instance
var defaultRegistry = NewProviderRegistry()

// RegisterProvider registers a provider globally.
func RegisterProvider(provider CloudStorageProvider) {
	defaultRegistry.Register(provider)
}

// GetProvider retrieves a provider globally.
func GetProvider(name string) (CloudStorageProvider, bool) {
	return defaultRegistry.Get(name)
}

// ListProviders lists all registered providers globally.
func ListProviders() []string {
	return defaultRegistry.List()
}
