package payment

import "context"

// MockService is a configurable test double for the payment.Service interface.
// Each method can be overridden via function fields; default implementations
// return reasonable test values.
//
// Usage:
//
//	svc := &payment.MockService{
//	    CreateFn: func(ctx context.Context, p payment.CreateIntentParams) (*payment.IntentResult, error) {
//	        return &payment.IntentResult{PaymentIntentID: "pi_test"}, nil
//	    },
//	}
type MockService struct {
	// CreateFn overrides CreatePaymentIntent behaviour. If nil, a default result is returned.
	CreateFn func(ctx context.Context, params CreateIntentParams) (*IntentResult, error)
	// RetrieveFn overrides RetrievePaymentIntent behaviour. If nil, a default result is returned.
	RetrieveFn func(ctx context.Context, id string) (*IntentResult, error)
	// VerifyFn overrides VerifyWebhookSignature behaviour. If nil, ErrUnsupportedEvent is returned.
	VerifyFn func(payload []byte, sig string) (*WebhookEvent, error)

	// Tracking fields for test assertions.
	CreateCalls   []CreateIntentParams
	RetrieveCalls []string
	VerifyCalls   int
}

// CreatePaymentIntent implements Service.
func (m *MockService) CreatePaymentIntent(ctx context.Context, params CreateIntentParams) (*IntentResult, error) {
	m.CreateCalls = append(m.CreateCalls, params)
	if m.CreateFn != nil {
		return m.CreateFn(ctx, params)
	}
	return &IntentResult{
		PaymentIntentID: "pi_mock_" + params.IdempotencyKey,
		ClientSecret:    "pi_mock_secret_" + params.IdempotencyKey,
		Status:          "requires_payment_method",
	}, nil
}

// RetrievePaymentIntent implements Service.
func (m *MockService) RetrievePaymentIntent(ctx context.Context, id string) (*IntentResult, error) {
	m.RetrieveCalls = append(m.RetrieveCalls, id)
	if m.RetrieveFn != nil {
		return m.RetrieveFn(ctx, id)
	}
	return &IntentResult{
		PaymentIntentID: id,
		ClientSecret:    "pi_mock_secret_retrieved",
		Status:          "requires_payment_method",
	}, nil
}

// VerifyWebhookSignature implements Service.
func (m *MockService) VerifyWebhookSignature(payload []byte, sig string) (*WebhookEvent, error) {
	m.VerifyCalls++
	if m.VerifyFn != nil {
		return m.VerifyFn(payload, sig)
	}
	return nil, ErrUnsupportedEvent
}
