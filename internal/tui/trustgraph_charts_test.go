package tui

import (
	"strings"
	"testing"
)

func TestBarChart_Render_BasicOutput(t *testing.T) {
	bc := NewBarChart("Test", 10)
	entries := []BarEntry{
		{Label: "alpha", Value: 10},
		{Label: "beta", Value: 5},
		{Label: "gamma", Value: 1},
	}

	result := bc.Render(entries, 60, 20)

	if !strings.Contains(result, "alpha") {
		t.Errorf("expected 'alpha' in output, got: %s", result)
	}
	if !strings.Contains(result, "beta") {
		t.Errorf("expected 'beta' in output, got: %s", result)
	}
	if !strings.Contains(result, "gamma") {
		t.Errorf("expected 'gamma' in output, got: %s", result)
	}
	// Alpha should have the longest bar.
	lines := strings.Split(result, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
}

func TestBarChart_Render_EmptyEntries(t *testing.T) {
	bc := NewBarChart("Empty", 10)
	result := bc.Render(nil, 60, 20)

	if !strings.Contains(result, "no data") {
		t.Errorf("expected 'no data' for empty entries, got: %s", result)
	}
}

func TestBarChart_Render_MaxBarsLimit(t *testing.T) {
	bc := NewBarChart("Limited", 3)
	entries := []BarEntry{
		{Label: "a", Value: 10},
		{Label: "b", Value: 8},
		{Label: "c", Value: 6},
		{Label: "d", Value: 4},
		{Label: "e", Value: 2},
	}

	result := bc.Render(entries, 60, 20)

	// Should show 3 bars + overflow indicator.
	if !strings.Contains(result, "+2 more") {
		t.Errorf("expected overflow indicator, got: %s", result)
	}
	// Should NOT contain "d" or "e".
	if strings.Contains(result, "  d ") {
		t.Errorf("entry 'd' should be hidden, got: %s", result)
	}
}

func TestBarChart_Render_LongLabels(t *testing.T) {
	bc := NewBarChart("Long", 5)
	entries := []BarEntry{
		{Label: "this-is-a-very-long-entity-name-that-exceeds-limit", Value: 10},
	}

	result := bc.Render(entries, 60, 20)

	// Label should be truncated.
	if strings.Contains(result, "exceeds-limit") {
		t.Errorf("expected label truncation, got: %s", result)
	}
	if !strings.Contains(result, "..") {
		t.Errorf("expected '..' truncation marker, got: %s", result)
	}
}

func TestBarChart_Render_BlockCharacters(t *testing.T) {
	bc := NewBarChart("Blocks", 5)
	entries := []BarEntry{
		{Label: "node", Value: 10},
	}

	result := bc.Render(entries, 60, 20)

	// Should contain the default bar character (■ BLACK SQUARE U+25A0).
	if !strings.ContainsRune(result, '\u25A0') {
		t.Errorf("expected Unicode black square character in output, got: %s", result)
	}
}

func TestSparkline_PushAndRender(t *testing.T) {
	s := NewSparkline("Triples", 10)

	// Push some values.
	for i := 0; i < 5; i++ {
		s.Push(i * 10)
	}

	result := s.Render(40)

	if !strings.HasPrefix(result, "Triples ") {
		t.Errorf("expected title prefix, got: %s", result)
	}
	// Should contain at least one sparkline block character.
	hasBlock := false
	for _, r := range result {
		if r >= '\u2581' && r <= '\u2588' {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		t.Errorf("expected sparkline block characters, got: %s", result)
	}
	// Should end with the latest value.
	if !strings.HasSuffix(result, "40") {
		t.Errorf("expected latest value '40' at end, got: %s", result)
	}
}

func TestSparkline_EmptyRender(t *testing.T) {
	s := NewSparkline("Empty", 10)
	result := s.Render(40)

	if !strings.Contains(result, "no data") {
		t.Errorf("expected 'no data' for empty sparkline, got: %s", result)
	}
}

func TestSparkline_RingBufferWrap(t *testing.T) {
	s := NewSparkline("Wrap", 5)

	// Push more values than the window size.
	for i := 0; i < 8; i++ {
		s.Push(i)
	}

	// Snapshot should have 5 values: [3, 4, 5, 6, 7] (oldest first).
	snap := s.snapshot()
	if len(snap) != 5 {
		t.Fatalf("expected 5 values after wrap, got %d", len(snap))
	}
	if snap[0] != 3 || snap[4] != 7 {
		t.Errorf("expected [3..7], got %v", snap)
	}
}

func TestSparkline_AllEqualValues(t *testing.T) {
	s := NewSparkline("Flat", 10)
	for i := 0; i < 5; i++ {
		s.Push(42)
	}

	result := s.Render(40)

	// Should render without panic and produce consistent blocks.
	if !strings.Contains(result, "42") {
		t.Errorf("expected value '42' in output, got: %s", result)
	}
}

func TestBarChart_Render_HeightConstraint(t *testing.T) {
	bc := NewBarChart("Small", 0) // unlimited MaxBars
	entries := make([]BarEntry, 20)
	for i := range entries {
		entries[i] = BarEntry{Label: string(rune('a' + i)), Value: 20 - i}
	}

	// Only 5 lines of height available.
	result := bc.Render(entries, 60, 5)
	lines := strings.Split(result, "\n")

	// Should have at most 5 lines (4 bars + 1 overflow).
	if len(lines) > 5 {
		t.Errorf("expected at most 5 lines with height=5, got %d lines", len(lines))
	}
}
