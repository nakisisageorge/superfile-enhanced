// Package s3provider implements CloudStorageProvider for S3-compatible storage.
// Supports AWS S3, MinIO, DigitalOcean Spaces, Wasabi, and other S3-compatible services.
package cloudstorage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Provider implements CloudStorageProvider for S3-compatible storage.
type S3Provider struct {
	client       *s3.Client
	bucket       string
	prefix       string
	endpoint     string
	region       string
	usePathStyle bool
}

// S3Config holds configuration for S3 provider.
type S3Config struct {
	Bucket       string `json:"bucket" toml:"bucket"`
	Region       string `json:"region" toml:"region"`
	Endpoint     string `json:"endpoint" toml:"endpoint"` // For MinIO, DigitalOcean, etc.
	AccessKey    string `json:"access_key" toml:"access_key"`
	SecretKey    string `json:"secret_key" toml:"secret_key"`
	Prefix       string `json:"prefix" toml:"prefix"` // Optional path prefix within bucket
	UsePathStyle bool   `json:"use_path_style" toml:"use_path_style"`
}

// NewS3Provider creates a new S3 provider.
func NewS3Provider() *S3Provider {
	return &S3Provider{}
}

// Name returns the provider name.
func (p *S3Provider) Name() string {
	return "s3"
}

// DisplayName returns the human-readable name.
func (p *S3Provider) DisplayName() string {
	return "S3 Compatible (AWS S3, MinIO, DigitalOcean Spaces, Wasabi)"
}

// Init initializes the S3 client.
func (p *S3Provider) Init(ctx context.Context, configMap map[string]string) error {
	cfg := S3Config{}
	for k, v := range configMap {
		switch k {
		case "bucket":
			cfg.Bucket = v
		case "region":
			cfg.Region = v
		case "endpoint":
			cfg.Endpoint = v
		case "access_key":
			cfg.AccessKey = v
		case "secret_key":
			cfg.SecretKey = v
		case "prefix":
			cfg.Prefix = v
		case "use_path_style":
			cfg.UsePathStyle = v == "true"
		}
	}

	if cfg.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}

	p.bucket = cfg.Bucket
	p.prefix = strings.Trim(cfg.Prefix, "/")
	p.endpoint = cfg.Endpoint
	p.region = cfg.Region
	p.usePathStyle = cfg.UsePathStyle

	// Load AWS config
	var opts []func(*config.LoadOptions) error
	opts = append(opts, config.WithRegion(cfg.Region))

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		))
	}

	if cfg.Endpoint != "" {
		opts = append(opts, config.WithBaseEndpoint(cfg.Endpoint))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint if provided
	s3Options := func(o *s3.Options) {
		o.UsePathStyle = p.usePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	}

	p.client = s3.NewFromConfig(awsCfg, s3Options)
	return nil
}

// ValidateConfig validates the S3 configuration.
func (p *S3Provider) ValidateConfig(configMap map[string]string) error {
	if _, ok := configMap["bucket"]; !ok {
		return fmt.Errorf("bucket is required")
	}
	if _, ok := configMap["region"]; !ok {
		return fmt.Errorf("region is required")
	}
	if _, ok := configMap["access_key"]; !ok {
		// Allow no credentials for IAM roles
	}
	if _, ok := configMap["secret_key"]; !ok {
		// Allow no credentials for IAM roles
	}
	return nil
}

// fullPath returns the full S3 key for a given path.
func (p *S3Provider) fullPath(pth string) string {
	pth = strings.TrimPrefix(pth, "/")
	if p.prefix == "" {
		return pth
	}
	return path.Join(p.prefix, pth)
}

// stripPrefix removes the provider prefix from an S3 key.
func (p *S3Provider) stripPrefix(key string) string {
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

// toFileInfo converts an S3 object to FileInfo.
func (p *S3Provider) toFileInfo(obj types.Object) *FileInfo {
	key := p.stripPrefix(aws.ToString(obj.Key))
	name := path.Base(key)
	if name == "" && key != "" {
		name = key
	}

	return &FileInfo{
		Name:        name,
		Path:        key,
		Size:        aws.ToInt64(obj.Size),
		ModTime:     aws.ToTime(obj.LastModified),
		IsDir:       false,
		ContentType: "",
		ETag:        strings.Trim(aws.ToString(obj.ETag), "\""),
	}
}

// toDirInfo converts a common prefix to a directory FileInfo.
func (p *S3Provider) toDirInfo(prefix string) *FileInfo {
	key := p.stripPrefix(prefix)
	name := path.Base(strings.TrimSuffix(key, "/"))
	if name == "" {
		name = key
	}
	return &FileInfo{
		Name:    name,
		Path:    key,
		Size:    0,
		ModTime: time.Time{},
		IsDir:   true,
		ETag:    "",
	}
}

func (p *S3Provider) List(ctx context.Context, pth string, opts ListOptions) ([]FileInfo, error) {
	fullPrefix := p.fullPath(pth)
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}

	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(p.bucket),
		Prefix:  aws.String(fullPrefix),
		MaxKeys: aws.Int32(int32(max(opts.MaxKeys, 1000))),
	}

	if opts.Delimiter != "" {
		input.Delimiter = aws.String(opts.Delimiter)
	} else if !opts.Recursive {
		input.Delimiter = aws.String("/")
	}

	if opts.Marker != "" {
		input.ContinuationToken = aws.String(opts.Marker)
	}

	result, err := p.client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	var files []FileInfo

	// Add directories (common prefixes)
	for _, cp := range result.CommonPrefixes {
		files = append(files, *p.toDirInfo(aws.ToString(cp.Prefix)))
	}

	// Add files
	for _, obj := range result.Contents {
		// Skip the "directory" placeholder object
		if strings.HasSuffix(aws.ToString(obj.Key), "/") && aws.ToInt64(obj.Size) == 0 {
			continue
		}
		files = append(files, *p.toFileInfo(obj))
	}

	return files, nil
}

func (p *S3Provider) Stat(ctx context.Context, pth string) (*FileInfo, error) {
	fullPath := p.fullPath(pth)

	// Try to get object metadata
	headOut, err := p.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullPath),
	})
	if err != nil {
		// Check if it's a "directory" (common prefix)
		listOut, err := p.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:    aws.String(p.bucket),
			Prefix:    aws.String(fullPath + "/"),
			MaxKeys:   aws.Int32(1),
			Delimiter: aws.String("/"),
		})
		if err == nil && (len(listOut.CommonPrefixes) > 0 || len(listOut.Contents) > 0) {
			return p.toDirInfo(fullPath + "/"), nil
		}
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	// Check if it's a directory marker
	if strings.HasSuffix(fullPath, "/") {
		return p.toDirInfo(fullPath), nil
	}

	info := p.toFileInfo(types.Object{
		Key:          aws.String(fullPath),
		Size:         headOut.ContentLength,
		LastModified: headOut.LastModified,
		ETag:         headOut.ETag,
	})
	info.ContentType = aws.ToString(headOut.ContentType)
	return info, nil
}

func (p *S3Provider) Get(ctx context.Context, pth string, w io.Writer) error {
	fullPath := p.fullPath(pth)

	downloader := manager.NewDownloader(p.client)
	input := &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullPath),
	}
	// The S3 downloader writes ranges out of order (multipart concurrency), so a
	// plain io.Writer can only be used directly when it also implements io.WriterAt.
	if wa, ok := w.(io.WriterAt); ok {
		_, err := downloader.Download(ctx, wa, input)
		if err != nil {
			return fmt.Errorf("failed to download object: %w", err)
		}
		return nil
	}
	buf := manager.NewWriteAtBuffer(nil)
	if _, err := downloader.Download(ctx, buf, input); err != nil {
		return fmt.Errorf("failed to download object: %w", err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}
	return nil
}

func (p *S3Provider) Put(ctx context.Context, pth string, r io.Reader, opts PutOptions) error {
	fullPath := p.fullPath(pth)

	input := &s3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullPath),
		Body:   r,
	}

	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}
	if len(opts.Metadata) > 0 {
		input.Metadata = opts.Metadata
	}
	if opts.StorageClass != "" {
		input.StorageClass = types.StorageClass(opts.StorageClass)
	}
	if opts.ACL != "" {
		input.ACL = types.ObjectCannedACL(opts.ACL)
	}

	uploader := manager.NewUploader(p.client)
	_, err := uploader.Upload(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to upload object: %w", err)
	}
	return nil
}

func (p *S3Provider) Delete(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)

	// Check if it's a directory (has trailing slash or is a prefix)
	if strings.HasSuffix(fullPath, "/") || pth == "" {
		// Delete all objects with this prefix
		paginator := s3.NewListObjectsV2Paginator(p.client, &s3.ListObjectsV2Input{
			Bucket: aws.String(p.bucket),
			Prefix: aws.String(fullPath),
		})

		var objectsToDelete []types.ObjectIdentifier
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				return fmt.Errorf("failed to list objects for deletion: %w", err)
			}
			for _, obj := range page.Contents {
				objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
					Key: aws.String(aws.ToString(obj.Key)),
				})
			}
		}

		if len(objectsToDelete) > 0 {
			_, err := p.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(p.bucket),
				Delete: &types.Delete{Objects: objectsToDelete, Quiet: aws.Bool(true)},
			})
			if err != nil {
				return fmt.Errorf("failed to delete objects: %w", err)
			}
		}
		return nil
	}

	// Delete single object
	_, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullPath),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

func (p *S3Provider) Mkdir(ctx context.Context, pth string) error {
	fullPath := p.fullPath(pth)
	if !strings.HasSuffix(fullPath, "/") {
		fullPath += "/"
	}

	// Create empty object with trailing slash to simulate directory
	_, err := p.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullPath),
		Body:   strings.NewReader(""),
	})
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	return nil
}

func (p *S3Provider) Copy(ctx context.Context, srcPath, dstPath string) error {
	srcFull := p.fullPath(srcPath)
	dstFull := p.fullPath(dstPath)

	_, err := p.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(p.bucket),
		Key:        aws.String(dstFull),
		CopySource: aws.String(url.PathEscape(p.bucket + "/" + srcFull)),
	})
	if err != nil {
		return fmt.Errorf("failed to copy object: %w", err)
	}
	return nil
}

func (p *S3Provider) Move(ctx context.Context, srcPath, dstPath string) error {
	if err := p.Copy(ctx, srcPath, dstPath); err != nil {
		return err
	}
	return p.Delete(ctx, srcPath)
}

func (p *S3Provider) PresignURL(ctx context.Context, pth string, expiry time.Duration) (string, error) {
	fullPath := p.fullPath(pth)

	presigner := s3.NewPresignClient(p.client)
	resp, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(p.bucket),
		Key:    aws.String(fullPath),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return resp.URL, nil
}

func (p *S3Provider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SupportsDirectories:     true,
		SupportsPresignURLs:     true,
		SupportsMultipart:       true,
		SupportsVersioning:      true,
		SupportsEncryption:      true,
		SupportsMetadata:        true,
		MaxFileSize:             5 * 1024 * 1024 * 1024 * 1024, // 5TB
		SupportedStorageClasses: []string{"STANDARD", "STANDARD_IA", "GLACIER", "DEEP_ARCHIVE", "REDUCED_REDUNDANCY"},
	}
}

// Register S3 provider on init
func init() {
	RegisterProvider(NewS3Provider())
}
