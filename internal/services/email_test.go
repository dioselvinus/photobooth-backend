package services

import (
	"sync"
	"testing"
	"time"

	"photobooth-backend/internal/config"
)

type mockSender struct {
	mu    sync.Mutex
	count int
}

func (m *mockSender) Send(to, subject, body string) error {
	m.mu.Lock()
	m.count++
	m.mu.Unlock()
	return nil
}

func TestAsyncEmailDispatcher(t *testing.T) {
	mock := &mockSender{}
	dispatcher := NewAsyncEmailDispatcher(mock, 10, 2)

	for i := 0; i < 5; i++ {
		_ = dispatcher.Send("test@example.com", "Subject", "Body")
	}

	time.Sleep(50 * time.Millisecond)
	dispatcher.Stop()

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.count != 5 {
		t.Errorf("Expected 5 emails processed by pool, got %d", mock.count)
	}
}

func TestNewMailerFromConfig(t *testing.T) {
	providers := []string{"smtp", "resend", "sendgrid", "mailgun", "noop"}
	for _, p := range providers {
		cfg := &config.Config{EmailProvider: p}
		sender, err := NewMailerFromConfig(cfg)
		if err != nil {
			t.Errorf("Failed to instantiate registered mailer provider %q: %v", p, err)
		}
		if sender == nil {
			t.Errorf("Expected non-nil sender for provider %q", p)
		}
	}

	// Test custom provider registration (Open-Closed Principle)
	RegisterProvider("custom_test", func(cfg *config.Config) (EmailSender, error) {
		return &mockSender{}, nil
	})

	cfg := &config.Config{EmailProvider: "custom_test"}
	customSender, err := NewMailerFromConfig(cfg)
	if err != nil || customSender == nil {
		t.Errorf("Failed to instantiate custom registered provider: %v", err)
	}
}
