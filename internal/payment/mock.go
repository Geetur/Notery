// mock.go — Configurable test double for the payment.Service interface.
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

	// Connect method overrides
	CreateConnectedAccountFn func(ctx context.Context, email string) (string, error)
	CreateOnboardingLinkFn   func(ctx context.Context, accountID, returnURL, refreshURL string) (string, error)
	GetAccountStatusFn       func(ctx context.Context, accountID string) (bool, error)

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
		AmountCents:     params.AmountCents,
		Currency:        params.Currency,
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

// --- Stripe Connect mock methods ---

// ConnectAccountFn overrides CreateConnectedAccount.
var _ Service = (*MockService)(nil) // compile-time check

// CreateConnectedAccount implements Service.
func (m *MockService) CreateConnectedAccount(ctx context.Context, email string) (string, error) {
	if m.CreateConnectedAccountFn != nil {
		return m.CreateConnectedAccountFn(ctx, email)
	}
	return "acct_mock_" + email, nil
}

// CreateOnboardingLink implements Service.
func (m *MockService) CreateOnboardingLink(ctx context.Context, accountID, returnURL, refreshURL string) (string, error) {
	if m.CreateOnboardingLinkFn != nil {
		return m.CreateOnboardingLinkFn(ctx, accountID, returnURL, refreshURL)
	}
	return "https://connect.stripe.com/mock/onboard/" + accountID, nil
}

// CreateTransfer implements Service.
func (m *MockService) CreateTransfer(_ context.Context, _ int64, _, destAccountID, _ string) (string, error) {
	return "tr_mock_" + destAccountID, nil
}

// GetAccountStatus implements Service.
func (m *MockService) GetAccountStatus(ctx context.Context, accountID string) (bool, error) {
	if m.GetAccountStatusFn != nil {
		return m.GetAccountStatusFn(ctx, accountID)
	}
	return true, nil
}
