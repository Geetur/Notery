package models

import "testing"

func TestVoteDirection_Constants(t *testing.T) {
	if VoteUp != "up" {
		t.Fatalf("VoteUp = %q, want up", VoteUp)
	}
	if VoteDown != "down" {
		t.Fatalf("VoteDown = %q, want down", VoteDown)
	}
}

func TestVoteDirection_Distinct(t *testing.T) {
	if VoteUp == VoteDown {
		t.Fatal("VoteUp and VoteDown must be distinct")
	}
}

func TestVote_ZeroValue(t *testing.T) {
	v := Vote{}
	if v.UserID != 0 {
		t.Fatalf("expected 0 UserID, got %d", v.UserID)
	}
	if v.Direction != "" {
		t.Fatalf("expected empty Direction, got %q", v.Direction)
	}
}

func TestOrderStatus_Constants(t *testing.T) {
	statuses := []OrderStatus{OrderPending, OrderPaid, OrderFulfilled, OrderFailed, OrderRefunded}
	seen := make(map[OrderStatus]bool)
	for _, s := range statuses {
		if seen[s] {
			t.Fatalf("duplicate order status: %s", s)
		}
		seen[s] = true
	}
}

func TestOrderStatus_Values(t *testing.T) {
	if OrderPending != "pending" {
		t.Fatalf("OrderPending = %q, want pending", OrderPending)
	}
	if OrderPaid != "paid" {
		t.Fatalf("OrderPaid = %q, want paid", OrderPaid)
	}
	if OrderFulfilled != "fulfilled" {
		t.Fatalf("OrderFulfilled = %q, want fulfilled", OrderFulfilled)
	}
}

func TestPurchase_TableName(t *testing.T) {
	p := Purchase{}
	if p.TableName() != "purchases" {
		t.Fatalf("expected table name 'purchases', got %q", p.TableName())
	}
}
