// Package azureblob implements CloudStorageProvider for Azure Blob Storage.
package cloudstorage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
)

// AzureBlobConfig holds configuration for Azure Blob provider.
type AzureBlobConfig struct {
	AccountName   string `json:"account_name" toml:"account_name"`
	AccountKey    string `json:"account_key" toml:"account_key"`
	ContainerName string `json:"container_name" toml:"container_name"`
	Endpoint      string `json:"endpoint" toml:"endpoint"` // Optional custom endpoint
	Prefix        string `json:"prefix" toml:"prefix"`
	UseManagedID  bool   `json:"use_managed_id" toml:"use_managed_id"`
}

// AzureBlobProvider implements CloudStorageProvider for Azure Blob Storage.
type AzureBlobProvider struct {
	client        *container.Client
	sharedKeyCred *azblob.SharedKeyCredential // nil when using managed identity
	serviceURL    string
	container     string
	prefix        string
	endpoint      string
}

// NewAzureBlobProvider creates a new Azure Blob provider.
func NewAzureBlobProvider() *AzureBlobProvider {
	return &AzureBlobProvider{}
}

func (p *AzureBlobProvider) Name() string {
	return "azure"
}

func (p *AzureBlobProvider) DisplayName() string {
	return "Azure Blob Storage"
}

func (p *AzureBlobProvider) Init(ctx context.Context, configMap map[string]string) error {
	cfg := AzureBlobConfig{}
	for k, v := range configMap {
		switch k {
		case "account_name":
			cfg.AccountName = v
		case "account_key":
			cfg.AccountKey = v
		case "container_name":
			cfg.ContainerName = v
		case "endpoint":
			cfg.Endpoint = v
		case "prefix":
			cfg.Prefix = v
		case "use_managed_id":
			cfg.UseManagedID = v == "true"
		}
	}

	if cfg.AccountName == "" {
		return fmt.Errorf("account_name is required")
	}
	if cfg.ContainerName == "" {
		return fmt.Errorf("container_name is required")
	}
	// Note: missing account_key / use_managed_id is surfaced here as a credential
	// construction failure below; requirement is declared by ValidateConfig.

	p.container = cfg.ContainerName
	p.prefix = strings.Trim(cfg.Prefix, "/")
	p.endpoint = cfg.Endpoint
	p.serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AccountName)

	// Build the credential chain: account key (shared key) or managed identity.
	if cfg.UseManagedID {
		cred, err := azidentity.NewManagedIdentityCredential(nil)
		if err != nil {
			return fmt.Errorf("failed to create managed identity credential: %w", err)
		}
		p.serviceURL = strings.TrimSuffix(cfg.Endpoint, "/")
		if cfg.Endpoint == "" {
			p.serviceURL = fmt.Sprintf("https://%s.blob.core.windows.net", cfg.AccountName)
		}
		containerURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(p.serviceURL, "/"), cfg.ContainerName)
		p.client, err = container.NewClient(containerURL, cred, nil)
		if err != nil {
			return fmt.Errorf("failed to create container client: %w", err)
		}
		return nil
	}

	// Shared key credential: required for service-level SAS (presigned) URLs.
	sharedKey, err := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccountKey)
	if err != nil {
		return fmt.Errorf("failed to create shared key credential: %w", err)
	}
	p.sharedKeyCred = sharedKey
	if cfg.Endpoint != "" {
		p.serviceURL = strings.TrimSuffix(cfg.Endpoint, "/")
	}
	containerURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(p.serviceURL, "/"), cfg.ContainerName)
	p.client, err = container.NewClientWithSharedKeyCredential(containerURL, sharedKey, nil)
	if err != nil {
		return fmt.Errorf("failed to create container client: %w", err)
	}

	return nil
}

func (p *AzureBlobProvider) ValidateConfig(configMap map[string]string) error {
	if _, ok := configMap["account_name"]; !ok {
		return fmt.Errorf("account_name is required")
	}
	if _, ok := configMap["container_name"]; !ok {
		return fmt.Errorf("container_name is required")
	}
	if _, ok := configMap["account_key"]; !ok {
		if _, ok := configMap["use_managed_id"]; !ok {
			return fmt.Errorf("account_key or use_managed_id is required")
		}
	}
	return nil
}

func (p *AzureBlobProvider) fullPath(pth string) string {
	pth = strings.TrimPrefix(pth, "/")
	if p.prefix == "" {
		return pth
	}
	return p.prefix + "/" + pth
}

func (p *AzureBlobProvider) stripPrefix(key string) string {
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

// deref safely dereferences a string pointer; nil becomes an empty string.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (p *AzureBlobProvider) toFileInfo(item *container.BlobItem) *FileInfo {
	name := deref(item.Name)
	key := p.stripPrefix(name)
	fileName := ""
	if key != "" {
		parts := strings.Split(key, "/")
		fileName = parts[len(parts)-1]
	} else {
		fileName = key
	}

	var size int64
	if item.Properties.ContentLength != nil {
		size = *item.Properties.ContentLength
	}

	var modTime time.Time
	if item.Properties.LastModified != nil {
		modTime = *item.Properties.LastModified
	}

	isDir := strings.HasSuffix(name, "/")

	return &FileInfo{
		Name:        fileName,
		Path:        key,
		Size:        size,
		ModTime:     modTime,
		IsDir:       isDir,
		ContentType: "",
		ETag:        "",
	}
}

func (p *AzureBlobProvider) List(ctx context.Context, pth string, opts ListOptions) ([]FileInfo, error) {
	fullPrefix := p.fullPath(pth)
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	pager := p.client.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
		Prefix: &fullPrefix,
	})

	var files []FileInfo
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list blobs: %w", err)
		}
		for _, item := range resp.Segment.BlobItems {
			files = append(files, *p.toFileInfo(item))
		}
	}

	return files, nil
}

func (p *AzureBlobProvider) Stat(ctx context.Context, pth string) (*FileInfo, error) {
	fullPath := p.fullPath(pth)

	// Try to get blob properties
	blobClient := p.client.NewBlockBlobClient(fullPath)
	props, err := blobClient.GetProperties(ctx, nil)
	if err != nil {
		// Check if it's a virtual directory
		listOpts := &container.ListBlobsFlatOptions{
			Prefix: to.Ptr(fullPath + "/"),
		}
		pager := p.client.NewListBlobsFlatPager(listOpts)
		if pager.More() {
			resp, err := pager.NextPage(ctx)
			if err == nil && len(resp.Segment.BlobItems) > 0 {
				return &FileInfo{
					Name:  strings.TrimSuffix(strings.TrimPrefix(fullPath, p.prefix+"/"), "/"),
					Path:  p.stripPrefix(fullPath),
					Size:  0,
					IsDir: true,
				}, nil
			}
		}
		return nil, fmt.Errorf("failed to get blob properties: %w", err)
	}

	info := &FileInfo{
		Name:    strings.TrimPrefix(fullPath, p.prefix+"/"),
		Path:    p.stripPrefix(fullPath),
		Size:    *props.ContentLength,
		ModTime: *props.LastModified,
		IsDir:   false,
	}
	if props.ContentType != nil {
		info.ContentType = *props.ContentType
	}
	if props.ETag != nil {
		info.ETag = string(*props.ETag)
	}
	return info, nil
}

func (p *AzureBlobProvider) Get(ctx context.Context, pth string, w io.Writer) error {
	fullPath := p.fullPath(pth)
	blobClient := p.client.NewBlockBlobClient(fullPath)

	resp, err := blobClient.DownloadStream(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to download blob: %w", err)
	}
	defer resp.Body.Close()

	_, err = io.Copy(w, resp.Body)
	return err
}

func (p *AzureBlobProvider) Put(ctx context.Context, pth string, r io.Reader, opts PutOptions) error {
	fullPath := p.fullPath(pth)
	blobClient := p.client.NewBlockBlobClient(fullPath)

	uploadOpts := &blockblob.UploadStreamOptions{}
	if opts.ContentType != "" {
		uploadOpts.HTTPHeaders = &blob.HTTPHeaders{
			BlobContentType: to.Ptr(opts.ContentType),
		}
	}
	if len(opts.Metadata) > 0 {
		metadata := make(map[string]*string, len(opts.Metadata))
		for k, v := range opts.Metadata {
			metadata[k] = to.Ptr(v)
		}
		uploadOpts.Metadata = metadata
	}

	_, err := blobClient.UploadStream(ctx, r, uploadOpts)
	if err != nil {
		return fmt.Errorf("failed to upload blob: %w", err)
	}
	return nil
}

func (p *AzureBlobProvider) Delete(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)

	// Check if it's a directory (prefix)
	if strings.HasSuffix(fullPath, "/") || pth == "" {
		// Delete all blobs with this prefix
		pager := p.client.NewListBlobsFlatPager(&container.ListBlobsFlatOptions{
			Prefix: &fullPath,
		})

		for pager.More() {
			resp, err := pager.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("failed to list blobs for deletion: %w", err)
			}
			for _, item := range resp.Segment.BlobItems {
				blobClient := p.client.NewBlockBlobClient(deref(item.Name))
				_, err := blobClient.Delete(ctx, nil)
				if err != nil {
					return fmt.Errorf("failed to delete blob: %w", err)
				}
			}
		}
		return nil
	}

	blobClient := p.client.NewBlockBlobClient(fullPath)
	_, err := blobClient.Delete(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to delete blob: %w", err)
	}
	return nil
}

func (p *AzureBlobProvider) Mkdir(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)
	if !strings.HasSuffix(fullPath, "/") {
		fullPath += "/"
	}

	// Create empty blob with trailing slash
	blobClient := p.client.NewBlockBlobClient(fullPath)
	_, err := blobClient.UploadStream(ctx, strings.NewReader(""), nil)
	return err
}

func (p *AzureBlobProvider) Copy(ctx context.Context, srcPath, dstPath string) error {
	srcFull := p.fullPath(srcPath)
	dstFull := p.fullPath(dstPath)

	srcURL := fmt.Sprintf("%s/%s", strings.TrimSuffix(p.client.URL(), "/"), srcFull)

	dstBlobClient := p.client.NewBlockBlobClient(dstFull)

	_, err := dstBlobClient.CopyFromURL(ctx, srcURL, nil)
	return err
}

func (p *AzureBlobProvider) Move(ctx context.Context, srcPath, dstPath string) error {
	if err := p.Copy(ctx, srcPath, dstPath); err != nil {
		return err
	}
	return p.Delete(ctx, srcPath)
}

func (p *AzureBlobProvider) PresignURL(ctx context.Context, pth string, expiry time.Duration) (string, error) {
	fullPath := p.fullPath(pth)

	if p.sharedKeyCred == nil {
		return "", fmt.Errorf("presigned URLs require account_key credential; managed identity cannot sign SAS tokens")
	}

	blobClient := p.client.NewBlockBlobClient(fullPath)
	// A user delegation key is issued with the account key (shared key) credential.
	// blob.Client.GetSASURL signs a blob-scoped read SAS valid for the given expiry.
	url, err := blobClient.GetSASURL(sas.BlobPermissions{Read: true}, time.Now().UTC().Add(expiry), nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate SAS URL: %w", err)
	}
	return url, nil
}

func (p *AzureBlobProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsDirectories:     true,
		SupportsPresignURLs:     true,
		SupportsMultipart:       true,
		SupportsVersioning:      true,
		SupportsEncryption:      true,
		SupportsMetadata:        true,
		MaxFileSize:             192 * 1024 * 1024 * 1024, // 192TB for block blobs
		SupportedStorageClasses: []string{"Hot", "Cool", "Cold", "Archive"},
	}
}

func init() {
	RegisterProvider(NewAzureBlobProvider())
}
