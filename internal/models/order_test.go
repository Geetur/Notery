package models

import (
	"testing"
)

func TestOrderStatusConstants(t *testing.T) {
	tests := []struct {
		status OrderStatus
		want   string
	}{
		{OrderPending, "pending"},
		{OrderPaid, "paid"},
		{OrderFulfilled, "fulfilled"},
		{OrderFailed, "failed"},
		{OrderRefunded, "refunded"},
	}

	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("OrderStatus constant: got %q, want %q", tt.status, tt.want)
		}
	}
}

func TestIsValidTransition(t *testing.T) {
	valid := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{OrderPending, OrderPaid},
		{OrderPending, OrderFailed},
		{OrderPaid, OrderFulfilled},
		{OrderPaid, OrderRefunded},
		{OrderFulfilled, OrderRefunded},
	}

	for _, tt := range valid {
		if !IsValidTransition(tt.from, tt.to) {
			t.Errorf("transition %s → %s should be valid", tt.from, tt.to)
		}
	}

	invalid := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		// Can't skip intermediate states
		{OrderPending, OrderFulfilled},
		{OrderPending, OrderRefunded},
		// Terminal states
		{OrderFailed, OrderPaid},
		{OrderFailed, OrderFulfilled},
		{OrderFailed, OrderPending},
		{OrderRefunded, OrderPaid},
		{OrderRefunded, OrderPending},
		{OrderRefunded, OrderFulfilled},
		// Can't go backwards
		{OrderFulfilled, OrderPending},
		{OrderFulfilled, OrderPaid},
		{OrderPaid, OrderPending},
	}

	for _, tt := range invalid {
		if IsValidTransition(tt.from, tt.to) {
			t.Errorf("transition %s → %s should be invalid", tt.from, tt.to)
		}
	}
}

func TestIsValidTransitionSelfTransition(t *testing.T) {
	statuses := []OrderStatus{
		OrderPending,
		OrderPaid,
		OrderFulfilled,
		OrderFailed,
		OrderRefunded,
	}

	for _, s := range statuses {
		if IsValidTransition(s, s) {
			t.Errorf("self-transition %s → %s should be invalid", s, s)
		}
	}
}

func TestOrderModelFieldDefaults(t *testing.T) {
	order := Order{}

	// Default status should be empty string (GORM default applied at DB level)
	if order.Status != "" {
		t.Errorf("expected empty default status, got %q", order.Status)
	}

	if order.TotalCents != 0 {
		t.Errorf("expected 0 TotalCents, got %d", order.TotalCents)
	}

	if order.Currency != "" {
		t.Errorf("expected empty default Currency (DB default), got %q", order.Currency)
	}

	if order.PaidAt != nil {
		t.Error("expected nil PaidAt")
	}

	if order.FailedAt != nil {
		t.Error("expected nil FailedAt")
	}

	if order.FailureReason != "" {
		t.Errorf("expected empty FailureReason, got %q", order.FailureReason)
	}

	if order.PaymentIntentID != "" {
		t.Errorf("expected empty PaymentIntentID, got %q", order.PaymentIntentID)
	}
}

func TestOrderItemRelationship(t *testing.T) {
	order := Order{
		UserID:     1,
		Status:     OrderPending,
		TotalCents: 1998,
		Currency:   "usd",
		Items: []OrderItem{
			{NoteID: 10, PriceCents: 999},
			{NoteID: 20, PriceCents: 999},
		},
	}

	if len(order.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(order.Items))
	}

	var sum int64
	for _, item := range order.Items {
		sum += item.PriceCents
	}
	if sum != order.TotalCents {
		t.Errorf("item sum %d != TotalCents %d", sum, order.TotalCents)
	}
}
