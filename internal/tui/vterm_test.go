package tui

import (
	"strings"
	"testing"
)

func TestVTermBasicWrite(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "hello") {
		t.Errorf("first line should start with 'hello', got %q", lines[0])
	}
}

func TestVTermLineFeed(t *testing.T) {
	v := NewVTerm(10, 5)
	v.Write([]byte("line1\r\nline2\r\nline3"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "line1") {
		t.Errorf("line 0: expected 'line1', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "line2") {
		t.Errorf("line 1: expected 'line2', got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "line3") {
		t.Errorf("line 2: expected 'line3', got %q", lines[2])
	}
}

func TestVTermCarriageReturn(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello\rworld"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// "world" should overwrite "hello" on the same line.
	if !strings.HasPrefix(lines[0], "world") {
		t.Errorf("expected 'world' to overwrite, got %q", lines[0])
	}
}

func TestVTermCursorMovement(t *testing.T) {
	v := NewVTerm(20, 5)
	// Move cursor to row 3, col 5 (1-indexed) and write.
	v.Write([]byte("\033[3;5Htest"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// Row 2 (0-indexed), starting at col 4.
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[2], "test") {
		t.Errorf("row 2 should contain 'test', got %q", lines[2])
	}
}

func TestVTermClearScreen(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello\r\nworld"))
	v.Write([]byte("\033[2J"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if trimmed != "" {
			t.Errorf("line %d should be empty after clear, got %q", i, line)
		}
	}
}

func TestVTermClearLine(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello"))
	v.Write([]byte("\r\033[K"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	trimmed := strings.TrimRight(lines[0], " ")
	if trimmed != "" {
		t.Errorf("line 0 should be cleared, got %q", lines[0])
	}
}

func TestVTermSGRPreserved(t *testing.T) {
	v := NewVTerm(20, 3)
	// Write with green foreground.
	v.Write([]byte("\033[32mgreen\033[0m"))
	rendered := v.Render()
	if !strings.Contains(rendered, "\033[32m") {
		t.Error("rendered output should contain SGR green code")
	}
	if !strings.Contains(rendered, "green") {
		t.Error("rendered output should contain 'green' text")
	}
}

func TestVTermAlternateBuffer(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("main content"))
	// Switch to alternate buffer.
	v.Write([]byte("\033[?1049h"))
	v.Write([]byte("alt buffer"))
	rendered := v.Render()
	if !strings.Contains(rendered, "alt buffer") {
		t.Error("should show alt buffer content")
	}
	if strings.Contains(rendered, "main content") {
		t.Error("should NOT show main buffer content while in alt buffer")
	}
	// Switch back to main buffer.
	v.Write([]byte("\033[?1049l"))
	rendered = v.Render()
	if !strings.Contains(rendered, "main") {
		t.Error("should show main buffer content after leaving alt buffer")
	}
}

func TestVTermScrolling(t *testing.T) {
	v := NewVTerm(10, 3)
	// Write more lines than the terminal height to trigger scrolling.
	v.Write([]byte("line1\r\nline2\r\nline3\r\nline4"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// After scrolling, line1 should be gone, and we should see line2, line3, line4.
	if strings.Contains(lines[0], "line1") {
		t.Error("line1 should have scrolled off")
	}
	if !strings.HasPrefix(lines[0], "line2") {
		t.Errorf("line 0 should be 'line2', got %q", lines[0])
	}
}

func TestVTermResize(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello"))
	v.Resize(20, 5)
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines after resize, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "hello") {
		t.Error("content should be preserved after resize")
	}
}

func TestVTermBackspace(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello\b\b"))
	// Cursor should now be at position 3.
	v.Write([]byte("XY"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "helXY") {
		t.Errorf("expected 'helXY', got %q", lines[0])
	}
}

func TestVTermTab(t *testing.T) {
	v := NewVTerm(20, 3)
	v.Write([]byte("a\tb"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// 'a' at col 0, tab to col 8, 'b' at col 8.
	if len(lines[0]) < 9 || !strings.Contains(lines[0], "b") {
		t.Errorf("tab should move to column 8, got %q", lines[0])
	}
}

func TestVTermCursorHome(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello"))
	// ESC[H = move to home (1,1).
	v.Write([]byte("\033[H"))
	v.Write([]byte("X"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "Xello") {
		t.Errorf("expected 'Xello', got %q", lines[0])
	}
}

func TestVTermEraseToEndOfLine(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("helloworld"))
	// Move to column 5 and erase to end.
	v.Write([]byte("\033[1;6H\033[0K"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	trimmed := strings.TrimRight(lines[0], " ")
	if trimmed != "hello" {
		t.Errorf("expected 'hello' after erase to EOL, got %q", trimmed)
	}
}

func TestVTermDeleteChars(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("abcdefgh"))
	// Move to column 3 and delete 2 characters.
	v.Write([]byte("\033[1;3H\033[2P"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	trimmed := strings.TrimRight(lines[0], " ")
	if trimmed != "abefgh" {
		t.Errorf("expected 'abefgh' after delete, got %q", trimmed)
	}
}

func TestVTermInsertLines(t *testing.T) {
	v := NewVTerm(10, 4)
	v.Write([]byte("line1\r\nline2\r\nline3"))
	// Move to row 2 and insert a blank line.
	v.Write([]byte("\033[2;1H\033[1L"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "line1") {
		t.Errorf("line 0 should be 'line1', got %q", lines[0])
	}
	trimmed := strings.TrimRight(lines[1], " ")
	if trimmed != "" {
		t.Errorf("line 1 should be blank after insert, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "line2") {
		t.Errorf("line 2 should be 'line2', got %q", lines[2])
	}
}

func TestVTermOSCStripped(t *testing.T) {
	v := NewVTerm(10, 3)
	// OSC sequence for setting window title, terminated by BEL.
	v.Write([]byte("\033]0;My Title\007hello"))
	rendered := v.Render()
	if !strings.Contains(rendered, "hello") {
		t.Error("text after OSC should be visible")
	}
	if strings.Contains(rendered, "My Title") {
		t.Error("OSC content should not appear in rendered output")
	}
}

func TestVTermOSCWithST(t *testing.T) {
	v := NewVTerm(10, 3)
	// OSC terminated by ESC \.
	v.Write([]byte("\033]0;title\033\\hello"))
	rendered := v.Render()
	if !strings.Contains(rendered, "hello") {
		t.Error("text after OSC should be visible")
	}
}

func TestVTermCursorVisibilityIgnored(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("\033[?25l")) // hide cursor
	v.Write([]byte("hello"))
	v.Write([]byte("\033[?25h")) // show cursor
	rendered := v.Render()
	if !strings.Contains(rendered, "hello") {
		t.Error("text should render regardless of cursor visibility")
	}
}

func TestVTermAutoWrap(t *testing.T) {
	v := NewVTerm(5, 3)
	v.Write([]byte("abcdefgh"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "abcde") {
		t.Errorf("line 0 should be 'abcde', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "fgh") {
		t.Errorf("line 1 should be 'fgh', got %q", lines[1])
	}
}

func TestVTermSaveCursorRestore(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello"))
	v.Write([]byte("\0337"))     // DECSC: Save cursor
	v.Write([]byte("\033[2;1H")) // Move to row 2
	v.Write([]byte("world"))
	v.Write([]byte("\0338")) // DECRC: Restore cursor
	v.Write([]byte("!"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "hello!") {
		t.Errorf("line 0 should be 'hello!', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "world") {
		t.Errorf("line 1 should be 'world', got %q", lines[1])
	}
}

func TestVTermScrollRegion(t *testing.T) {
	v := NewVTerm(10, 5)
	v.Write([]byte("line1\r\nline2\r\nline3\r\nline4\r\nline5"))
	// Set scroll region to rows 2-4 (1-indexed).
	v.Write([]byte("\033[2;4r"))
	// Move to the last row in the region and add a new line.
	v.Write([]byte("\033[4;1H"))
	v.Write([]byte("\r\n"))
	v.Write([]byte("new"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// line1 should be unchanged (outside scroll region).
	if !strings.HasPrefix(lines[0], "line1") {
		t.Errorf("line 0 (outside region) should be 'line1', got %q", lines[0])
	}
	// line5 should be unchanged (outside scroll region).
	if !strings.HasPrefix(lines[4], "line5") {
		t.Errorf("line 4 (outside region) should be 'line5', got %q", lines[4])
	}
}

func TestVTermReset(t *testing.T) {
	v := NewVTerm(10, 3)
	v.Write([]byte("hello"))
	v.Write([]byte("\033c")) // RIS: Full reset
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if trimmed != "" {
			t.Errorf("line %d should be empty after reset, got %q", i, line)
		}
	}
}

func TestVTermUTF8MultiByteChars(t *testing.T) {
	v := NewVTerm(20, 3)
	// Write a braille spinner character (U+2800 series) — 3-byte UTF-8.
	// U+2807 = 0xE2, 0xA0, 0x87
	v.Write([]byte{0xE2, 0xA0, 0x87})
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// Should contain the braille character, NOT garbled individual bytes.
	if !strings.Contains(lines[0], "\u2807") {
		t.Errorf("expected braille char U+2807, got %q", lines[0])
	}
}

func TestVTermUTF8Emoji(t *testing.T) {
	v := NewVTerm(20, 3)
	// Write a 4-byte UTF-8 emoji (U+1F600 = grinning face).
	v.Write([]byte("hi "))
	v.Write([]byte{0xF0, 0x9F, 0x98, 0x80})
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.Contains(lines[0], "hi") {
		t.Errorf("expected 'hi' prefix, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "\U0001F600") {
		t.Errorf("expected emoji U+1F600, got %q", lines[0])
	}
}

func TestVTermUTF8SplitAcrossWrites(t *testing.T) {
	v := NewVTerm(20, 3)
	// Write a 3-byte UTF-8 char split across two Write calls.
	// U+2807 = 0xE2, 0xA0, 0x87
	v.Write([]byte{0xE2})
	v.Write([]byte{0xA0, 0x87})
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.Contains(lines[0], "\u2807") {
		t.Errorf("expected braille char from split writes, got %q", lines[0])
	}
}

func TestVTermWriteSatisfiesIOWriter(t *testing.T) {
	v := NewVTerm(10, 3)
	// Verify the Write method returns correct count.
	input := []byte("hello")
	n, err := v.Write(input)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write returned %d, expected %d", n, len(input))
	}
}

func TestVTermLineFeedPreservesColumn(t *testing.T) {
	// LF (\n) should only move the cursor down, NOT reset the column.
	// This is critical for TUI apps like Claude Code that position the
	// cursor with escape sequences and then use bare LF to move down.
	v := NewVTerm(20, 5)
	// Position cursor at column 5, then send a bare LF.
	v.Write([]byte("\033[1;6H"))  // Move to row 1, col 6 (1-indexed)
	v.Write([]byte("\n"))          // LF: move down, column stays at 5
	v.Write([]byte("X"))           // Should appear at row 2, col 5 (0-indexed)
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	// 'X' should be at column 5 of line 1 (0-indexed), not column 0.
	if len(lines[1]) < 6 || lines[1][5] != 'X' {
		t.Errorf("expected 'X' at column 5 of row 1, got line %q", lines[1])
	}
}

func TestVTermCRLFBehavior(t *testing.T) {
	// \r\n should move to column 0 and then down. This is the standard
	// sequence emitted by PTYs with ONLCR enabled.
	v := NewVTerm(20, 5)
	v.Write([]byte("hello\r\nworld"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	if !strings.HasPrefix(lines[0], "hello") {
		t.Errorf("line 0: expected 'hello', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "world") {
		t.Errorf("line 1: expected 'world' at column 0, got %q", lines[1])
	}
}

func TestVTermSGRIncrementalUpdate(t *testing.T) {
	// Verify that SGR parameters are applied incrementally:
	// setting bold+green, then resetting only fg, should keep bold.
	v := NewVTerm(20, 3)
	// Set bold + green fg.
	v.Write([]byte("\033[1;32mBOLD"))
	// Reset only fg (39), bold should remain.
	v.Write([]byte("\033[39mSTILL"))
	// Full reset.
	v.Write([]byte("\033[0mNORM"))
	rendered := v.Render()
	// "BOLD" should have bold + green.
	if !strings.Contains(rendered, "\033[1;32m") {
		t.Errorf("expected bold+green SGR for 'BOLD', got %q", rendered)
	}
	// "STILL" should have bold only (with a reset before it since fg changed).
	if !strings.Contains(rendered, "\033[1m") {
		t.Errorf("expected bold-only SGR for 'STILL', got %q", rendered)
	}
	// "NORM" should be default (no SGR or after a reset).
	if !strings.Contains(rendered, "NORM") {
		t.Errorf("expected 'NORM' in output, got %q", rendered)
	}
}

func TestVTermSGRBackgroundReset(t *testing.T) {
	// Verify background color does NOT leak to adjacent unstyled cells.
	v := NewVTerm(20, 3)
	// Write with blue background.
	v.Write([]byte("\033[44mBLUE\033[0mNORMAL"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// The "NORMAL" text should NOT be preceded by any active background.
	// After the reset, no SGR should be active for "NORMAL".
	// Check that \033[0m appears between "BLUE" and "NORMAL".
	if !strings.Contains(lines[0], "\033[0m") {
		t.Errorf("expected reset between styled and unstyled text, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "NORMAL") {
		t.Errorf("expected 'NORMAL' text in output, got %q", lines[0])
	}
}

func TestVTermSGRBgFgIndependent(t *testing.T) {
	// Setting bg should not affect fg, and vice versa.
	v := NewVTerm(30, 3)
	// Set red fg + blue bg.
	v.Write([]byte("\033[31;44mAB"))
	// Reset only bg (49), red fg should remain.
	v.Write([]byte("\033[49mCD"))
	// Reset only fg (39), no styling should remain.
	v.Write([]byte("\033[39mEF"))
	rendered := v.Render()
	// "AB" should have red fg + blue bg.
	if !strings.Contains(rendered, "\033[") {
		t.Errorf("expected SGR sequences in output, got %q", rendered)
	}
	// "EF" should be default — no SGR before it (or preceded by reset).
	if !strings.Contains(rendered, "EF") {
		t.Errorf("expected 'EF' in output, got %q", rendered)
	}
}

func TestVTermSGR256Color(t *testing.T) {
	v := NewVTerm(20, 3)
	// 256-color fg (color 208 = orange) + 256-color bg (color 17 = dark blue).
	v.Write([]byte("\033[38;5;208;48;5;17mHI\033[0m"))
	rendered := v.Render()
	if !strings.Contains(rendered, "38;5;208") {
		t.Errorf("expected 256-color fg in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "48;5;17") {
		t.Errorf("expected 256-color bg in output, got %q", rendered)
	}
}

func TestVTermSGRTrueColor(t *testing.T) {
	v := NewVTerm(20, 3)
	// True-color fg (RGB 255,128,0) + true-color bg (RGB 0,0,64).
	v.Write([]byte("\033[38;2;255;128;0;48;2;0;0;64mHI\033[0m"))
	rendered := v.Render()
	if !strings.Contains(rendered, "38;2;255;128;0") {
		t.Errorf("expected truecolor fg in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "48;2;0;0;64") {
		t.Errorf("expected truecolor bg in output, got %q", rendered)
	}
}

func TestVTermSGRNoLeakBetweenLines(t *testing.T) {
	// Background color on one line must NOT bleed to the next line.
	v := NewVTerm(10, 3)
	v.Write([]byte("\033[42m"))  // Green background.
	v.Write([]byte("line1"))
	v.Write([]byte("\033[0m"))   // Reset.
	v.Write([]byte("\r\nline2")) // Line 2 should be unstyled.
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")
	// line2 should have no SGR codes.
	if strings.Contains(lines[1], "\033[") {
		t.Errorf("line 2 should have no SGR codes, got %q", lines[1])
	}
}

func TestVTermSGRBrightColors(t *testing.T) {
	v := NewVTerm(20, 3)
	// Bright red fg (91) + bright cyan bg (106).
	v.Write([]byte("\033[91;106mHI\033[0m"))
	rendered := v.Render()
	// Bright red fg should be emitted as 91 (which is basic color 8+1=9, rendered as 90+1).
	if !strings.Contains(rendered, "\033[") {
		t.Errorf("expected SGR in output, got %q", rendered)
	}
	if !strings.Contains(rendered, "HI") {
		t.Errorf("expected 'HI' in output, got %q", rendered)
	}
}

func TestVTermSGRStyleCarryOverBlocked(t *testing.T) {
	// This is the core bug test: when a cell with bg color is followed by
	// an unstyled cell, the unstyled cell must NOT inherit the bg color.
	v := NewVTerm(10, 3)
	// Write 3 chars with bg color, then 3 without.
	v.Write([]byte("\033[44mAAA\033[0mBBB"))
	rendered := v.Render()
	lines := strings.Split(rendered, "\n")

	// Find the position of "BBB" in the rendered line.
	bbbIdx := strings.Index(lines[0], "BBB")
	if bbbIdx == -1 {
		t.Fatalf("expected 'BBB' in output, got %q", lines[0])
	}

	// Check that there is a reset sequence before "BBB".
	beforeBBB := lines[0][:bbbIdx]
	if !strings.Contains(beforeBBB, "\033[0m") {
		t.Errorf("expected \\033[0m before 'BBB' to clear bg color, got prefix %q", beforeBBB)
	}

	// Check that "BBB" is NOT immediately preceded by any bg color SGR
	// after the last reset.
	lastReset := strings.LastIndex(beforeBBB, "\033[0m")
	if lastReset >= 0 {
		afterReset := beforeBBB[lastReset+4:] // after "\033[0m"
		if strings.Contains(afterReset, "\033[4") {
			t.Errorf("bg color SGR found after reset before 'BBB': %q", afterReset)
		}
	}
}
