// email_test.go — Tests for the email package: Mailer implementations and template builders.
package email

import (
	"strings"
	"testing"
)

// ===== MOCK MAILER =====

func TestMockMailer_Send(t *testing.T) {
	m := &MockMailer{}
	err := m.Send("user@example.com", "Hello", "<p>body</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Sent) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(m.Sent))
	}
	if m.Sent[0].To != "user@example.com" {
		t.Errorf("unexpected To: %s", m.Sent[0].To)
	}
	if m.Sent[0].Subject != "Hello" {
		t.Errorf("unexpected Subject: %s", m.Sent[0].Subject)
	}
	if m.Sent[0].Body != "<p>body</p>" {
		t.Errorf("unexpected Body: %s", m.Sent[0].Body)
	}
}

func TestMockMailer_MultipleSends(t *testing.T) {
	m := &MockMailer{}
	for i := 0; i < 5; i++ {
		_ = m.Send("user@example.com", "Subject", "Body")
	}
	if len(m.Sent) != 5 {
		t.Fatalf("expected 5 sent emails, got %d", len(m.Sent))
	}
}

// ===== LOG MAILER =====

func TestLogMailer_Send_NoError(t *testing.T) {
	m := &LogMailer{}
	err := m.Send("user@example.com", "Hello", "<p>body</p>")
	if err != nil {
		t.Fatalf("LogMailer.Send should never error: %v", err)
	}
}

// ===== NEW MAILER FACTORY =====

func TestNewMailer_NoSMTP_ReturnsLogMailer(t *testing.T) {
	m := NewMailer("", "", "", "", "")
	if _, ok := m.(*LogMailer); !ok {
		t.Fatalf("expected *LogMailer, got %T", m)
	}
}

func TestNewMailer_NoHost_ReturnsLogMailer(t *testing.T) {
	m := NewMailer("", "587", "user", "pass", "noreply@test.com")
	if _, ok := m.(*LogMailer); !ok {
		t.Fatalf("expected *LogMailer, got %T", m)
	}
}

func TestNewMailer_NoUser_ReturnsLogMailer(t *testing.T) {
	m := NewMailer("smtp.example.com", "587", "", "pass", "noreply@test.com")
	if _, ok := m.(*LogMailer); !ok {
		t.Fatalf("expected *LogMailer, got %T", m)
	}
}

func TestNewMailer_FullConfig_ReturnsSMTPMailer(t *testing.T) {
	m := NewMailer("smtp.example.com", "587", "user", "pass", "noreply@test.com")
	smtp, ok := m.(*SMTPMailer)
	if !ok {
		t.Fatalf("expected *SMTPMailer, got %T", m)
	}
	if smtp.Host != "smtp.example.com" {
		t.Errorf("unexpected Host: %s", smtp.Host)
	}
	if smtp.Port != "587" {
		t.Errorf("unexpected Port: %s", smtp.Port)
	}
	if smtp.From != "noreply@test.com" {
		t.Errorf("unexpected From: %s", smtp.From)
	}
}

// ===== VERIFICATION EMAIL TEMPLATE =====

func TestVerificationEmail_Subject(t *testing.T) {
	subject, _ := VerificationEmail("https://notery.app", "abc123")
	if subject != "Verify your Notery account" {
		t.Errorf("unexpected subject: %s", subject)
	}
}

func TestVerificationEmail_ContainsToken(t *testing.T) {
	_, body := VerificationEmail("https://notery.app", "my-secret-token")
	if !strings.Contains(body, "my-secret-token") {
		t.Error("body should contain the token")
	}
}

func TestVerificationEmail_ContainsBaseURL(t *testing.T) {
	_, body := VerificationEmail("https://notery.app", "abc123")
	if !strings.Contains(body, "https://notery.app") {
		t.Error("body should contain the base URL")
	}
}

func TestVerificationEmail_ContainsVerifyPath(t *testing.T) {
	_, body := VerificationEmail("https://notery.app", "abc123")
	expected := "https://notery.app/api/v1/auth/verify-email?token=abc123"
	if !strings.Contains(body, expected) {
		t.Errorf("body should contain verify URL: %s", expected)
	}
}

func TestVerificationEmail_IsHTML(t *testing.T) {
	_, body := VerificationEmail("https://notery.app", "abc123")
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("body should be an HTML document")
	}
}

func TestVerificationEmail_Contains24HExpiry(t *testing.T) {
	_, body := VerificationEmail("https://notery.app", "abc123")
	if !strings.Contains(body, "24 hours") {
		t.Error("body should mention 24 hour expiry")
	}
}

// ===== PASSWORD RESET EMAIL TEMPLATE =====

func TestPasswordResetEmail_Subject(t *testing.T) {
	subject, _ := PasswordResetEmail("https://notery.app", "rst456")
	if subject != "Reset your Notery password" {
		t.Errorf("unexpected subject: %s", subject)
	}
}

func TestPasswordResetEmail_ContainsToken(t *testing.T) {
	_, body := PasswordResetEmail("https://notery.app", "reset-token-xyz")
	if !strings.Contains(body, "reset-token-xyz") {
		t.Error("body should contain the reset token")
	}
}

func TestPasswordResetEmail_ContainsBaseURL(t *testing.T) {
	_, body := PasswordResetEmail("https://notery.app", "abc123")
	if !strings.Contains(body, "https://notery.app") {
		t.Error("body should contain the base URL")
	}
}

func TestPasswordResetEmail_ContainsResetPath(t *testing.T) {
	_, body := PasswordResetEmail("https://notery.app", "rst456")
	expected := "https://notery.app/reset-password?token=rst456"
	if !strings.Contains(body, expected) {
		t.Errorf("body should contain reset URL: %s", expected)
	}
}

func TestPasswordResetEmail_IsHTML(t *testing.T) {
	_, body := PasswordResetEmail("https://notery.app", "rst456")
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("body should be an HTML document")
	}
}

func TestPasswordResetEmail_Contains1HExpiry(t *testing.T) {
	_, body := PasswordResetEmail("https://notery.app", "rst456")
	if !strings.Contains(body, "1 hour") {
		t.Error("body should mention 1 hour expiry")
	}
}

// ===== MAILER INTERFACE COMPLIANCE =====

func TestMailerInterfaceCompliance(t *testing.T) {
	var _ Mailer = &SMTPMailer{}
	var _ Mailer = &LogMailer{}
	var _ Mailer = &MockMailer{}
}
