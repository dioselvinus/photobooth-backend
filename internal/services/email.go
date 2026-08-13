package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"time"

	"photobooth-backend/internal/config"
)

// EmailSender defines the core abstraction for email dispatching (Dependency Inversion Principle)
type EmailSender interface {
	Send(to string, subject string, htmlBody string) error
}

// --- Provider Registry (Open-Closed Principle) ---
type ProviderBuilder func(cfg *config.Config) (EmailSender, error)

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ProviderBuilder)
)

func init() {
	RegisterProvider("smtp", func(cfg *config.Config) (EmailSender, error) {
		return &SMTPSender{
			Host:     cfg.SMTPHost,
			Port:     cfg.SMTPPort,
			Username: cfg.SMTPUsername,
			Password: cfg.SMTPPassword,
			From:     cfg.SMTPFrom,
		}, nil
	})

	RegisterProvider("resend", func(cfg *config.Config) (EmailSender, error) {
		if cfg.ResendAPIKey == "" {
			slog.Warn("[Mailer] RESEND_API_KEY is empty; email sending via Resend will fail until configured.")
		}
		return &ResendSender{
			APIKey: cfg.ResendAPIKey,
			Domain: cfg.BaseDomain,
		}, nil
	})

	RegisterProvider("sendgrid", func(cfg *config.Config) (EmailSender, error) {
		if cfg.SendGridAPIKey == "" {
			slog.Warn("[Mailer] SENDGRID_API_KEY is empty; email sending via SendGrid will fail until configured.")
		}
		return &SendGridSender{
			APIKey: cfg.SendGridAPIKey,
			From:   cfg.SMTPFrom,
		}, nil
	})

	RegisterProvider("mailgun", func(cfg *config.Config) (EmailSender, error) {
		if cfg.MailgunAPIKey == "" || cfg.MailgunDomain == "" {
			slog.Warn("[Mailer] MAILGUN_API_KEY or MAILGUN_DOMAIN is empty.")
		}
		return &MailgunSender{
			APIKey: cfg.MailgunAPIKey,
			Domain: cfg.MailgunDomain,
			From:   cfg.SMTPFrom,
		}, nil
	})

	RegisterProvider("noop", func(cfg *config.Config) (EmailSender, error) {
		return &NoOpSender{}, nil
	})
	RegisterProvider("mock", func(cfg *config.Config) (EmailSender, error) {
		return &NoOpSender{}, nil
	})
}

// RegisterProvider registers a new email provider constructor.
// Open for extension: new providers can be registered without modifying existing code.
func RegisterProvider(name string, builder ProviderBuilder) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[strings.ToLower(name)] = builder
}

// NewMailerFromConfig builds the configured EmailSender instance dynamically from registry.
func NewMailerFromConfig(cfg *config.Config) (EmailSender, error) {
	registryMu.RLock()
	builder, ok := registry[strings.ToLower(cfg.EmailProvider)]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported email provider %q: available providers are %v", cfg.EmailProvider, AvailableProviders())
	}
	return builder(cfg)
}

// AvailableProviders returns a list of registered provider names.
func AvailableProviders() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	keys := make([]string, 0, len(registry))
	for k := range registry {
		keys = append(keys, k)
	}
	return keys
}

// --- 1. Mailpit / Standard & SSL SMTP Implementation ---
type SMTPSender struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
}

func (s *SMTPSender) Send(to string, subject string, htmlBody string) error {
	addr := fmt.Sprintf("%s:%s", s.Host, s.Port)
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=UTF-8;\r\n\r\n%s", s.From, to, subject, htmlBody)

	var auth smtp.Auth
	if s.Username != "" {
		auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
	}

	// Implicit SSL/TLS mode (Standard for SMTP Port 465)
	if s.Port == "465" {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         s.Host,
		}

		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("ssl tls dial failed: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.Host)
		if err != nil {
			return fmt.Errorf("smtp client creation failed: %w", err)
		}
		defer client.Quit()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth failed: %w", err)
			}
		}

		if err := client.Mail(s.From); err != nil {
			return fmt.Errorf("smtp mail command failed: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("smtp rcpt command failed: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("smtp data command failed: %w", err)
		}

		_, err = w.Write([]byte(msg))
		if err != nil {
			return fmt.Errorf("smtp write body failed: %w", err)
		}

		err = w.Close()
		if err != nil {
			return fmt.Errorf("smtp close data writer failed: %w", err)
		}

		slog.Info("Successfully sent email via SMTP SSL (Port 465)", "to", to, "host", s.Host)
		return nil
	}

	// Standard STARTTLS / Unencrypted SMTP (Port 587 / 25 / 1025)
	err := smtp.SendMail(addr, auth, s.From, []string{to}, []byte(msg))
	if err != nil {
		return fmt.Errorf("smtp send failed: %w", err)
	}
	slog.Info("Captured SMTP email", "to", to, "host", s.Host, "port", s.Port)
	return nil
}

// --- 2. Resend HTTP Implementation ---
type ResendSender struct {
	APIKey string
	Domain string
}

func (r *ResendSender) Send(to string, subject string, htmlBody string) error {
	payload := map[string]interface{}{
		"from":    "PicCorner <no-reply@" + r.Domain + ">",
		"to":      []string{to},
		"subject": subject,
		"html":    htmlBody,
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Authorization", "Bearer "+r.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("resend api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("resend api failed with status code: %v", resp.StatusCode)
	}

	slog.Info("Successfully sent email via Resend API", "to", to)
	return nil
}

// --- 3. SendGrid Implementation ---
type SendGridSender struct {
	APIKey string
	From   string
}

func (s *SendGridSender) Send(to string, subject string, htmlBody string) error {
	payload := map[string]interface{}{
		"personalizations": []map[string]interface{}{
			{
				"to": []map[string]string{{"email": to}},
			},
		},
		"from":    map[string]string{"email": s.From},
		"subject": subject,
		"content": []map[string]string{
			{"type": "text/html", "value": htmlBody},
		},
	}

	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequest("POST", "https://api.sendgrid.com/v3/mail/send", bytes.NewBuffer(body))
	if err != nil {
		return err
	}

	httpReq.Header.Set("Authorization", "Bearer "+s.APIKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("sendgrid api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sendgrid api failed with status code: %v", resp.StatusCode)
	}

	slog.Info("Successfully sent email via SendGrid API", "to", to)
	return nil
}

// --- 4. Mailgun Implementation ---
type MailgunSender struct {
	APIKey string
	Domain string
	From   string
}

func (m *MailgunSender) Send(to string, subject string, htmlBody string) error {
	endpoint := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", m.Domain)
	form := url.Values{}
	form.Set("from", m.From)
	form.Set("to", to)
	form.Set("subject", subject)
	form.Set("html", htmlBody)

	httpReq, err := http.NewRequest("POST", endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}

	httpReq.SetBasicAuth("api", m.APIKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("mailgun api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("mailgun api failed with status code: %v", resp.StatusCode)
	}

	slog.Info("Successfully sent email via Mailgun API", "to", to)
	return nil
}

// --- 5. NoOp / Mock Implementation ---
type NoOpSender struct{}

func (n *NoOpSender) Send(to string, subject string, htmlBody string) error {
	slog.Info("[NoOp Mailer] Email suppressed", "to", to, "subject", subject)
	return nil
}

// --- Async Bounded Email Dispatcher Worker Pool ---
type EmailJob struct {
	To       string
	Subject  string
	HTMLBody string
}

type AsyncEmailDispatcher struct {
	sender  EmailSender
	queue   chan EmailJob
	workers int
	wg      sync.WaitGroup
}

func NewAsyncEmailDispatcher(sender EmailSender, bufferSize int, workerCount int) *AsyncEmailDispatcher {
	d := &AsyncEmailDispatcher{
		sender:  sender,
		queue:   make(chan EmailJob, bufferSize),
		workers: workerCount,
	}
	d.startWorkerPool()
	return d
}

func (d *AsyncEmailDispatcher) startWorkerPool() {
	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go func(workerID int) {
			defer d.wg.Done()
			for job := range d.queue {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				done := make(chan error, 1)

				go func(j EmailJob) {
					done <- d.sender.Send(j.To, j.Subject, j.HTMLBody)
				}(job)

				select {
				case err := <-done:
					if err != nil {
						slog.Error("Failed to dispatch email", "worker_id", workerID, "to", job.To, "error", err)
					}
				case <-ctx.Done():
					slog.Warn("Timeout dispatching email", "worker_id", workerID, "to", job.To)
				}
				cancel()
			}
		}(i + 1)
	}
}

// Dispatch enqueues an email job without blocking HTTP request handler.
func (d *AsyncEmailDispatcher) Dispatch(job EmailJob) {
	select {
	case d.queue <- job:
	default:
		slog.Warn("Email queue full, dropping job", "capacity", cap(d.queue), "to", job.To)
	}
}

// Send fulfills EmailSender interface.
func (d *AsyncEmailDispatcher) Send(to string, subject string, htmlBody string) error {
	d.Dispatch(EmailJob{
		To:       to,
		Subject:  subject,
		HTMLBody: htmlBody,
	})
	return nil
}

// Stop gracefully drains remaining queued emails and terminates workers.
func (d *AsyncEmailDispatcher) Stop() {
	close(d.queue)
	d.wg.Wait()
	slog.Info("Async email worker pool stopped cleanly")
}
