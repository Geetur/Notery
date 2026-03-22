package handlers

import (
	"context"
	"fmt"
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

// ===== STRIPE ERROR REPORTING TESTS =====

func TestStripeConnect_CreateAccountError_SurfacesMessage(t *testing.T) {
	app := testApp(t)
	app.FrontendURL = "http://localhost:3000"
	app.Payment = &payment.MockService{
		CreateConnectedAccountFn: func(_ context.Context, _ string) (string, error) {
			return "", fmt.Errorf("stripe: create connected account: connect not enabled for this account")
		},
	}
	uid := seedUser(t, app.DB, "connect_err")

	w := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w, http.StatusInternalServerError)
	r := respJSON(t, w)
	errMsg, _ := r["error"].(string)
	if errMsg == "" {
		t.Fatal("expected error message in response")
	}
}

func TestStripeConnect_OnboardingLinkError_SurfacesMessage(t *testing.T) {
	app := testApp(t)
	app.FrontendURL = "http://localhost:3000"
	app.Payment = &payment.MockService{
		CreateOnboardingLinkFn: func(_ context.Context, _, _, _ string) (string, error) {
			return "", fmt.Errorf("stripe: account link expired or invalid")
		},
	}
	uid := seedUser(t, app.DB, "link_err")
	// Pre-set an account ID so we skip CreateConnectedAccount
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("stripe_account_id", "acct_link_err")

	w := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w, http.StatusInternalServerError)
	r := respJSON(t, w)
	errMsg, _ := r["error"].(string)
	if errMsg == "" {
		t.Fatal("expected error message in response")
	}
}

func TestStripeStatus_AccountCheckError_ReturnsCachedState(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{
		GetAccountStatusFn: func(_ context.Context, _ string) (bool, error) {
			return false, fmt.Errorf("stripe API error")
		},
	}
	uid := seedUser(t, app.DB, "status_err")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("stripe_account_id", "acct_status_err")

	w := serve("GET", "/me/stripe/status", "/me/stripe/status",
		nil, app.StripeStatus, authMW(uid))
	// Should still return 200 with cached state, not 500
	assertStatus(t, w, http.StatusOK)
	r := respJSON(t, w)
	if r["has_account"] != true {
		t.Fatal("expected has_account=true from cached state")
	}
}

func TestStripeConnect_SavesAccountID(t *testing.T) {
	app := testApp(t)
	app.Payment = &payment.MockService{}
	app.FrontendURL = "http://localhost:3000"
	uid := seedUser(t, app.DB, "save_acct")

	w := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w, http.StatusOK)

	// Verify account ID was persisted
	var user models.User
	app.DB.First(&user, uid)
	if user.StripeAccountID == "" {
		t.Fatal("StripeAccountID should be saved after connect")
	}

	// Second call should reuse existing account, not create new
	w2 := serve("POST", "/me/stripe/connect", "/me/stripe/connect",
		nil, app.StripeConnect, authMW(uid))
	assertStatus(t, w2, http.StatusOK)
	r2 := respJSON(t, w2)
	if r2["account_id"] != user.StripeAccountID {
		t.Fatal("expected same account ID on second connect call")
	}
}

func TestStripeRefreshLink_ErrorSurfacesMessage(t *testing.T) {
	app := testApp(t)
	app.FrontendURL = "http://localhost:3000"
	app.Payment = &payment.MockService{
		CreateOnboardingLinkFn: func(_ context.Context, _, _, _ string) (string, error) {
			return "", fmt.Errorf("stripe: link generation failed")
		},
	}
	uid := seedUser(t, app.DB, "refresh_err")
	app.DB.Model(&models.User{}).Where("id = ?", uid).Update("stripe_account_id", "acct_refresh_err")

	w := serve("POST", "/me/stripe/refresh-link", "/me/stripe/refresh-link",
		nil, app.StripeRefreshLink, authMW(uid))
	assertStatus(t, w, http.StatusInternalServerError)
	r := respJSON(t, w)
	errMsg, _ := r["error"].(string)
	if errMsg == "" {
		t.Fatal("expected error message in response")
	}
}
