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
	"fmt"
	"log"
	"net/smtp"
	"strings"
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

// Send delivers an email via SMTP.
// Uses LOGIN auth for providers that don't support PLAIN (e.g. Outlook/Office365),
// falling back to PLAIN auth for providers that support it.
func (m *SMTPMailer) Send(to, subject, body string) error {
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

	// Try LOGIN auth first (required by Outlook/Office365), then fall back to PLAIN.
	auth := &loginAuth{username: m.User, password: m.Pass}
	err := smtp.SendMail(addr, auth, m.From, []string{to}, []byte(msg))
	if err != nil && strings.Contains(err.Error(), "Unrecognized authentication type") {
		// Fall back to PLAIN auth for providers that don't support LOGIN
		plainAuth := smtp.PlainAuth("", m.User, m.Pass, m.Host)
		return smtp.SendMail(addr, plainAuth, m.From, []string{to}, []byte(msg))
	}
	return err
}

// LogMailer logs emails to stdout instead of sending them.
// Used when SMTP is not configured (local development).
type LogMailer struct{}

// Send logs the email content to stdout.
func (m *LogMailer) Send(to, subject, body string) error {
	log.Printf("[EMAIL] To: %s | Subject: %s | Body length: %d", to, subject, len(body))
	return nil
}

// NewMailer creates a Mailer based on configuration.
// Returns an SMTPMailer if SMTP is configured, or a LogMailer for development.
func NewMailer(host, port, user, pass, from string) Mailer {
	if host == "" || user == "" {
		log.Println("SMTP not configured — emails will be logged to stdout (development mode)")
		return &LogMailer{}
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
