package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"photobooth-backend/internal/config"
	"photobooth-backend/internal/models"
	"photobooth-backend/internal/repository"
	"photobooth-backend/internal/services"
	"photobooth-backend/internal/storage"
	"photobooth-backend/internal/utils"
)

// galleryLifetime is how long a gallery remains viewable after creation.
const galleryLifetime = 30 * 24 * time.Hour

// Handler encapsulates HTTP request handling, delegating business and data concerns (Single Responsibility & Dependency Inversion)
type Handler struct {
	Repo      repository.SessionRepository // Interface abstraction (DIP)
	Cfg       *config.Config
	Tmpl      *template.Template
	Presigner storage.StoragePresigner // Polymorphic storage interface (OCP & LSP)
	Mailer    services.EmailSender     // Polymorphic email interface (OCP & LSP)
}

func NewHandler(repo repository.SessionRepository, cfg *config.Config, tmpl *template.Template, presigner storage.StoragePresigner, mailer services.EmailSender) *Handler {
	return &Handler{Repo: repo, Cfg: cfg, Tmpl: tmpl, Presigner: presigner, Mailer: mailer}
}

// respondJSONError produces standardized JSON error responses for API clients.
func respondJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "error",
		"message": message,
	})
}

// POST /api/sessions (RESTful Create - initializes session record with short code)
func (h *Handler) HandleCreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req models.CreateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.DeviceID = "unknown-kiosk"
	}

	sessionCode := utils.GenerateShortCode(8)

	if err := h.Repo.Create(r.Context(), sessionCode); err != nil {
		slog.Error("Failed to create session in repository", "session_code", sessionCode, "error", err)
		respondJSONError(w, http.StatusInternalServerError, "Internal database error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"session_code": sessionCode,
	})
}

// POST /api/sessions/{code}/uploads (RESTful Sub-resource: Generate batch upload URLs)
func (h *Handler) HandleGenerateBatchUploads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	code := r.PathValue("code")
	if code == "" {
		respondJSONError(w, http.StatusBadRequest, "Session code is required")
		return
	}

	var req models.BatchUploadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Files) == 0 {
		respondJSONError(w, http.StatusBadRequest, "Invalid request payload or empty files list")
		return
	}

	if err := h.Repo.UpdateFileCount(r.Context(), code, len(req.Files)); err != nil {
		if errors.Is(err, repository.ErrSessionNotFound) {
			respondJSONError(w, http.StatusNotFound, "Session not found")
			return
		}
		slog.Error("Failed to update session file count", "session_code", code, "error", err)
		respondJSONError(w, http.StatusInternalServerError, "Internal database error")
		return
	}

	uploadMap := make(map[string]string)
	for _, fileName := range req.Files {
		objectKey := fmt.Sprintf("sessions/%s/%s", code, filepath.Base(fileName))
		presignedURL, err := h.Presigner.GeneratePresignedUploadURL(objectKey, 15*time.Minute)
		if err != nil {
			slog.Error("Failed to generate presigned upload URL", "file", fileName, "error", err)
			respondJSONError(w, http.StatusInternalServerError, "Storage configuration error")
			return
		}
		uploadMap[fileName] = presignedURL
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"session_code": code,
		"upload_urls":  uploadMap,
	})
}

// PATCH /api/sessions/{code} (RESTful Update - upserts image batches and optional email)
func (h *Handler) HandleUpdateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		respondJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	code := r.PathValue("code")
	if code == "" {
		respondJSONError(w, http.StatusBadRequest, "Session code is required")
		return
	}

	var req models.UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if err := h.Repo.UpdateImagesAndEmail(r.Context(), code, req.ImageURLs, req.Email, req.FileCount); err != nil {
		slog.Error("Failed to update session images and email", "session_code", code, "error", err)
		respondJSONError(w, http.StatusInternalServerError, "Internal database error")
		return
	}

	if req.Email != "" {
		scheme := "https"
		if h.Cfg.AppEnv == "local" {
			scheme = "http"
		}
		downloadLink := scheme + "://" + h.Cfg.BaseDomain + "/s/" + code
		subject := "Your Photobooth Pictures are Ready! 📸"

		var bodyBuf bytes.Buffer
		data := map[string]string{
			"DownloadLink": downloadLink,
		}
		if err := h.Tmpl.ExecuteTemplate(&bodyBuf, "email_session.html", data); err != nil {
			slog.Error("Failed to render email HTML template", "error", err)
		} else {
			if err := h.Mailer.Send(req.Email, subject, bodyBuf.String()); err != nil {
				slog.Error("Failed to dispatch session email", "session_code", code, "email", req.Email, "error", err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":       "success",
		"session_code": code,
		"message":      "Session updated successfully",
	})
}

// GET /s/{code} (RESTful Read - Gallery Web Page)
func (h *Handler) HandleGetGallery(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		h.renderNotFound(w)
		return
	}

	session, err := h.Repo.GetByCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrSessionNotFound) {
			h.renderNotFound(w)
			return
		}
		slog.Error("Database query error on gallery lookup", "session_code", code, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	freshURLs := make([]string, 0, len(session.ImageURLs))
	for _, raw := range session.ImageURLs {
		key, err := h.Presigner.ExtractObjectKey(raw)
		if err != nil {
			slog.Warn("Skipping invalid image URL", "raw_url", raw, "error", err)
			continue
		}
		signed, err := h.Presigner.GeneratePresignedDownloadURL(key, filepath.Base(key), 24*time.Hour)
		if err != nil {
			slog.Warn("Failed to presign download URL", "key", key, "error", err)
			continue
		}
		freshURLs = append(freshURLs, signed)
	}
	session.ImageURLs = freshURLs

	if time.Since(session.CreatedAt) > galleryLifetime {
		h.renderNotFound(w)
		return
	}
	session.DaysLeft = int(time.Until(session.CreatedAt.Add(galleryLifetime)).Hours() / 24)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "gallery.html", session); err != nil {
		slog.Error("Template execution error", "session_code", code, "error", err)
	}
}

// GET /api/sessions/{code} (JSON feed for live gallery polling)
func (h *Handler) HandleGetSession(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" {
		respondJSONError(w, http.StatusNotFound, "Session code is required")
		return
	}

	session, err := h.Repo.GetByCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, repository.ErrSessionNotFound) {
			respondJSONError(w, http.StatusNotFound, "Gallery not found")
			return
		}
		slog.Error("Database query error on session API", "session_code", code, "error", err)
		respondJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if time.Since(session.CreatedAt) > galleryLifetime {
		respondJSONError(w, http.StatusNotFound, "Gallery expired")
		return
	}

	urls := make([]string, 0, len(session.ImageURLs))
	for _, raw := range session.ImageURLs {
		key, err := h.Presigner.ExtractObjectKey(raw)
		if err != nil {
			continue
		}
		signed, err := h.Presigner.GeneratePresignedDownloadURL(key, filepath.Base(key), 24*time.Hour)
		if err != nil {
			continue
		}
		urls = append(urls, signed)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "success",
		"session_code": session.SessionCode,
		"image_urls":   urls,
		"file_count":   session.FileCount,
	})
}

// GET / (Root Home Page / Under Development)
func (h *Handler) HandleLanding(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.Tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		slog.Error("Landing page template execution error", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// Wildcard handler for unrecognized routes
func (h *Handler) HandleNotFound(w http.ResponseWriter, r *http.Request) {
	h.renderNotFound(w)
}

// renderNotFound serves the shared 404 page for unknown or expired galleries.
func (h *Handler) renderNotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	if err := h.Tmpl.ExecuteTemplate(w, "404.html", nil); err != nil {
		slog.Error("404 template execution error", "error", err)
	}
}
