// notification_test.go — Tests for notification model utilities.
package models

import "testing"

// ===== MILESTONE DETECTION TESTS =====

func TestShouldNotifyMilestone_CrossesFirst(t *testing.T) {
	m, ok := ShouldNotifyMilestone(4, 5)
	if !ok {
		t.Fatal("expected milestone notification at 5")
	}
	if m != 5 {
		t.Fatalf("expected milestone 5, got %d", m)
	}
}

func TestShouldNotifyMilestone_CrossesTen(t *testing.T) {
	m, ok := ShouldNotifyMilestone(9, 10)
	if !ok {
		t.Fatal("expected milestone notification at 10")
	}
	if m != 10 {
		t.Fatalf("expected milestone 10, got %d", m)
	}
}

func TestShouldNotifyMilestone_ExactlyAtMilestone(t *testing.T) {
	// Already at 10, new count is 10 — no notification
	_, ok := ShouldNotifyMilestone(10, 10)
	if ok {
		t.Fatal("should not notify when count doesn't cross milestone")
	}
}

func TestShouldNotifyMilestone_NoCrossing(t *testing.T) {
	_, ok := ShouldNotifyMilestone(6, 8)
	if ok {
		t.Fatal("should not notify when no milestone crossed")
	}
}

func TestShouldNotifyMilestone_JumpOverMultiple(t *testing.T) {
	// Jumps from 3 to 12 — should report first crossed milestone (5)
	m, ok := ShouldNotifyMilestone(3, 12)
	if !ok {
		t.Fatal("expected milestone notification")
	}
	if m != 5 {
		t.Fatalf("expected first crossed milestone 5, got %d", m)
	}
}

func TestShouldNotifyMilestone_HighMilestone(t *testing.T) {
	m, ok := ShouldNotifyMilestone(999, 1000)
	if !ok {
		t.Fatal("expected milestone notification at 1000")
	}
	if m != 1000 {
		t.Fatalf("expected milestone 1000, got %d", m)
	}
}

func TestShouldNotifyMilestone_BeyondAll(t *testing.T) {
	// Already past all milestones
	_, ok := ShouldNotifyMilestone(10000, 10001)
	if ok {
		t.Fatal("should not notify beyond last milestone")
	}
}

func TestShouldNotifyMilestone_ZeroToFive(t *testing.T) {
	m, ok := ShouldNotifyMilestone(0, 5)
	if !ok {
		t.Fatal("expected milestone notification from 0 to 5")
	}
	if m != 5 {
		t.Fatalf("expected milestone 5, got %d", m)
	}
}
