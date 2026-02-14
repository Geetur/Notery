package email

import (
	"strings"
	"testing"
)

func TestMockMailer_Send(t *testing.T) {
	m := &MockMailer{}

	err := m.Send("user@example.com", "Test Subject", "<p>Hello</p>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(m.Sent) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(m.Sent))
	}
	if m.Sent[0].To != "user@example.com" {
		t.Fatalf("unexpected To: %s", m.Sent[0].To)
	}
	if m.Sent[0].Subject != "Test Subject" {
		t.Fatalf("unexpected Subject: %s", m.Sent[0].Subject)
	}
	if m.Sent[0].Body != "<p>Hello</p>" {
		t.Fatalf("unexpected Body: %s", m.Sent[0].Body)
	}
}

func TestMockMailer_MultipleEmails(t *testing.T) {
	m := &MockMailer{}

	_ = m.Send("a@test.com", "Sub1", "Body1")
	_ = m.Send("b@test.com", "Sub2", "Body2")
	_ = m.Send("c@test.com", "Sub3", "Body3")

	if len(m.Sent) != 3 {
		t.Fatalf("expected 3 sent emails, got %d", len(m.Sent))
	}
}

func TestLogMailer_Send(t *testing.T) {
	m := &LogMailer{}
	err := m.Send("user@example.com", "Test", "Body")
	if err != nil {
		t.Fatalf("LogMailer should never return error: %v", err)
	}
}

func TestNewMailer_NoSMTP_ReturnsLogMailer(t *testing.T) {
	m := NewMailer("", "", "", "", "")
	if _, ok := m.(*LogMailer); !ok {
		t.Fatal("expected LogMailer when SMTP not configured")
	}
}

func TestNewMailer_WithSMTP_ReturnsSMTPMailer(t *testing.T) {
	m := NewMailer("smtp.example.com", "587", "user", "pass", "noreply@example.com")
	if _, ok := m.(*SMTPMailer); !ok {
		t.Fatal("expected SMTPMailer when SMTP is configured")
	}
}

func TestNewMailer_HostOnlyNoUser_ReturnsLogMailer(t *testing.T) {
	m := NewMailer("smtp.example.com", "587", "", "", "")
	if _, ok := m.(*LogMailer); !ok {
		t.Fatal("expected LogMailer when SMTP user is empty")
	}
}

func TestVerificationEmail_ContainsToken(t *testing.T) {
	subject, body := VerificationEmail("https://api.notery.app", "abc123def456")

	if subject != "Verify your Notery account" {
		t.Fatalf("unexpected subject: %s", subject)
	}

	if !strings.Contains(body, "abc123def456") {
		t.Fatal("email body should contain the token")
	}

	if !strings.Contains(body, "https://api.notery.app/api/v1/auth/verify-email?token=abc123def456") {
		t.Fatal("email body should contain the full verification URL")
	}
}

func TestVerificationEmail_ContainsHTML(t *testing.T) {
	_, body := VerificationEmail("http://localhost:8080", "token123")

	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Fatal("email body should be HTML")
	}
	if !strings.Contains(body, "Welcome to Notery") {
		t.Fatal("email body should contain welcome text")
	}
	if !strings.Contains(body, "24 hours") {
		t.Fatal("email body should mention 24-hour expiry")
	}
}
