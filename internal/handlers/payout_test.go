package handlers

import (
	"net/http"
	"testing"

	"github.com/Geetur/Notery/internal/models"
	"github.com/Geetur/Notery/internal/payment"
)

// ===== PAYOUT HANDLER TESTS =====

func TestStripeConnect_NoPaymentProvider(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "nopay")

	w := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w, http.StatusServiceUnavailable)
}

func TestStripeConnect_NewAccount(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	app.FrontendURL = "http://localhost:3000"
	uid := seedUser(t, app.DB, "creator_new")

	w := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["onboarding_url"] == nil || r["onboarding_url"] == "" {
		t.Fatal("expected onboarding_url in response")
	}
	if r["account_id"] == nil || r["account_id"] == "" {
		t.Fatal("expected account_id in response")
	}

	// Verify account ID was saved to user
	var user models.User
	app.DB.First(&user, uid)
	if user.StripeAccountID == "" {
		t.Fatal("expected StripeAccountID to be saved on user")
	}
}

func TestStripeConnect_ExistingAccount(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	app.FrontendURL = "http://localhost:3000"
	uid := seedUser(t, app.DB, "creator_existing")

	// Pre-set a Stripe account ID
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("stripe_account_id", "acct_existing")

	w := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["account_id"] != "acct_existing" {
		t.Fatalf("expected existing account ID, got %v", r["account_id"])
	}
}

func TestStripeStatus_NoAccount(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	uid := seedUser(t, app.DB, "status_noacct")

	w := serve("GET", "/me/stripe/status", "/me/stripe/status",
		nil, app.StripeStatus, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["has_account"] != false {
		t.Fatal("expected has_account=false for user without Stripe account")
	}
}

func TestStripeStatus_WithAccount(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	uid := seedUser(t, app.DB, "status_acct")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("stripe_account_id", "acct_test_status")

	w := serve("GET", "/me/stripe/status", "/me/stripe/status",
		nil, app.StripeStatus, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["has_account"] != true {
		t.Fatal("expected has_account=true")
	}
}

func TestStripeRefreshLink_NoPaymentProvider(t *testing.T) {
	app := testApp(t)
	uid := seedUser(t, app.DB, "refresh_nopay")

	w := serve("POST", "/me/stripe/refresh-link", "/me/stripe/refresh-link",
		nil, app.StripeRefreshLink, authMW(uid))
	assertStatus(t, w, http.StatusServiceUnavailable)
}

func TestStripeRefreshLink_NoAccount(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	uid := seedUser(t, app.DB, "refresh_noacct")

	w := serve("POST", "/me/stripe/refresh-link", "/me/stripe/refresh-link",
		nil, app.StripeRefreshLink, authMW(uid))
	assertStatus(t, w, http.StatusBadRequest)
}

func TestStripeRefreshLink_HappyPath(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	app.FrontendURL = "http://localhost:3000"
	uid := seedUser(t, app.DB, "refresh_ok")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("stripe_account_id", "acct_refresh")

	w := serve("POST", "/me/stripe/refresh-link", "/me/stripe/refresh-link",
		nil, app.StripeRefreshLink, authMW(uid))
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["onboarding_url"] == nil || r["onboarding_url"] == "" {
		t.Fatal("expected onboarding_url in response")
	}
}
