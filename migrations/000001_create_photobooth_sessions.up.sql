CREATE TABLE IF NOT EXISTS photobooth_sessions (
    id SERIAL PRIMARY KEY,
    session_code VARCHAR(32) NOT NULL UNIQUE,
    email VARCHAR(255),
    image_urls TEXT[] DEFAULT '{}',
    file_count INT DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_photobooth_sessions_session_code ON photobooth_sessions(session_code);
