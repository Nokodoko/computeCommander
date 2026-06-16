package agentui

import (
	"reflect"
	"testing"
)

func TestPadOrTruncate_Pad(t *testing.T) {
	got := PadOrTruncate([]string{"a", "b"}, 5)
	want := []string{"a", "b", "", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PadOrTruncate(['a','b'], 5) = %v, want %v", got, want)
	}
}

func TestPadOrTruncate_Truncate(t *testing.T) {
	got := PadOrTruncate([]string{"a", "b", "c", "d"}, 2)
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PadOrTruncate(['a','b','c','d'], 2) = %v, want %v", got, want)
	}
}

func TestPadOrTruncate_Exact(t *testing.T) {
	in := []string{"x", "y", "z"}
	got := PadOrTruncate(in, 3)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("PadOrTruncate exact = %v, want %v", got, in)
	}
	// Mutation safety: callers may not assume the returned slice shares
	// backing storage with the input.
	got[0] = "MUTATED"
	if in[0] == "MUTATED" {
		t.Errorf("PadOrTruncate must not return the caller's slice; in mutated to %v", in)
	}
}

func TestPadOrTruncate_ZeroOrNegative(t *testing.T) {
	if got := PadOrTruncate([]string{"a"}, 0); got != nil {
		t.Errorf("PadOrTruncate(_, 0) = %v, want nil", got)
	}
	if got := PadOrTruncate([]string{"a"}, -1); got != nil {
		t.Errorf("PadOrTruncate(_, -1) = %v, want nil", got)
	}
}

func TestClampWidth(t *testing.T) {
	got := ClampWidth([]string{"hello world", "abc", ""}, 5)
	want := []string{"hello", "abc", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClampWidth = %v, want %v", got, want)
	}
}

func TestDegradedMarker_Default(t *testing.T) {
	got := DegradedMarker("agents", 4)
	want := []string{"agents: unavailable", "", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DegradedMarker = %v, want %v", got, want)
	}
}

func TestDegradedMarker_WithReason(t *testing.T) {
	got := DegradedMarkerWithReason("evals", "no data", 3)
	want := []string{"evals: no data", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DegradedMarkerWithReason = %v, want %v", got, want)
	}
	got = DegradedMarkerWithReason("git", "not a repo", 5)
	want = []string{"git: not a repo", "", "", "", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DegradedMarkerWithReason git = %v, want %v", got, want)
	}
}

func TestDegradedMarker_ZeroLines(t *testing.T) {
	if got := DegradedMarker("agents", 0); got != nil {
		t.Errorf("DegradedMarker(_, 0) = %v, want nil", got)
	}
	if got := DegradedMarker("agents", -1); got != nil {
		t.Errorf("DegradedMarker(_, -1) = %v, want nil", got)
	}
}
