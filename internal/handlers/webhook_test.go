package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Geetur/Notery/internal/models"
	"github.com/Geetur/Notery/internal/payment"
)

// serveWebhook fires a POST request with a raw body and Stripe-Signature header.
func serveWebhook(app *App, body []byte, signature string) *httptest.ResponseRecorder {
	r := gin.New()
	r.POST("/webhooks/stripe", app.HandleStripeWebhook)

	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader(body))
	req.Header.Set("Stripe-Signature", signature)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ===== WEBHOOK HANDLER TESTS =====

func TestWebhook_NoPaymentProvider(t *testing.T) {
	app := testApp(t)
	// app.Payment is nil by default from testApp
	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusServiceUnavailable)
}

func TestWebhook_MissingSignature(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}

	r := gin.New()
	r.POST("/webhooks/stripe", app.HandleStripeWebhook)
	req := httptest.NewRequest("POST", "/webhooks/stripe", bytes.NewReader([]byte(`{}`)))
	// No Stripe-Signature header
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assertStatus(t, w, http.StatusBadRequest)
}

func TestWebhook_InvalidSignature(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return nil, payment.ErrUnsupportedEvent
		},
	}
	w := serveWebhook(app, []byte(`{}`), "bad_sig")
	// ErrUnsupportedEvent → 200 (acknowledge unsupported)
	assertStatus(t, w, http.StatusOK)
}

func TestWebhook_PaymentSucceeded_OrderNotFound(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentSucceeded,
				PaymentIntentID: "pi_nonexistent",
				AmountCents:     1000,
				Currency:        "usd",
			}, nil
		},
	}
	w := serveWebhook(app, []byte(`{}`), "sig_test")
	// Order not found → 200 (acknowledge, no retry)
	assertStatus(t, w, http.StatusOK)
}

func TestWebhook_PaymentSucceeded_Idempotent(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_idem")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderFulfilled,
		TotalCents:      1000,
		Currency:        "usd",
		PaymentIntentID: "pi_already_done",
		IdempotencyKey:  "idem1",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentSucceeded,
				PaymentIntentID: "pi_already_done",
				AmountCents:     1000,
				Currency:        "usd",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["already_processed"] != true {
		t.Fatal("expected already_processed=true for fulfilled order")
	}
}

func TestWebhook_PaymentSucceeded_AmountMismatch(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_mismatch")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderPending,
		TotalCents:      1000,
		Currency:        "usd",
		PaymentIntentID: "pi_mismatch",
		IdempotencyKey:  "idem2",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentSucceeded,
				PaymentIntentID: "pi_mismatch",
				AmountCents:     9999, // wrong amount
				Currency:        "usd",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	// Should NOT be fulfilled — amount mismatch
	if r["error"] == nil {
		t.Fatal("expected error about amount mismatch")
	}

	// Verify order is still pending
	var check models.Order
	app.DB.First(&check, order.ID)
	if check.Status != models.OrderPending {
		t.Fatalf("order should remain pending after amount mismatch, got %s", check.Status)
	}
}

func TestWebhook_PaymentSucceeded_CurrencyMismatch(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_curr")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderPending,
		TotalCents:      1000,
		Currency:        "usd",
		PaymentIntentID: "pi_currency",
		IdempotencyKey:  "idem3",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentSucceeded,
				PaymentIntentID: "pi_currency",
				AmountCents:     1000,
				Currency:        "eur", // wrong currency
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["error"] == nil {
		t.Fatal("expected error about currency mismatch")
	}
}

func TestWebhook_PaymentFailed_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_fail")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderPending,
		TotalCents:      500,
		Currency:        "usd",
		PaymentIntentID: "pi_fail",
		IdempotencyKey:  "idem4",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentFailed,
				PaymentIntentID: "pi_fail",
				AmountCents:     500,
				Currency:        "usd",
				FailureMessage:  "card_declined",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)

	var check models.Order
	app.DB.First(&check, order.ID)
	if check.Status != models.OrderFailed {
		t.Fatalf("expected order status failed, got %s", check.Status)
	}
	if check.FailureReason != "card_declined" {
		t.Fatalf("expected failure reason 'card_declined', got %q", check.FailureReason)
	}
}

func TestWebhook_PaymentFailed_Idempotent(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_fail_idem")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderFailed,
		TotalCents:      500,
		Currency:        "usd",
		PaymentIntentID: "pi_fail_idem",
		IdempotencyKey:  "idem5",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentFailed,
				PaymentIntentID: "pi_fail_idem",
				FailureMessage:  "card_declined",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["already_processed"] != true {
		t.Fatal("expected already_processed=true for already-failed order")
	}
}

func TestWebhook_PaymentFailed_FulfilledWins(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_fulfilled_wins")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderFulfilled,
		TotalCents:      500,
		Currency:        "usd",
		PaymentIntentID: "pi_fulfilled_wins",
		IdempotencyKey:  "idem6",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentFailed,
				PaymentIntentID: "pi_fulfilled_wins",
				FailureMessage:  "card_declined",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["ignored"] != true {
		t.Fatal("expected ignored=true — fulfillment should win over failure")
	}

	// Verify order is still fulfilled
	var check models.Order
	app.DB.First(&check, order.ID)
	if check.Status != models.OrderFulfilled {
		t.Fatalf("order should remain fulfilled, got %s", check.Status)
	}
}

func TestWebhook_PaymentCanceled_HappyPath(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_cancel")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderPending,
		TotalCents:      300,
		Currency:        "usd",
		PaymentIntentID: "pi_cancel",
		IdempotencyKey:  "idem7",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentCanceled,
				PaymentIntentID: "pi_cancel",
				FailureMessage:  "user_canceled",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)

	var check models.Order
	app.DB.First(&check, order.ID)
	if check.Status != models.OrderFailed {
		t.Fatalf("expected order status failed after cancel, got %s", check.Status)
	}
}

func TestWebhook_PaymentCanceled_FulfilledWins(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "buyer_cancel_wins")

	order := models.Order{
		UserID:          uid,
		Status:          models.OrderFulfilled,
		TotalCents:      300,
		Currency:        "usd",
		PaymentIntentID: "pi_cancel_fulfilled",
		IdempotencyKey:  "idem8",
	}
	app.DB.Create(&order)

	app.Payment = &payment.MockService{
		VerifyFn: func(payload []byte, sig string) (*payment.WebhookEvent, error) {
			return &payment.WebhookEvent{
				Type:            payment.EventPaymentCanceled,
				PaymentIntentID: "pi_cancel_fulfilled",
				FailureMessage:  "user_canceled",
			}, nil
		},
	}

	w := serveWebhook(app, []byte(`{}`), "sig_test")
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["ignored"] != true {
		t.Fatal("expected ignored=true — fulfillment wins over cancellation")
	}
}
