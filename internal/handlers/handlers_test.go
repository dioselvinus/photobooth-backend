package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"photobooth-backend/internal/config"
	"photobooth-backend/internal/repository"
	"photobooth-backend/internal/services"
	"photobooth-backend/internal/storage"
)

func setupTestHandler() (*Handler, *repository.MockSessionRepository) {
	repo := repository.NewMockSessionRepository()
	cfg := &config.Config{
		AppEnv:          "local",
		Port:            "8080",
		BaseDomain:      "localhost:8080",
		StorageProvider: "mock",
	}
	tmpl := template.Must(template.New("404.html").Parse("404 Not Found"))
	presigner := &storage.MockStoragePresigner{Bucket: "mock-bucket"}
	mailer := services.NewAsyncEmailDispatcher(&services.NoOpSender{}, 10, 1)

	h := NewHandler(repo, cfg, tmpl, presigner, mailer)
	return h, repo
}

func TestHandleCreateSession(t *testing.T) {
	h, _ := setupTestHandler()

	req := httptest.NewRequest("POST", "/api/sessions", bytes.NewBufferString(`{"device_id":"kiosk-1"}`))
	rec := httptest.NewRecorder()

	h.HandleCreateSession(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rec.Code)
	}

	var res map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&res)
	if res["status"] != "success" {
		t.Errorf("Expected status success, got %v", res["status"])
	}
	if res["session_code"] == nil || len(res["session_code"].(string)) != 8 {
		t.Errorf("Expected 8-character session code")
	}
}

func TestHandleGenerateBatchUploads(t *testing.T) {
	h, repo := setupTestHandler()
	ctx := context.Background()

	sessionCode := "testcode"
	_ = repo.Create(ctx, sessionCode)

	body := `{"files":["photo1.jpg","photo2.jpg"]}`
	req := httptest.NewRequest("POST", "/api/sessions/testcode/uploads", bytes.NewBufferString(body))
	req.SetPathValue("code", sessionCode)
	rec := httptest.NewRecorder()

	h.HandleGenerateBatchUploads(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", rec.Code)
	}

	var res map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&res)
	if res["status"] != "success" {
		t.Errorf("Expected status success, got %v", res["status"])
	}
	urls, ok := res["upload_urls"].(map[string]interface{})
	if !ok || len(urls) != 2 {
		t.Errorf("Expected 2 upload URLs, got %v", res["upload_urls"])
	}
}

func TestHandleUpdateSession(t *testing.T) {
	h, repo := setupTestHandler()
	ctx := context.Background()

	sessionCode := "testcode"
	_ = repo.Create(ctx, sessionCode)

	body := `{"email":"user@example.com","image_urls":["http://localhost:9000/mock-bucket/sessions/testcode/p1.jpg"],"file_count":1}`
	req := httptest.NewRequest("PATCH", "/api/sessions/testcode", bytes.NewBufferString(body))
	req.SetPathValue("code", sessionCode)
	rec := httptest.NewRecorder()

	h.HandleUpdateSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", rec.Code)
	}
}
