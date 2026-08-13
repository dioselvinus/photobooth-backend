package models

import "time"

type CreateSessionRequest struct {
	DeviceID string `json:"device_id"`
}

type BatchUploadRequest struct {
	Files []string `json:"files"`
}

type UpdateSessionRequest struct {
	Email     string   `json:"email"`
	ImageURLs []string `json:"image_urls"`
	FileCount int      `json:"file_count"`
}

type SessionRecord struct {
	ID          string    `db:"id"`
	SessionCode string    `db:"session_code"`
	FileCount   int       `db:"file_count"`
	Email       *string   `db:"email"`
	ImageURLs   []string  `db:"image_urls"`
	CreatedAt   time.Time `db:"created_at"`
	ModifiedAt  time.Time `db:"modified_at"`
	DaysLeft    int
}
