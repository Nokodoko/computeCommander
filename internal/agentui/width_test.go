package agentui

import (
	"strings"
	"testing"
)

func TestVisibleLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"hello", 5},
		{"\033[31mhello\033[0m", 5},
		{"\033[1m\033[31mhello\033[0m", 5},
		{"a\033[31mb\033[0mc", 3},
		// Multi-byte UTF-8 (e.g. middle-dot · is 2 bytes, one rune).
		{"·", 1},
		{"a·b", 3},
	}
	for _, tc := range cases {
		got := VisibleLen(tc.in)
		if got != tc.want {
			t.Errorf("VisibleLen(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestTruncate_NoTrim(t *testing.T) {
	cases := []struct {
		in string
		w  int
	}{
		{"hello", 5},
		{"hello", 10},
		{"\033[31mhello\033[0m", 5},
	}
	for _, tc := range cases {
		got := Truncate(tc.in, tc.w)
		if got != tc.in {
			t.Errorf("Truncate(%q, %d) = %q, want unchanged", tc.in, tc.w, got)
		}
	}
}

func TestTruncate_BasicCut(t *testing.T) {
	got := Truncate("hello world", 5)
	if got != "hello" {
		t.Errorf("Truncate(\"hello world\", 5) = %q, want \"hello\"", got)
	}
}

func TestTruncate_ANSIBalanced(t *testing.T) {
	// SGR opened, content cut, must close with reset.
	got := Truncate("\033[31mhello world\033[0m", 5)
	if !strings.Contains(got, "hello") {
		t.Fatalf("Truncate result missing 'hello': %q", got)
	}
	if !strings.HasSuffix(got, "\033[0m") {
		t.Errorf("Truncate must append reset after cut, got %q", got)
	}
}

func TestTruncate_ZeroWidth(t *testing.T) {
	if got := Truncate("anything", 0); got != "" {
		t.Errorf("Truncate(_, 0) must return empty, got %q", got)
	}
	if got := Truncate("anything", -1); got != "" {
		t.Errorf("Truncate(_, -1) must return empty, got %q", got)
	}
}

func TestStripANSI(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"\033[31mred\033[0m", "red"},
		{"\033[1m\033[31mbold-red\033[0m text", "bold-red text"},
		{"", ""},
	}
	for _, tc := range cases {
		got := StripANSI(tc.in)
		if got != tc.want {
			t.Errorf("StripANSI(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
