package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"photobooth-backend/internal/models"

	"github.com/lib/pq"
)

var ErrSessionNotFound = errors.New("session not found")

// SessionRepository abstracts data access for photobooth sessions (Dependency Inversion Principle)
type SessionRepository interface {
	Create(ctx context.Context, sessionCode string) error
	UpdateFileCount(ctx context.Context, sessionCode string, count int) error
	UpdateImagesAndEmail(ctx context.Context, sessionCode string, imageURLs []string, email string, fileCount int) error
	GetByCode(ctx context.Context, sessionCode string) (*models.SessionRecord, error)
}

// PostgresSessionRepository implements SessionRepository for PostgreSQL
type PostgresSessionRepository struct {
	db *sql.DB
}

func NewPostgresSessionRepository(db *sql.DB) *PostgresSessionRepository {
	return &PostgresSessionRepository{db: db}
}

func (r *PostgresSessionRepository) Create(ctx context.Context, sessionCode string) error {
	query := `INSERT INTO photobooth_sessions (session_code, image_urls) VALUES ($1, $2)`
	_, err := r.db.ExecContext(ctx, query, sessionCode, pq.Array([]string{}))
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	return nil
}

func (r *PostgresSessionRepository) UpdateFileCount(ctx context.Context, sessionCode string, count int) error {
	query := `
        UPDATE photobooth_sessions
        SET file_count = $1
        WHERE session_code = $2
    `
	result, err := r.db.ExecContext(ctx, query, count, sessionCode)
	if err != nil {
		return fmt.Errorf("failed to update file count: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSessionNotFound
	}
	return nil
}

func (r *PostgresSessionRepository) UpdateImagesAndEmail(ctx context.Context, sessionCode string, imageURLs []string, email string, fileCount int) error {
	query := `
		UPDATE photobooth_sessions
		SET image_urls = ARRAY(SELECT DISTINCT unnest(image_urls || $1)),
			file_count = GREATEST(
				(SELECT count(*) FROM (SELECT DISTINCT unnest(image_urls || $1)) t),
				$4
			),
			email = COALESCE(NULLIF($2, ''), email)
		WHERE session_code = $3
	`
	_, err := r.db.ExecContext(ctx, query, pq.Array(imageURLs), email, sessionCode, fileCount)
	if err != nil {
		return fmt.Errorf("failed to update session images and email: %w", err)
	}
	return nil
}

func (r *PostgresSessionRepository) GetByCode(ctx context.Context, sessionCode string) (*models.SessionRecord, error) {
	query := `SELECT id, session_code, email, image_urls, file_count, created_at FROM photobooth_sessions WHERE session_code = $1`
	row := r.db.QueryRowContext(ctx, query, sessionCode)

	var session models.SessionRecord
	var imageURLsArray pq.StringArray

	err := row.Scan(&session.ID, &session.SessionCode, &session.Email, &imageURLsArray, &session.FileCount, &session.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to query session: %w", err)
	}

	session.ImageURLs = []string(imageURLsArray)
	return &session, nil
}
