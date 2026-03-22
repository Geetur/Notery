// stripe.go — Stripe payment provider implementation.
package payment

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/account"
	"github.com/stripe/stripe-go/v82/accountlink"
	"github.com/stripe/stripe-go/v82/paymentintent"
	"github.com/stripe/stripe-go/v82/transfer"
	"github.com/stripe/stripe-go/v82/webhook"
)

// StripeService implements the Service interface using Stripe.
type StripeService struct {
	webhookSecret string
}

// NewStripeService creates a new Stripe payment service.
// The secretKey authenticates all Stripe API calls (set globally via stripe.Key).
// The webhookSecret verifies incoming webhook signatures.
func NewStripeService(secretKey, webhookSecret string) *StripeService {
	stripe.Key = secretKey
	return &StripeService{
		webhookSecret: webhookSecret,
	}
}

// CreatePaymentIntent creates a Stripe PaymentIntent for the given order.
func (s *StripeService) CreatePaymentIntent(ctx context.Context, params CreateIntentParams) (*IntentResult, error) {
	piParams := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(params.AmountCents),
		Currency: stripe.String(params.Currency),
		AutomaticPaymentMethods: &stripe.PaymentIntentAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}
	piParams.Context = ctx

	// Attach metadata for webhook reconciliation
	if params.Metadata != nil {
		for k, v := range params.Metadata {
			piParams.AddMetadata(k, v)
		}
	}

	// Set idempotency key to prevent duplicate charges on retried requests
	if params.IdempotencyKey != "" {
		piParams.SetIdempotencyKey(params.IdempotencyKey)
	}

	// Set receipt email if available
	if params.CustomerEmail != "" {
		piParams.ReceiptEmail = stripe.String(params.CustomerEmail)
	}

	pi, err := paymentintent.New(piParams)
	if err != nil {
		return nil, fmt.Errorf("stripe: create payment intent: %w", err)
	}

	return &IntentResult{
		PaymentIntentID: pi.ID,
		ClientSecret:    pi.ClientSecret,
		Status:          string(pi.Status),
		AmountCents:     pi.Amount,
		Currency:        string(pi.Currency),
	}, nil
}

// RetrievePaymentIntent fetches an existing PaymentIntent from Stripe.
func (s *StripeService) RetrievePaymentIntent(ctx context.Context, paymentIntentID string) (*IntentResult, error) {
	params := &stripe.PaymentIntentParams{}
	params.Context = ctx

	pi, err := paymentintent.Get(paymentIntentID, params)
	if err != nil {
		return nil, fmt.Errorf("stripe: retrieve payment intent: %w", err)
	}

	return &IntentResult{
		PaymentIntentID: pi.ID,
		ClientSecret:    pi.ClientSecret,
		Status:          string(pi.Status),
		AmountCents:     pi.Amount,
		Currency:        string(pi.Currency),
	}, nil
}

// VerifyWebhookSignature validates and parses a Stripe webhook event.
// Only payment_intent.succeeded and payment_intent.payment_failed are handled;
// all other event types return ErrUnsupportedEvent.
func (s *StripeService) VerifyWebhookSignature(payload []byte, signature string) (*WebhookEvent, error) {
	event, err := webhook.ConstructEvent(payload, signature, s.webhookSecret)
	if err != nil {
		return nil, fmt.Errorf("stripe: webhook signature verification failed: %w", err)
	}

	switch event.Type {
	case "payment_intent.succeeded":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal payment_intent.succeeded: %w", err)
		}
		return &WebhookEvent{
			Type:            EventPaymentSucceeded,
			PaymentIntentID: pi.ID,
			AmountCents:     pi.Amount,
			Currency:        string(pi.Currency),
		}, nil

	case "payment_intent.payment_failed":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal payment_intent.payment_failed: %w", err)
		}
		failMsg := "Payment failed"
		if pi.LastPaymentError != nil {
			failMsg = pi.LastPaymentError.Msg
		}
		return &WebhookEvent{
			Type:            EventPaymentFailed,
			PaymentIntentID: pi.ID,
			AmountCents:     pi.Amount,
			Currency:        string(pi.Currency),
			FailureMessage:  failMsg,
		}, nil

	case "payment_intent.canceled":
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return nil, fmt.Errorf("stripe: unmarshal payment_intent.canceled: %w", err)
		}
		cancelMsg := "Payment was canceled"
		if pi.CancellationReason != "" {
			cancelMsg = "Canceled: " + string(pi.CancellationReason)
		}
		return &WebhookEvent{
			Type:            EventPaymentCanceled,
			PaymentIntentID: pi.ID,
			AmountCents:     pi.Amount,
			Currency:        string(pi.Currency),
			FailureMessage:  cancelMsg,
		}, nil

	default:
		return nil, ErrUnsupportedEvent
	}
}

// --- Stripe Connect (Payouts) ---

// CreateConnectedAccount creates a Stripe Express Connected Account for a creator.
func (s *StripeService) CreateConnectedAccount(ctx context.Context, email string) (string, error) {
	params := &stripe.AccountParams{
		Type:    stripe.String(string(stripe.AccountTypeExpress)),
		Email:   stripe.String(email),
		Country: stripe.String("US"),
		Capabilities: &stripe.AccountCapabilitiesParams{
			Transfers: &stripe.AccountCapabilitiesTransfersParams{
				Requested: stripe.Bool(true),
			},
		},
	}
	params.Context = ctx

	acct, err := account.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe: create connected account: %w", err)
	}
	return acct.ID, nil
}

// CreateOnboardingLink generates a Stripe Account Link for Express onboarding.
func (s *StripeService) CreateOnboardingLink(ctx context.Context, accountID, returnURL, refreshURL string) (string, error) {
	params := &stripe.AccountLinkParams{
		Account:    stripe.String(accountID),
		RefreshURL: stripe.String(refreshURL),
		ReturnURL:  stripe.String(returnURL),
		Type:       stripe.String("account_onboarding"),
	}
	params.Context = ctx

	link, err := accountlink.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe: create onboarding link: %w", err)
	}
	return link.URL, nil
}

// CreateTransfer sends funds to a creator's Connected Account.
func (s *StripeService) CreateTransfer(ctx context.Context, amountCents int64, currency, destAccountID, transferGroup string) (string, error) {
	params := &stripe.TransferParams{
		Amount:        stripe.Int64(amountCents),
		Currency:      stripe.String(currency),
		Destination:   stripe.String(destAccountID),
		TransferGroup: stripe.String(transferGroup),
	}
	params.Context = ctx

	tr, err := transfer.New(params)
	if err != nil {
		return "", fmt.Errorf("stripe: create transfer: %w", err)
	}
	return tr.ID, nil
}

// GetAccountStatus checks whether a Connected Account has charges_enabled.
func (s *StripeService) GetAccountStatus(ctx context.Context, accountID string) (bool, error) {
	params := &stripe.AccountParams{}
	params.Context = ctx

	acct, err := account.GetByID(accountID, params)
	if err != nil {
		return false, fmt.Errorf("stripe: get account status: %w", err)
	}
	return acct.ChargesEnabled, nil
}
