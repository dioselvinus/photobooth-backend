package config

import (
	"fmt"
	"os"
)

type Config struct {
	AppEnv          string
	Port            string
	DatabaseURL     string
	DirectDatabaseURL string
	BaseDomain      string
	EmailProvider   string
	SMTPHost        string
	SMTPPort        string
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	ResendAPIKey    string
	SendGridAPIKey  string
	MailgunAPIKey   string
	MailgunDomain   string
	StorageProvider string
	S3Endpoint      string
	S3Region        string
	S3AccessKey     string
	S3SecretKey     string
	S3Bucket        string
}

func LoadConfig() *Config {
	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "local"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/photobooth_local?sslmode=disable"
	}

	directDbURL := os.Getenv("DIRECT_DATABASE_URL")
	if directDbURL == "" {
		directDbURL = dbURL
	}

	baseDomain := os.Getenv("BASE_DOMAIN")
	if baseDomain == "" {
		baseDomain = "localhost:" + port
	}

	smtpHost := os.Getenv("SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "localhost"
	}
	smtpPort := os.Getenv("SMTP_PORT")
	if smtpPort == "" {
		smtpPort = "1025"
	}
	smtpFrom := os.Getenv("SMTP_FROM")
	if smtpFrom == "" {
		smtpFrom = "no-reply@piccorner.local"
	}

	emailProvider := os.Getenv("EMAIL_PROVIDER")
	if emailProvider == "" {
		if appEnv == "local" {
			emailProvider = "smtp"
		} else {
			emailProvider = "resend"
		}
	}

	storageProvider := os.Getenv("STORAGE_PROVIDER")
	if storageProvider == "" {
		if os.Getenv("R2_ACCOUNT_ID") != "" {
			storageProvider = "r2"
		} else {
			storageProvider = "s3"
		}
	}

	endpoint := os.Getenv("S3_ENDPOINT")
	r2AccountID := os.Getenv("R2_ACCOUNT_ID")
	if endpoint == "" && r2AccountID != "" {
		endpoint = fmt.Sprintf("https://%s.r2.cloudflarestorage.com", r2AccountID)
	}

	region := os.Getenv("S3_REGION")
	if region == "" {
		if storageProvider == "r2" || r2AccountID != "" {
			region = "auto"
		} else {
			region = "us-east-1"
		}
	}

	accessKey := os.Getenv("S3_ACCESS_KEY")
	if accessKey == "" {
		accessKey = os.Getenv("R2_ACCESS_KEY_ID")
	}

	secretKey := os.Getenv("S3_SECRET_KEY")
	if secretKey == "" {
		secretKey = os.Getenv("R2_SECRET_ACCESS_KEY")
	}

	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = os.Getenv("R2_BUCKET")
	}

	return &Config{
		AppEnv:            appEnv,
		Port:              port,
		DatabaseURL:       dbURL,
		DirectDatabaseURL: directDbURL,
		BaseDomain:        baseDomain,
		EmailProvider:   emailProvider,
		SMTPHost:        smtpHost,
		SMTPPort:        smtpPort,
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        smtpFrom,
		ResendAPIKey:    os.Getenv("RESEND_API_KEY"),
		SendGridAPIKey: os.Getenv("SENDGRID_API_KEY"),
		MailgunAPIKey:  os.Getenv("MAILGUN_API_KEY"),
		MailgunDomain:  os.Getenv("MAILGUN_DOMAIN"),
		StorageProvider: storageProvider,
		S3Endpoint:      endpoint,
		S3Region:        region,
		S3AccessKey:     accessKey,
		S3SecretKey:     secretKey,
		S3Bucket:        bucket,
	}
}
