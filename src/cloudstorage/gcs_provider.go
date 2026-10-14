// Package gcs implements CloudStorageProvider for Google Cloud Storage.
package cloudstorage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GCSConfig holds configuration for GCS provider.
type GCSConfig struct {
	ProjectID   string `json:"project_id" toml:"project_id"`
	Bucket      string `json:"bucket" toml:"bucket"`
	Credentials string `json:"credentials" toml:"credentials"` // Path to service account JSON
	Prefix      string `json:"prefix" toml:"prefix"`
	Endpoint    string `json:"endpoint" toml:"endpoint"` // For emulators
}

// GCSProvider implements CloudStorageProvider for Google Cloud Storage.
type GCSProvider struct {
	client *storage.Client
	bucket *storage.BucketHandle
	prefix string
}

// NewGCSProvider creates a new GCS provider.
func NewGCSProvider() *GCSProvider {
	return &GCSProvider{}
}

func (p *GCSProvider) Name() string {
	return "gcs"
}

func (p *GCSProvider) DisplayName() string {
	return "Google Cloud Storage"
}

func (p *GCSProvider) Init(ctx context.Context, configMap map[string]string) error {
	cfg := GCSConfig{}
	for k, v := range configMap {
		switch k {
		case "project_id":
			cfg.ProjectID = v
		case "bucket":
			cfg.Bucket = v
		case "credentials":
			cfg.Credentials = v
		case "prefix":
			cfg.Prefix = v
		case "endpoint":
			cfg.Endpoint = v
		}
	}

	if cfg.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if cfg.ProjectID == "" {
		return fmt.Errorf("project_id is required")
	}

	p.prefix = strings.Trim(cfg.Prefix, "/")

	var opts []option.ClientOption
	if cfg.Credentials != "" {
		opts = append(opts, option.WithCredentialsFile(cfg.Credentials))
	}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithEndpoint(cfg.Endpoint))
		opts = append(opts, option.WithoutAuthentication()) // For emulators
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}

	p.client = client
	p.bucket = client.Bucket(cfg.Bucket)

	return nil
}

func (p *GCSProvider) ValidateConfig(configMap map[string]string) error {
	if _, ok := configMap["bucket"]; !ok {
		return fmt.Errorf("bucket is required")
	}
	if _, ok := configMap["project_id"]; !ok {
		return fmt.Errorf("project_id is required")
	}
	return nil
}

func (p *GCSProvider) fullPath(pth string) string {
	pth = strings.TrimPrefix(pth, "/")
	if p.prefix == "" {
		return pth
	}
	return p.prefix + "/" + pth
}

func (p *GCSProvider) stripPrefix(key string) string {
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

func (p *GCSProvider) toFileInfo(attrs *storage.ObjectAttrs) *FileInfo {
	key := p.stripPrefix(attrs.Name)
	name := ""
	if key != "" {
		parts := strings.Split(key, "/")
		name = parts[len(parts)-1]
	} else {
		name = key
	}

	isDir := strings.HasSuffix(attrs.Name, "/") && attrs.Size == 0

	return &FileInfo{
		Name:         name,
		Path:         key,
		Size:         attrs.Size,
		ModTime:      attrs.Updated,
		IsDir:        isDir,
		ContentType:  attrs.ContentType,
		ETag:         attrs.Etag,
		StorageClass: attrs.StorageClass,
		Metadata:     attrs.Metadata,
	}
}

func (p *GCSProvider) List(ctx context.Context, pth string, opts ListOptions) ([]FileInfo, error) {
	fullPrefix := p.fullPath(pth)
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	query := &storage.Query{
		Prefix:    fullPrefix,
		Delimiter: "/",
	}

	if opts.Recursive {
		query.Delimiter = ""
	}

	it := p.bucket.Objects(ctx, query)
	var files []FileInfo

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate objects: %w", err)
		}
		// With a delimiter set, GCS surfaces one synthetic "object" per common
		// prefix: attrs.Prefix is the directory path and attrs.Name is empty.
		if attrs.Prefix != "" {
			files = append(files, *p.toDirInfo(attrs.Prefix))
			continue
		}
		files = append(files, *p.toFileInfo(attrs))
	}

	return files, nil
}

func (p *GCSProvider) toDirInfo(prefix string) *FileInfo {
	key := p.stripPrefix(prefix)
	name := ""
	if key != "" {
		parts := strings.Split(strings.TrimSuffix(key, "/"), "/")
		name = parts[len(parts)-1]
	}
	return &FileInfo{
		Name:  name,
		Path:  key,
		Size:  0,
		IsDir: true,
	}
}

func (p *GCSProvider) Stat(ctx context.Context, pth string) (*FileInfo, error) {
	fullPath := p.fullPath(pth)

	attrs, err := p.bucket.Object(fullPath).Attrs(ctx)
	if err == storage.ErrObjectNotExist {
		// Check if it's a directory
		if !strings.HasSuffix(fullPath, "/") {
			fullPath += "/"
		}
		it := p.bucket.Objects(ctx, &storage.Query{Prefix: fullPath, Delimiter: "/"})
		_, err := it.Next()
		if err != iterator.Done {
			return p.toDirInfo(fullPath), nil
		}
		return nil, fmt.Errorf("object not found: %s", pth)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get object attrs: %w", err)
	}
	return p.toFileInfo(attrs), nil
}

func (p *GCSProvider) Get(ctx context.Context, pth string, w io.Writer) error {
	fullPath := p.fullPath(pth)
	reader, err := p.bucket.Object(fullPath).NewReader(ctx)
	if err != nil {
		return fmt.Errorf("failed to create reader: %w", err)
	}
	defer reader.Close()

	_, err = io.Copy(w, reader)
	return err
}

func (p *GCSProvider) Put(ctx context.Context, pth string, r io.Reader, opts PutOptions) error {
	fullPath := p.fullPath(pth)
	obj := p.bucket.Object(fullPath)

	writer := obj.NewWriter(ctx)
	writer.ContentType = opts.ContentType
	writer.Metadata = opts.Metadata
	writer.StorageClass = opts.StorageClass

	if _, err := io.Copy(writer, r); err != nil {
		writer.Close()
		return fmt.Errorf("failed to write object: %w", err)
	}
	return writer.Close()
}

func (p *GCSProvider) Delete(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)

	if strings.HasSuffix(fullPath, "/") || pth == "" {
		// Delete all objects with this prefix
		it := p.bucket.Objects(ctx, &storage.Query{Prefix: fullPath})
		for {
			attrs, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to iterate for deletion: %w", err)
			}
			if err := p.bucket.Object(attrs.Name).Delete(ctx); err != nil {
				return fmt.Errorf("failed to delete object: %w", err)
			}
		}
		return nil
	}

	return p.bucket.Object(fullPath).Delete(ctx)
}

func (p *GCSProvider) Mkdir(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)
	if !strings.HasSuffix(fullPath, "/") {
		fullPath += "/"
	}

	obj := p.bucket.Object(fullPath)
	writer := obj.NewWriter(ctx)
	return writer.Close()
}

func (p *GCSProvider) Copy(ctx context.Context, srcPath, dstPath string) error {
	srcFull := p.fullPath(srcPath)
	dstFull := p.fullPath(dstPath)

	srcObj := p.bucket.Object(srcFull)
	dstObj := p.bucket.Object(dstFull)

	_, err := dstObj.CopierFrom(srcObj).Run(ctx)
	return err
}

func (p *GCSProvider) Move(ctx context.Context, srcPath, dstPath string) error {
	if err := p.Copy(ctx, srcPath, dstPath); err != nil {
		return err
	}
	return p.Delete(ctx, srcPath)
}

func (p *GCSProvider) PresignURL(ctx context.Context, pth string, expiry time.Duration) (string, error) {
	fullPath := p.fullPath(pth)

	// BucketHandle.SignedURL signs with the client's credentials. Service-account
	// credentials sign locally; other credential types require IAM
	// roles/iam.serviceAccountTokenCreator and are rejected by the SDK.
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(expiry),
	}

	url, err := p.bucket.SignedURL(fullPath, opts)
	if err != nil {
		return "", fmt.Errorf("signed URL generation requires service account credentials with signer role: %w", err)
	}
	return url, nil
}

func (p *GCSProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsDirectories:     true,
		SupportsPresignURLs:     true,
		SupportsMultipart:       true,
		SupportsVersioning:      true,
		SupportsEncryption:      true,
		SupportsMetadata:        true,
		MaxFileSize:             5 * 1024 * 1024 * 1024 * 1024, // 5TB
		SupportedStorageClasses: []string{"STANDARD", "NEARLINE", "COLDLINE", "ARCHIVE"},
	}
}

func init() {
	RegisterProvider(NewGCSProvider())
}
