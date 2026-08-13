package storage

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"photobooth-backend/internal/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// StoragePresigner defines the abstraction for Object Storage operations (Interface Segregation & Dependency Inversion)
type StoragePresigner interface {
	EnsureBucket(ctx context.Context) error
	GeneratePresignedUploadURL(objectKey string, lifetime time.Duration) (string, error)
	GeneratePresignedDownloadURL(objectKey, filename string, lifetime time.Duration) (string, error)
	ExtractObjectKey(rawURL string) (string, error)
	GetBucketName() string
}

// --- Storage Provider Registry (Open-Closed Principle) ---
type StorageBuilder func(cfg *config.Config) (StoragePresigner, error)

var (
	storageRegistryMu sync.RWMutex
	storageRegistry   = make(map[string]StorageBuilder)
)

func init() {
	// 1. Generic S3 Provider (AWS S3, MinIO, Wasabi, DigitalOcean Spaces)
	RegisterStorageProvider("s3", func(cfg *config.Config) (StoragePresigner, error) {
		return NewS3Presigner(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket), nil
	})
	RegisterStorageProvider("minio", func(cfg *config.Config) (StoragePresigner, error) {
		return NewS3Presigner(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket), nil
	})

	// 2. Cloudflare R2 Provider (uses unified SSOT configuration)
	RegisterStorageProvider("r2", func(cfg *config.Config) (StoragePresigner, error) {
		return NewS3Presigner(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket), nil
	})

	// 3. Mock / Local Dev Fallback Provider
	RegisterStorageProvider("mock", func(cfg *config.Config) (StoragePresigner, error) {
		return &MockStoragePresigner{Bucket: "mock-bucket"}, nil
	})
}

// RegisterStorageProvider allows adding new object storage providers without modifying existing code.
func RegisterStorageProvider(name string, builder StorageBuilder) {
	storageRegistryMu.Lock()
	defer storageRegistryMu.Unlock()
	storageRegistry[strings.ToLower(name)] = builder
}

// NewStorageFromConfig dynamically builds the configured StoragePresigner instance.
// If an unlisted provider is specified (e.g. Backblaze, Wasabi, DigitalOcean, Scaleway, Linode),
// it automatically falls back to the generic S3-compatible driver.
func NewStorageFromConfig(cfg *config.Config) (StoragePresigner, error) {
	storageRegistryMu.RLock()
	builder, ok := storageRegistry[strings.ToLower(cfg.StorageProvider)]
	storageRegistryMu.RUnlock()

	if !ok {
		slog.Info("Unregistered storage provider specified; using universal S3-compatible driver", "provider", cfg.StorageProvider, "endpoint", cfg.S3Endpoint)
		return NewS3Presigner(cfg.S3Endpoint, cfg.S3Region, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket), nil
	}
	return builder(cfg)
}

// AvailableStorageProviders returns a list of registered storage provider names.
func AvailableStorageProviders() []string {
	storageRegistryMu.RLock()
	defer storageRegistryMu.RUnlock()
	keys := make([]string, 0, len(storageRegistry))
	for k := range storageRegistry {
		keys = append(keys, k)
	}
	return keys
}

// --- Standard S3 & Cloudflare R2 Presigner Implementation ---
type Presigner struct {
	Client *s3.Client
	Bucket string
}

func NewS3Presigner(endpoint, region, accessKey, secretKey, bucket string) *Presigner {
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		slog.Error("Failed to load AWS S3 SDK config", "error", err)
		panic(err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		o.UsePathStyle = true
	})

	return &Presigner{
		Client: client,
		Bucket: bucket,
	}
}

func NewR2Presigner(accountID, accessKey, secretKey, bucket string) *Presigner {
	r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
	cfg, err := awsconfig.LoadDefaultConfig(context.TODO(),
		awsconfig.WithRegion("auto"),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		slog.Error("Failed to load Cloudflare R2 SDK config", "error", err)
		panic(err)
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(r2Endpoint)
		o.UsePathStyle = true
	})

	slog.Info("Initialized Cloudflare R2 Presigner", "endpoint", r2Endpoint, "bucket", bucket)
	return &Presigner{
		Client: client,
		Bucket: bucket,
	}
}

func (p *Presigner) GetBucketName() string {
	return p.Bucket
}

func (p *Presigner) EnsureBucket(ctx context.Context) error {
	_, err := p.Client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(p.Bucket)})
	if err == nil {
		return nil
	}
	_, err = p.Client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(p.Bucket)})
	return err
}

func (p *Presigner) GeneratePresignedUploadURL(objectKey string, lifetime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(p.Client)

	request, err := presignClient.PresignPutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: aws.String(p.Bucket),
		Key:    aws.String(objectKey),
	}, s3.WithPresignExpires(lifetime))

	if err != nil {
		return "", err
	}

	return request.URL, nil
}

func (p *Presigner) GeneratePresignedDownloadURL(objectKey, filename string, lifetime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(p.Client)

	request, err := presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket:                     aws.String(p.Bucket),
		Key:                        aws.String(objectKey),
		ResponseContentDisposition: aws.String(`attachment; filename="` + filename + `"`),
	}, s3.WithPresignExpires(lifetime))

	if err != nil {
		return "", err
	}

	return request.URL, nil
}

// ExtractObjectKey parses a raw stored URL and extracts the clean object key.
func (p *Presigner) ExtractObjectKey(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimPrefix(u.Path, "/")
	if p.Bucket != "" {
		path = strings.TrimPrefix(path, p.Bucket+"/")
	}
	if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, "..") {
		return "", fmt.Errorf("invalid object path")
	}
	return path, nil
}

// --- Mock / Fallback Presigner Implementation ---
type MockStoragePresigner struct {
	Bucket string
}

func (m *MockStoragePresigner) GetBucketName() string {
	return m.Bucket
}

func (m *MockStoragePresigner) EnsureBucket(ctx context.Context) error {
	return nil
}

func (m *MockStoragePresigner) GeneratePresignedUploadURL(objectKey string, lifetime time.Duration) (string, error) {
	return fmt.Sprintf("http://localhost:9000/%s/%s?upload=true", m.Bucket, objectKey), nil
}

func (m *MockStoragePresigner) GeneratePresignedDownloadURL(objectKey, filename string, lifetime time.Duration) (string, error) {
	return fmt.Sprintf("http://localhost:9000/%s/%s?download=true", m.Bucket, objectKey), nil
}

func (m *MockStoragePresigner) ExtractObjectKey(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimPrefix(path, m.Bucket+"/")
	return path, nil
}
