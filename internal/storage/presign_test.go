package storage

import (
	"context"
	"testing"
	"time"

	"photobooth-backend/internal/config"
)

func TestStorageFactory(t *testing.T) {
	providers := []struct {
		name string
		cfg  *config.Config
	}{
		{
			name: "s3",
			cfg: &config.Config{
				StorageProvider: "s3",
				S3Region:        "us-east-1",
				S3AccessKey:     "test",
				S3SecretKey:     "test",
				S3Bucket:        "my-bucket",
			},
		},
		{
			name: "r2",
			cfg: &config.Config{
				StorageProvider: "r2",
				S3Endpoint:      "https://test-account-id.r2.cloudflarestorage.com",
				S3Region:        "auto",
				S3AccessKey:     "test-key",
				S3SecretKey:     "test-secret",
				S3Bucket:        "r2-bucket",
			},
		},
		{
			name: "mock",
			cfg: &config.Config{
				StorageProvider: "mock",
			},
		},
		{
			name: "unlisted_backblaze",
			cfg: &config.Config{
				StorageProvider: "backblaze_b2",
				S3Endpoint:      "https://s3.us-west-000.backblazeb2.com",
				S3Region:        "us-west-000",
				S3AccessKey:     "key",
				S3SecretKey:     "secret",
				S3Bucket:        "my-b2-bucket",
			},
		},
	}

	for _, tc := range providers {
		p, err := NewStorageFromConfig(tc.cfg)
		if err != nil {
			t.Errorf("Failed to instantiate storage provider %q: %v", tc.name, err)
		}
		if p == nil {
			t.Errorf("Expected non-nil presigner for provider %q", tc.name)
		}
	}

	// Test custom storage registration (Open-Closed Principle)
	RegisterStorageProvider("custom_disk", func(cfg *config.Config) (StoragePresigner, error) {
		return &MockStoragePresigner{Bucket: "custom-bucket"}, nil
	})

	cfg := &config.Config{StorageProvider: "custom_disk"}
	customStorage, err := NewStorageFromConfig(cfg)
	if err != nil || customStorage == nil {
		t.Errorf("Failed to instantiate custom registered storage provider: %v", err)
	}

	uploadURL, err := customStorage.GeneratePresignedUploadURL("test.jpg", 10*time.Minute)
	if err != nil || uploadURL == "" {
		t.Errorf("GeneratePresignedUploadURL failed: %v", err)
	}

	downloadURL, err := customStorage.GeneratePresignedDownloadURL("test.jpg", "test.jpg", 10*time.Minute)
	if err != nil || downloadURL == "" {
		t.Errorf("GeneratePresignedDownloadURL failed: %v", err)
	}

	if err := customStorage.EnsureBucket(context.Background()); err != nil {
		t.Errorf("EnsureBucket failed: %v", err)
	}
}
