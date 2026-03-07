package blueprint

import (
	"testing"
)

func TestGenerateBlueprintID(t *testing.T) {
	id := GenerateBlueprintID()
	if len(id) < 4 {
		t.Fatal("expected non-empty ID")
	}
	if id[:3] != "bp-" {
		t.Fatalf("expected bp- prefix, got %q", id)
	}
	id2 := GenerateBlueprintID()
	if id == id2 {
		t.Fatal("expected unique IDs")
	}
}

func TestValidStatus(t *testing.T) {
	tests := []struct {
		status Status
		valid  bool
	}{
		{StatusPending, true},
		{StatusRunning, true},
		{StatusPassed, true},
		{StatusFailed, true},
		{StatusBlocked, true},
		{StatusCancelled, true},
		{"invalid", false},
		{"", false},
	}
	for _, tt := range tests {
		if ValidStatus(tt.status) != tt.valid {
			t.Errorf("ValidStatus(%q) = %v, want %v", tt.status, !tt.valid, tt.valid)
		}
	}
}

func TestValidTransition(t *testing.T) {
	tests := []struct {
		from, to Status
		valid    bool
	}{
		{StatusPending, StatusRunning, true},
		{StatusPending, StatusBlocked, true},
		{StatusPending, StatusCancelled, true},
		{StatusPending, StatusPassed, false},
		{StatusRunning, StatusPassed, true},
		{StatusRunning, StatusFailed, true},
		{StatusRunning, StatusCancelled, true},
		{StatusRunning, StatusPending, false},
		{StatusBlocked, StatusPending, true},
		{StatusBlocked, StatusCancelled, true},
		{StatusBlocked, StatusPassed, false},
		{StatusFailed, StatusPending, true},
		{StatusFailed, StatusCancelled, true},
		{StatusFailed, StatusRunning, false},
		{StatusPassed, StatusRunning, false},
	}
	for _, tt := range tests {
		if ValidTransition(tt.from, tt.to) != tt.valid {
			t.Errorf("ValidTransition(%s->%s) = %v, want %v", tt.from, tt.to, !tt.valid, tt.valid)
		}
	}
}

func TestGenerateRunID(t *testing.T) {
	id := GenerateRunID()
	if id[:4] != "bpr-" {
		t.Fatalf("expected bpr- prefix, got %q", id)
	}
}
