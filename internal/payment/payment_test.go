package payment

import (
	"context"
	"errors"
	"testing"
)

// TestServiceInterfaceCompliance verifies that both StripeService and MockService
// satisfy the Service interface at compile time.
func TestServiceInterfaceCompliance(t *testing.T) {
	// These assignments verify interface satisfaction at compile time.
	var _ Service = (*StripeService)(nil)
	var _ Service = (*MockService)(nil)
}

func TestEventTypeConstants(t *testing.T) {
	tests := []struct {
		eventType WebhookEventType
		want      string
	}{
		{EventPaymentSucceeded, "payment_intent.succeeded"},
		{EventPaymentFailed, "payment_intent.payment_failed"},
		{EventPaymentCanceled, "payment_intent.canceled"},
	}

	for _, tt := range tests {
		if string(tt.eventType) != tt.want {
			t.Errorf("WebhookEventType %q != expected %q", tt.eventType, tt.want)
		}
	}
}

func TestErrUnsupportedEvent(t *testing.T) {
	if ErrUnsupportedEvent == nil {
		t.Fatal("ErrUnsupportedEvent should not be nil")
	}
	if ErrUnsupportedEvent.Error() != "unsupported webhook event type" {
		t.Errorf("unexpected error message: %s", ErrUnsupportedEvent.Error())
	}
}

// ----- MockService Tests -----

func TestMockServiceDefaults(t *testing.T) {
	svc := &MockService{}
	ctx := context.Background()

	// CreatePaymentIntent with defaults
	result, err := svc.CreatePaymentIntent(ctx, CreateIntentParams{
		AmountCents:    999,
		Currency:       "usd",
		IdempotencyKey: "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PaymentIntentID != "pi_mock_test-key" {
		t.Errorf("expected PaymentIntentID 'pi_mock_test-key', got %q", result.PaymentIntentID)
	}
	if result.ClientSecret != "pi_mock_secret_test-key" {
		t.Errorf("expected ClientSecret 'pi_mock_secret_test-key', got %q", result.ClientSecret)
	}
	if result.Status != "requires_payment_method" {
		t.Errorf("expected Status 'requires_payment_method', got %q", result.Status)
	}
	if result.AmountCents != 999 {
		t.Errorf("expected AmountCents 999, got %d", result.AmountCents)
	}
	if result.Currency != "usd" {
		t.Errorf("expected Currency 'usd', got %q", result.Currency)
	}

	// RetrievePaymentIntent with defaults
	result, err = svc.RetrievePaymentIntent(ctx, "pi_existing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PaymentIntentID != "pi_existing" {
		t.Errorf("expected PaymentIntentID 'pi_existing', got %q", result.PaymentIntentID)
	}

	// VerifyWebhookSignature with defaults returns ErrUnsupportedEvent
	_, err = svc.VerifyWebhookSignature([]byte("test"), "sig")
	if !errors.Is(err, ErrUnsupportedEvent) {
		t.Errorf("expected ErrUnsupportedEvent, got %v", err)
	}
}

func TestMockServiceCallTracking(t *testing.T) {
	svc := &MockService{}
	ctx := context.Background()

	// Three create calls
	_, _ = svc.CreatePaymentIntent(ctx, CreateIntentParams{AmountCents: 100})
	_, _ = svc.CreatePaymentIntent(ctx, CreateIntentParams{AmountCents: 200})
	_, _ = svc.CreatePaymentIntent(ctx, CreateIntentParams{AmountCents: 300})

	if len(svc.CreateCalls) != 3 {
		t.Fatalf("expected 3 CreateCalls, got %d", len(svc.CreateCalls))
	}
	if svc.CreateCalls[0].AmountCents != 100 {
		t.Errorf("expected first call amount 100, got %d", svc.CreateCalls[0].AmountCents)
	}
	if svc.CreateCalls[2].AmountCents != 300 {
		t.Errorf("expected third call amount 300, got %d", svc.CreateCalls[2].AmountCents)
	}

	// Two retrieve calls
	_, _ = svc.RetrievePaymentIntent(ctx, "pi_1")
	_, _ = svc.RetrievePaymentIntent(ctx, "pi_2")

	if len(svc.RetrieveCalls) != 2 {
		t.Fatalf("expected 2 RetrieveCalls, got %d", len(svc.RetrieveCalls))
	}
	if svc.RetrieveCalls[0] != "pi_1" {
		t.Errorf("expected first retrieve call 'pi_1', got %q", svc.RetrieveCalls[0])
	}

	// One verify call
	_, _ = svc.VerifyWebhookSignature(nil, "")
	if svc.VerifyCalls != 1 {
		t.Errorf("expected 1 VerifyCall, got %d", svc.VerifyCalls)
	}
}

func TestMockServiceCustomBehavior(t *testing.T) {
	ctx := context.Background()

	// Custom CreatePaymentIntent
	svc := &MockService{
		CreateFn: func(_ context.Context, params CreateIntentParams) (*IntentResult, error) {
			return &IntentResult{
				PaymentIntentID: "pi_custom",
				ClientSecret:    "custom_secret",
				Status:          "succeeded",
			}, nil
		},
	}

	result, err := svc.CreatePaymentIntent(ctx, CreateIntentParams{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.PaymentIntentID != "pi_custom" {
		t.Errorf("expected 'pi_custom', got %q", result.PaymentIntentID)
	}

	// Custom VerifyWebhookSignature
	svc2 := &MockService{
		VerifyFn: func(payload []byte, sig string) (*WebhookEvent, error) {
			return &WebhookEvent{
				Type:            EventPaymentSucceeded,
				PaymentIntentID: "pi_webhook",
			}, nil
		},
	}

	event, err := svc2.VerifyWebhookSignature([]byte("test"), "sig_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != EventPaymentSucceeded {
		t.Errorf("expected EventPaymentSucceeded, got %q", event.Type)
	}
	if event.PaymentIntentID != "pi_webhook" {
		t.Errorf("expected 'pi_webhook', got %q", event.PaymentIntentID)
	}
}

func TestMockServiceErrorSimulation(t *testing.T) {
	ctx := context.Background()

	expectedErr := errors.New("stripe rate limit exceeded")

	svc := &MockService{
		CreateFn: func(_ context.Context, _ CreateIntentParams) (*IntentResult, error) {
			return nil, expectedErr
		},
	}

	_, err := svc.CreatePaymentIntent(ctx, CreateIntentParams{AmountCents: 999})
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestCreateIntentParamsMetadata(t *testing.T) {
	params := CreateIntentParams{
		OrderID:        42,
		AmountCents:    1999,
		Currency:       "usd",
		IdempotencyKey: "ik_abc",
		CustomerEmail:  "user@example.com",
		Metadata: map[string]string{
			"order_id": "42",
			"user_id":  "7",
		},
	}

	if params.OrderID != 42 {
		t.Errorf("unexpected OrderID: %d", params.OrderID)
	}
	if params.Metadata["order_id"] != "42" {
		t.Errorf("unexpected metadata order_id: %s", params.Metadata["order_id"])
	}
}
