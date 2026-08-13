package main

import (
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"photobooth-backend/internal/config"
	"photobooth-backend/internal/database"
	"photobooth-backend/internal/handlers"
	"photobooth-backend/internal/middleware"
	"photobooth-backend/internal/repository"
	"photobooth-backend/internal/services"
	"photobooth-backend/internal/storage"

	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables. Prioritize production over local.
	if _, err := os.Stat(".env.production"); err == nil {
		_ = godotenv.Load(".env.production")
	} else {
		_ = godotenv.Load()
	}

	cfg := config.LoadConfig()

	// Configure Structured Logging (log/slog)
	var logger *slog.Logger
	if cfg.AppEnv == "production" || cfg.AppEnv == "staging" {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	}
	slog.SetDefault(logger)

	slog.Info("Starting Photobooth Backend", "env", cfg.AppEnv, "port", cfg.Port)

	db := database.NewPostgresDB(cfg.DatabaseURL)
	defer db.Close()

	// Construct StoragePresigner using provider factory (Open-Closed Principle)
	presigner, err := storage.NewStorageFromConfig(cfg)
	if err != nil {
		slog.Error("Failed to initialize storage provider", "provider", cfg.StorageProvider, "error", err)
		os.Exit(1)
	}
	if err := presigner.EnsureBucket(context.Background()); err != nil {
		slog.Error("Failed to ensure storage bucket", "provider", cfg.StorageProvider, "bucket", presigner.GetBucketName(), "error", err)
		os.Exit(1)
	}
	slog.Info("Storage bucket verified and ready", "provider", cfg.StorageProvider, "bucket", presigner.GetBucketName())

	// Construct EmailSender using provider factory (Open-Closed Principle)
	rawMailer, err := services.NewMailerFromConfig(cfg)
	if err != nil {
		slog.Error("Failed to initialize email provider", "provider", cfg.EmailProvider, "error", err)
		os.Exit(1)
	}
	slog.Info("Initialized email provider", "provider", cfg.EmailProvider)

	asyncMailer := services.NewAsyncEmailDispatcher(rawMailer, 100, 5)
	defer asyncMailer.Stop()

	tmpl, err := template.ParseFiles("templates/gallery.html", "templates/404.html", "templates/email_session.html")
	if err != nil {
		slog.Error("Failed to parse HTML templates", "error", err)
		os.Exit(1)
	}

	repo := repository.NewPostgresSessionRepository(db)
	h := handlers.NewHandler(repo, cfg, tmpl, presigner, asyncMailer)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/sessions", h.HandleCreateSession)
	mux.HandleFunc("POST /api/sessions/{code}/uploads", h.HandleGenerateBatchUploads)
	mux.HandleFunc("PATCH /api/sessions/{code}", h.HandleUpdateSession)
	mux.HandleFunc("GET /s/{code}", h.HandleGetGallery)
	mux.HandleFunc("GET /api/sessions/{code}", h.HandleGetSession)

	// Apply Middleware Stack: CORS + Rate Limiter (20 requests/sec, burst capacity of 50)
	rateLimiter := middleware.NewRateLimiter(20.0, 50.0)
	handlerWithMiddleware := middleware.CORSMiddleware(middleware.RateLimitMiddleware(rateLimiter)(mux))

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      handlerWithMiddleware,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Listen for OS interrupt / terminate signals for Graceful Shutdown
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("HTTP server running", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	<-stopChan
	slog.Info("Shutdown signal received, closing connections cleanly...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP server forced to shutdown", "error", err)
	} else {
		slog.Info("HTTP server stopped cleanly")
	}
}
