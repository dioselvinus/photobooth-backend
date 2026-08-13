package repository

import (
	"context"
	"sync"
	"time"

	"photobooth-backend/internal/models"
)

type MockSessionRepository struct {
	mu       sync.Mutex
	Sessions map[string]*models.SessionRecord
}

func NewMockSessionRepository() *MockSessionRepository {
	return &MockSessionRepository{
		Sessions: make(map[string]*models.SessionRecord),
	}
}

func (m *MockSessionRepository) Create(ctx context.Context, sessionCode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Sessions[sessionCode] = &models.SessionRecord{
		SessionCode: sessionCode,
		ImageURLs:   []string{},
		CreatedAt:   time.Now(),
	}
	return nil
}

func (m *MockSessionRepository) UpdateFileCount(ctx context.Context, sessionCode string, count int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.Sessions[sessionCode]
	if !ok {
		return ErrSessionNotFound
	}
	sess.FileCount = count
	return nil
}

func (m *MockSessionRepository) UpdateImagesAndEmail(ctx context.Context, sessionCode string, imageURLs []string, email string, fileCount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.Sessions[sessionCode]
	if !ok {
		return ErrSessionNotFound
	}
	sess.ImageURLs = append(sess.ImageURLs, imageURLs...)
	sess.Email = &email
	sess.FileCount = fileCount
	return nil
}

func (m *MockSessionRepository) GetByCode(ctx context.Context, sessionCode string) (*models.SessionRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.Sessions[sessionCode]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return sess, nil
}
