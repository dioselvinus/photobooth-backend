package repository

import (
	"context"
	"testing"
)

func TestMockSessionRepository(t *testing.T) {
	repo := NewMockSessionRepository()
	ctx := context.Background()

	if err := repo.Create(ctx, "abc12345"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.UpdateFileCount(ctx, "abc12345", 3); err != nil {
		t.Fatalf("UpdateFileCount failed: %v", err)
	}

	sess, err := repo.GetByCode(ctx, "abc12345")
	if err != nil || sess == nil {
		t.Fatalf("GetByCode failed: %v", err)
	}

	if sess.FileCount != 3 {
		t.Errorf("Expected file count 3, got %d", sess.FileCount)
	}
}
