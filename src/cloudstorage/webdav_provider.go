// Package webdav implements CloudStorageProvider for WebDAV servers.
// Supports Nextcloud, OwnCloud, and other WebDAV-compatible servers.
package cloudstorage

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/studio-b12/gowebdav"
)

// WebDAVConfig holds configuration for WebDAV provider.
type WebDAVConfig struct {
	URL      string `json:"url" toml:"url"`
	Username string `json:"username" toml:"username"`
	Password string `json:"password" toml:"password"`
	Prefix   string `json:"prefix" toml:"prefix"`
	Insecure bool   `json:"insecure" toml:"insecure"` // Skip TLS verification
}

// WebDAVProvider implements CloudStorageProvider for WebDAV.
type WebDAVProvider struct {
	client *gowebdav.Client
	prefix string
}

// NewWebDAVProvider creates a new WebDAV provider.
func NewWebDAVProvider() *WebDAVProvider {
	return &WebDAVProvider{}
}

func (p *WebDAVProvider) Name() string {
	return "webdav"
}

func (p *WebDAVProvider) DisplayName() string {
	return "WebDAV (Nextcloud, OwnCloud, etc.)"
}

func (p *WebDAVProvider) Init(ctx context.Context, configMap map[string]string) error {
	cfg := WebDAVConfig{}
	for k, v := range configMap {
		switch k {
		case "url":
			cfg.URL = v
		case "username":
			cfg.Username = v
		case "password":
			cfg.Password = v
		case "prefix":
			cfg.Prefix = v
		case "insecure":
			cfg.Insecure = v == "true"
		}
	}

	if cfg.URL == "" {
		return fmt.Errorf("url is required")
	}
	if cfg.Username == "" {
		return fmt.Errorf("username is required")
	}
	if cfg.Password == "" {
		return fmt.Errorf("password is required")
	}

	p.prefix = strings.Trim(cfg.Prefix, "/")

	// Create WebDAV client
	client := gowebdav.NewClient(cfg.URL, cfg.Username, cfg.Password)
	if cfg.Insecure {
		// Opt-in only: skips TLS certificate verification for self-hosted servers
		// with internal CAs. Never enable for public endpoints.
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit user opt-in
		client.SetTransport(transport)
	}

	// Test connection
	if err := client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to WebDAV server: %w", err)
	}

	p.client = client
	return nil
}

func (p *WebDAVProvider) ValidateConfig(configMap map[string]string) error {
	if _, ok := configMap["url"]; !ok {
		return fmt.Errorf("url is required")
	}
	if _, ok := configMap["username"]; !ok {
		return fmt.Errorf("username is required")
	}
	if _, ok := configMap["password"]; !ok {
		return fmt.Errorf("password is required")
	}
	return nil
}

func (p *WebDAVProvider) fullPath(pth string) string {
	pth = strings.TrimPrefix(pth, "/")
	if p.prefix == "" {
		return pth
	}
	return path.Join(p.prefix, pth)
}

func (p *WebDAVProvider) stripPrefix(key string) string {
	if p.prefix == "" {
		return key
	}
	prefix := p.prefix + "/"
	if strings.HasPrefix(key, prefix) {
		return strings.TrimPrefix(key, prefix)
	}
	if key == p.prefix {
		return ""
	}
	return key
}

func (p *WebDAVProvider) toFileInfo(info os.FileInfo, fullPath string) *FileInfo {
	name := info.Name()
	if name == "." || name == "/" {
		name = path.Base(fullPath)
	}

	return &FileInfo{
		Name:    name,
		Path:    p.stripPrefix(fullPath),
		Size:    info.Size(),
		ModTime: info.ModTime(),
		IsDir:   info.IsDir(),
	}
}

func (p *WebDAVProvider) List(ctx context.Context, pth string, opts ListOptions) ([]FileInfo, error) {
	fullPath := p.fullPath(pth)
	if fullPath == "" {
		fullPath = "/"
	}

	files, err := p.client.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	var result []FileInfo
	for _, f := range files {
		entry := path.Join(fullPath, f.Name())
		if f.IsDir() {
			entry += "/"
		}
		result = append(result, *p.toFileInfo(f, entry))
	}

	return result, nil
}

func (p *WebDAVProvider) Stat(ctx context.Context, pth string) (*FileInfo, error) {
	fullPath := p.fullPath(pth)

	info, err := p.client.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat: %w", err)
	}

	return p.toFileInfo(info, fullPath), nil
}

func (p *WebDAVProvider) Get(ctx context.Context, pth string, w io.Writer) error {
	fullPath := p.fullPath(pth)

	reader, err := p.client.ReadStream(fullPath)
	if err != nil {
		return fmt.Errorf("failed to read stream: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(w, reader)
	return err
}

func (p *WebDAVProvider) Put(ctx context.Context, pth string, r io.Reader, opts PutOptions) error {
	fullPath := p.fullPath(pth)

	// Create parent directories if needed
	dir := path.Dir(fullPath)
	if dir != "/" && dir != "." {
		if err := p.client.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create parent directories: %w", err)
		}
	}

	return p.client.WriteStream(fullPath, r, 0644)
}

func (p *WebDAVProvider) Delete(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)

	info, err := p.client.Stat(fullPath)
	if err != nil {
		return fmt.Errorf("failed to stat for deletion: %w", err)
	}

	if info.IsDir() {
		return p.client.RemoveAll(fullPath)
	}
	return p.client.Remove(fullPath)
}

func (p *WebDAVProvider) Mkdir(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)
	return p.client.MkdirAll(fullPath, 0755)
}

func (p *WebDAVProvider) Copy(ctx context.Context, srcPath, dstPath string) error {
	srcFull := p.fullPath(srcPath)
	dstFull := p.fullPath(dstPath)

	// WebDAV doesn't have native copy, so download and re-upload
	reader, err := p.client.ReadStream(srcFull)
	if err != nil {
		return fmt.Errorf("failed to read source: %w", err)
	}
	defer reader.Close()

	return p.client.WriteStream(dstFull, reader, 0644)
}

func (p *WebDAVProvider) Move(ctx context.Context, srcPath, dstPath string) error {
	srcFull := p.fullPath(srcPath)
	dstFull := p.fullPath(dstPath)

	// WebDAV MOVE method
	return p.client.Rename(srcFull, dstFull, true)
}

func (p *WebDAVProvider) PresignURL(ctx context.Context, pth string, expiry time.Duration) (string, error) {
	// WebDAV doesn't have standard presigned URLs
	// Some servers support token-based auth, but not standard
	return "", fmt.Errorf("presigned URLs not supported by WebDAV")
}

func (p *WebDAVProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsDirectories:     true,
		SupportsPresignURLs:     false,
		SupportsMultipart:       false,
		SupportsVersioning:      false,
		SupportsEncryption:      false,
		SupportsMetadata:        false,
		MaxFileSize:             0, // No specific limit
		SupportedStorageClasses: []string{},
	}
}

func init() {
	RegisterProvider(NewWebDAVProvider())
}
