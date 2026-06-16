package agentui

import (
	"strings"
	"testing"
)

func TestNewPalette_NoColor(t *testing.T) {
	p := NewPalette(true)
	if p.Bold != "" || p.Dim != "" || p.Reset != "" || p.Red != "" ||
		p.Green != "" || p.Yellow != "" || p.Cyan != "" || p.Purple != "" {
		t.Fatalf("NewPalette(true) must return all-empty palette, got %+v", p)
	}
}

func TestNewPalette_Color(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	p := NewPalette(false)
	for name, v := range map[string]string{
		"Bold": p.Bold, "Dim": p.Dim, "Reset": p.Reset,
		"Red": p.Red, "Green": p.Green, "Yellow": p.Yellow,
		"Cyan": p.Cyan, "Purple": p.Purple,
	} {
		if !strings.HasPrefix(v, "\033[") {
			t.Errorf("palette.%s = %q, want ANSI CSI escape", name, v)
		}
	}
}

func TestNewPalette_HonoursNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	p := NewPalette(false) // explicit flag false; env should still suppress
	if p.Reset != "" {
		t.Fatalf("NO_COLOR=1 must suppress palette, got Reset=%q", p.Reset)
	}
}

func TestHex24_ValidColors(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	p := NewPalette(false)
	cases := []struct {
		hex  string
		want string
	}{
		{"#FF6B6B", "\033[38;2;255;107;107m"},
		{"#4ECDC4", "\033[38;2;78;205;196m"},
		{"#50fa7b", "\033[38;2;80;250;123m"},
		{"#ff5555", "\033[38;2;255;85;85m"},
		{"FF6B6B", "\033[38;2;255;107;107m"}, // no leading "#"
	}
	for _, tc := range cases {
		got := p.Hex24(tc.hex)
		if got != tc.want {
			t.Errorf("Hex24(%q) = %q, want %q", tc.hex, got, tc.want)
		}
	}
}

func TestHex24_NoColor(t *testing.T) {
	p := NewPalette(true)
	if got := p.Hex24("#FF6B6B"); got != "" {
		t.Errorf("Hex24 in NoColor mode must return empty, got %q", got)
	}
}

func TestHex24_Garbage(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	p := NewPalette(false)
	cases := []string{"", "#GGG", "FF", "FF6B6BZZ", "###"}
	for _, c := range cases {
		if got := p.Hex24(c); got != "" {
			t.Errorf("Hex24(%q) must return empty on garbage, got %q", c, got)
		}
	}
}
