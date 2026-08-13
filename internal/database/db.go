package database

import (
	"database/sql"
	"log/slog"
	"time"

	"photobooth-backend/migrations"

	_ "github.com/lib/pq"
)

func NewPostgresDB(connStr string) *sql.DB {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		panic(err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		slog.Error("Failed to ping database", "error", err)
		panic(err)
	}

	slog.Info("Successfully connected to PostgreSQL database")

	if err := migrations.RunMigrations(db); err != nil {
		slog.Error("Failed to run database migrations", "error", err)
		panic(err)
	}

	return db
}


