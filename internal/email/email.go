// Package email provides email sending capabilities for the Notery API.
//
// ARCHITECTURE:
//
//	The Mailer interface allows swapping implementations (SMTP for production,
//	MockMailer for tests). When SMTP is not configured, emails are logged to
//	stdout for local development.
//
//	Email verification tokens are generated server-side and sent as URL parameters.
//	The verification URL points to an API endpoint that validates the token.
package email

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// SMTP connection timeouts to prevent goroutine leaks and blocked handlers.
const (
	smtpDialTimeout = 10 * time.Second // TCP dial timeout
	smtpSendTimeout = 30 * time.Second // Overall send deadline (dial + auth + data)
)

// Mailer defines the interface for sending emails.
type Mailer interface {
	// Send sends an email to the given recipient.
	Send(to, subject, body string) error
}

// SMTPMailer sends emails via an SMTP server.
type SMTPMailer struct {
	Host string
	Port string
	User string
	Pass string
	From string
}

// loginAuth implements smtp.Auth using the LOGIN mechanism.
// Some providers (e.g. Outlook/Office365) require LOGIN instead of PLAIN.
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.TrimSpace(string(fromServer))
	switch strings.ToLower(prompt) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected LOGIN prompt: %q", prompt)
	}
}

// Send delivers an email via SMTP with connection timeouts to prevent blocking.
// Tries PLAIN auth first (works with most providers including Resend SMTP),
// falling back to LOGIN auth for providers that require it (e.g. Outlook/Office365).
func (m *SMTPMailer) Send(to, subject, body string) error {
	// Reject CRLF injection in header-interpolated fields.
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("invalid characters in email header fields")
	}

	addr := m.Host + ":" + m.Port

	msg := strings.Join([]string{
		"From: " + m.From,
		"To: " + to,
		"Subject: " + subject,
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
		"",
		body,
	}, "\r\n")

	// Try PLAIN auth first (most providers), fall back to LOGIN (Outlook/Office365).
	plainAuth := smtp.PlainAuth("", m.User, m.Pass, m.Host)
	err := m.sendWithTimeout(addr, plainAuth, to, []byte(msg))
	if err != nil {
		errLower := strings.ToLower(err.Error())
		if strings.Contains(errLower, "auth") || strings.Contains(errLower, "unrecognized") || strings.Contains(errLower, "plain") {
			log.Printf("[EMAIL] PLAIN auth failed, trying LOGIN: %v", err)
			loginA := &loginAuth{username: m.User, password: m.Pass}
			return m.sendWithTimeout(addr, loginA, to, []byte(msg))
		}
	}
	return err
}

// sendWithTimeout establishes an SMTP connection with dial and overall timeouts,
// preventing indefinite blocking when the SMTP server is slow or unreachable.
func (m *SMTPMailer) sendWithTimeout(addr string, auth smtp.Auth, to string, msg []byte) error {
	// Dial with timeout instead of blocking indefinitely
	conn, err := net.DialTimeout("tcp", addr, smtpDialTimeout)
	if err != nil {
		return fmt.Errorf("smtp dial: %w", err)
	}
	// Set an overall deadline for the entire SMTP session
	if err := conn.SetDeadline(time.Now().Add(smtpSendTimeout)); err != nil {
		conn.Close()
		return fmt.Errorf("smtp set deadline: %w", err)
	}

	// Create SMTP client on the raw connection
	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("smtp new client: %w", err)
	}
	defer c.Close()

	// STARTTLS if supported
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: m.Host}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}

	// Authenticate
	if err := c.Auth(auth); err != nil {
		return fmt.Errorf("smtp auth: %w", err)
	}

	// Set sender
	if err := c.Mail(m.From); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}

	// Set recipient
	if err := c.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}

	// Write message body
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("smtp data close: %w", err)
	}

	return c.Quit()
}

// LogMailer logs emails to stdout instead of sending them.
// Used when SMTP is not configured (local development).
type LogMailer struct{}

// Send logs the email content to stdout.
func (m *LogMailer) Send(to, subject, body string) error {
	log.Printf("[EMAIL] To: %s | Subject: %s | Body length: %d", to, subject, len(body))
	return nil
}

// ResendMailer sends emails via the Resend HTTP API (https://api.resend.com/emails).
// This bypasses SMTP port blocking on platforms like Railway that block outbound port 587.
type ResendMailer struct {
	APIKey string // Resend API key (same value as SMTP_PASS)
	From   string // Sender address (e.g. "noreply@yourdomain.com")
}

// resendRequest is the JSON payload for the Resend API.
type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

// resendErrorResponse captures Resend API error details.
type resendErrorResponse struct {
	StatusCode int    `json:"statusCode"`
	Message    string `json:"message"`
	Name       string `json:"name"`
}

// Send delivers an email via the Resend HTTP API.
func (m *ResendMailer) Send(to, subject, body string) error {
	log.Printf("[EMAIL] Sending via Resend API to %s | Subject: %s", to, subject)

	payload := resendRequest{
		From:    m.From,
		To:      []string{to},
		Subject: subject,
		HTML:    body,
	}
	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("resend marshal: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("resend request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+m.APIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("resend send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("[EMAIL] Resend API success (%d) for %s", resp.StatusCode, to)
		return nil
	}

	// Read error body for diagnostics
	errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	var resendErr resendErrorResponse
	if json.Unmarshal(errBody, &resendErr) == nil && resendErr.Message != "" {
		log.Printf("[EMAIL] Resend API error (%d): %s — %s", resp.StatusCode, resendErr.Name, resendErr.Message)
		return fmt.Errorf("resend API error %d: %s", resp.StatusCode, resendErr.Message)
	}
	log.Printf("[EMAIL] Resend API error (%d): %s", resp.StatusCode, string(errBody))
	return fmt.Errorf("resend API error %d: %s", resp.StatusCode, string(errBody))
}

// NewMailer creates a Mailer based on configuration.
// If SMTP host is smtp.resend.com, returns a ResendMailer (HTTP API) to bypass port blocking.
// Otherwise returns an SMTPMailer or a LogMailer for development.
func NewMailer(host, port, user, pass, from string) Mailer {
	if host == "" || user == "" {
		log.Println("SMTP not configured — emails will be logged to stdout (development mode)")
		return &LogMailer{}
	}

	// Resend HTTP API: bypasses SMTP port blocking on Railway/Render/etc.
	if strings.EqualFold(host, "smtp.resend.com") {
		log.Printf("Resend HTTP mailer configured (from %s)", from)
		return &ResendMailer{
			APIKey: pass, // Resend uses the SMTP password as the API key
			From:  from,
		}
	}

	log.Printf("SMTP mailer configured: %s:%s from %s", host, port, from)
	return &SMTPMailer{
		Host: host,
		Port: port,
		User: user,
		Pass: pass,
		From: from,
	}
}

// MockMailer records sent emails for testing.
type MockMailer struct {
	Sent []MockEmail
}

// MockEmail records a single sent email for testing assertions.
type MockEmail struct {
	To      string
	Subject string
	Body    string
}

// Send records the email instead of sending it.
func (m *MockMailer) Send(to, subject, body string) error {
	m.Sent = append(m.Sent, MockEmail{To: to, Subject: subject, Body: body})
	return nil
}

// VerificationEmail builds the HTML email body for email verification.
func VerificationEmail(baseURL, token string) (subject, body string) {
	subject = "Verify your Notery account"
	verifyURL := fmt.Sprintf("%s/api/v1/auth/verify-email?token=%s", baseURL, token)
	body = fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
  <h2>Welcome to Notery!</h2>
  <p>Please verify your email address by clicking the link below:</p>
  <p><a href="%s" style="display: inline-block; padding: 12px 24px; background: #2563eb; color: white; text-decoration: none; border-radius: 6px;">Verify Email</a></p>
  <p>Or copy this URL: <code>%s</code></p>
  <p>This link expires in 24 hours.</p>
  <p style="color: #666; font-size: 12px;">If you didn't create a Notery account, you can ignore this email.</p>
</body>
</html>`, verifyURL, verifyURL)
	return
}

// PasswordResetEmail builds the HTML email body for password reset.
func PasswordResetEmail(baseURL, token string) (subject, body string) {
	subject = "Reset your Notery password"
	resetURL := fmt.Sprintf("%s/reset-password?token=%s", baseURL, token)
	body = fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
  <h2>Password Reset</h2>
  <p>We received a request to reset your Notery account password.</p>
  <p><a href="%s" style="display: inline-block; padding: 12px 24px; background: #2563eb; color: white; text-decoration: none; border-radius: 6px;">Reset Password</a></p>
  <p>Or copy this URL: <code>%s</code></p>
  <p>This link expires in 1 hour. If you didn't request this, you can safely ignore this email.</p>
  <p style="color: #666; font-size: 12px;">Your password won't change until you create a new one using this link.</p>
</body>
</html>`, resetURL, resetURL)
	return
}
