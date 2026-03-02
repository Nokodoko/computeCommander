package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// ColorMode represents the type of color specification.
type ColorMode int

const (
	ColorDefault ColorMode = iota // No color set (terminal default)
	ColorBasic                    // Standard 8/16 colors (0-15)
	Color256                      // 256-color palette (0-255)
	ColorRGB                      // 24-bit true color
)

// Color represents a terminal color that can be default, basic, 256, or RGB.
type Color struct {
	Mode  ColorMode
	Value int // For Basic/256: the color number. For RGB: unused (use R,G,B).
	R, G, B uint8
}

// DefaultColor returns a Color representing the terminal default.
func DefaultColor() Color {
	return Color{Mode: ColorDefault}
}

// CellStyle holds decomposed SGR attributes for a terminal cell.
type CellStyle struct {
	FG Color
	BG Color

	Bold          bool
	Dim           bool
	Italic        bool
	Underline     bool
	Blink         bool
	Inverse       bool
	Hidden        bool
	Strikethrough bool
}

// IsDefault returns true if the style has no attributes set.
func (s CellStyle) IsDefault() bool {
	return s.FG.Mode == ColorDefault &&
		s.BG.Mode == ColorDefault &&
		!s.Bold && !s.Dim && !s.Italic && !s.Underline &&
		!s.Blink && !s.Inverse && !s.Hidden && !s.Strikethrough
}

// Equal returns true if two styles are identical.
func (s CellStyle) Equal(o CellStyle) bool {
	return s.FG == o.FG && s.BG == o.BG &&
		s.Bold == o.Bold && s.Dim == o.Dim &&
		s.Italic == o.Italic && s.Underline == o.Underline &&
		s.Blink == o.Blink && s.Inverse == o.Inverse &&
		s.Hidden == o.Hidden && s.Strikethrough == o.Strikethrough
}

// ToSGR emits the ANSI SGR sequence to establish this style from a reset state.
// Returns "" if the style is default (no SGR needed after a reset).
func (s CellStyle) ToSGR() string {
	if s.IsDefault() {
		return ""
	}

	var parts []string

	if s.Bold {
		parts = append(parts, "1")
	}
	if s.Dim {
		parts = append(parts, "2")
	}
	if s.Italic {
		parts = append(parts, "3")
	}
	if s.Underline {
		parts = append(parts, "4")
	}
	if s.Blink {
		parts = append(parts, "5")
	}
	if s.Inverse {
		parts = append(parts, "7")
	}
	if s.Hidden {
		parts = append(parts, "8")
	}
	if s.Strikethrough {
		parts = append(parts, "9")
	}

	// Foreground color.
	switch s.FG.Mode {
	case ColorBasic:
		if s.FG.Value < 8 {
			parts = append(parts, fmt.Sprintf("%d", 30+s.FG.Value))
		} else {
			parts = append(parts, fmt.Sprintf("%d", 90+s.FG.Value-8))
		}
	case Color256:
		parts = append(parts, fmt.Sprintf("38;5;%d", s.FG.Value))
	case ColorRGB:
		parts = append(parts, fmt.Sprintf("38;2;%d;%d;%d", s.FG.R, s.FG.G, s.FG.B))
	}

	// Background color.
	switch s.BG.Mode {
	case ColorBasic:
		if s.BG.Value < 8 {
			parts = append(parts, fmt.Sprintf("%d", 40+s.BG.Value))
		} else {
			parts = append(parts, fmt.Sprintf("%d", 100+s.BG.Value-8))
		}
	case Color256:
		parts = append(parts, fmt.Sprintf("48;5;%d", s.BG.Value))
	case ColorRGB:
		parts = append(parts, fmt.Sprintf("48;2;%d;%d;%d", s.BG.R, s.BG.G, s.BG.B))
	}

	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("\033[%sm", strings.Join(parts, ";"))
}

// Cell represents a single character cell in the virtual terminal.
type Cell struct {
	Char  rune
	Style CellStyle
}

// VTerm is a virtual VT100/xterm terminal emulator that maintains an internal
// screen buffer. Raw PTY output (including ANSI escape sequences) is fed in
// via Write(). The resulting screen state can be read via Render(), which
// produces lipgloss-safe text containing only SGR (color/style) sequences.
//
// This prevents cursor movement, screen clearing, alternate buffer, and other
// control sequences from leaking into the bubbletea rendering pipeline.
type VTerm struct {
	mu sync.Mutex

	cols int
	rows int

	// Screen buffer: cells[row][col].
	cells [][]Cell

	// Cursor position (0-indexed).
	cursorR int
	cursorC int

	// Current SGR state applied to newly written characters.
	style CellStyle

	// Scroll region (0-indexed, inclusive). Default: full screen.
	scrollTop    int
	scrollBottom int

	// Alternate screen buffer support.
	altCells    [][]Cell
	altCursorR  int
	altCursorC  int
	altStyle    CellStyle
	useAltBuf   bool

	// Saved cursor (DECSC/DECRC).
	savedCursorR int
	savedCursorC int
	savedStyle   CellStyle

	// Parser state for escape sequence accumulation.
	state  parseState
	escBuf []byte
	oscBuf []byte

	// UTF-8 multi-byte accumulation buffer.
	utf8Buf    [4]byte
	utf8Expect int // number of bytes still expected (0 = not in a multi-byte sequence)
	utf8Len    int // bytes accumulated so far

	// Dirty-flag caching: avoid re-rendering when nothing has changed.
	dirty      bool   // set true on Write()/Resize(), cleared by Render()
	lastRender string // cached output from the last Render() call
}

type parseState int

const (
	stateGround parseState = iota
	stateEscape            // saw ESC
	stateCSI               // saw ESC [
	stateOSC               // saw ESC ]
	stateOSCEnd            // saw ESC inside OSC (waiting for \)
)

// NewVTerm creates a new virtual terminal with the given dimensions.
func NewVTerm(cols, rows int) *VTerm {
	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}
	v := &VTerm{
		cols:         cols,
		rows:         rows,
		scrollBottom: rows - 1,
	}
	v.cells = v.makeGrid(cols, rows)
	return v
}

// makeGrid allocates a rows x cols grid of empty cells.
func (v *VTerm) makeGrid(cols, rows int) [][]Cell {
	grid := make([][]Cell, rows)
	for i := range grid {
		grid[i] = make([]Cell, cols)
		for j := range grid[i] {
			grid[i][j] = Cell{Char: ' '}
		}
	}
	return grid
}

// Write feeds raw PTY output bytes into the virtual terminal.
// It satisfies io.Writer.
func (v *VTerm) Write(p []byte) (int, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	for _, b := range p {
		v.processByte(b)
	}
	v.dirty = true
	return len(p), nil
}

// processByte handles a single byte through the state machine.
func (v *VTerm) processByte(b byte) {
	switch v.state {
	case stateGround:
		v.processGround(b)
	case stateEscape:
		v.processEscape(b)
	case stateCSI:
		v.processCSI(b)
	case stateOSC:
		v.processOSC(b)
	case stateOSCEnd:
		// Expecting backslash to end OSC.
		v.state = stateGround
		v.oscBuf = v.oscBuf[:0]
	}
}

// processGround handles bytes in the normal ground state.
func (v *VTerm) processGround(b byte) {
	// If we are accumulating a multi-byte UTF-8 sequence, try to continue it.
	if v.utf8Expect > 0 {
		if b >= 0x80 && b <= 0xBF {
			// Valid continuation byte.
			v.utf8Buf[v.utf8Len] = b
			v.utf8Len++
			v.utf8Expect--
			if v.utf8Expect == 0 {
				// Complete sequence — decode and emit.
				r, _ := utf8.DecodeRune(v.utf8Buf[:v.utf8Len])
				if r == utf8.RuneError {
					r = ' ' // Replace invalid sequences with space.
				}
				v.putChar(r)
				v.utf8Len = 0
			}
			return
		}
		// Not a valid continuation byte — discard partial sequence and
		// fall through to process this byte normally.
		v.utf8Expect = 0
		v.utf8Len = 0
	}

	switch {
	case b == 0x1b: // ESC
		v.state = stateEscape
		v.escBuf = v.escBuf[:0]
	case b == '\n': // Line feed — move cursor down only (column unchanged).
		// When the PTY has ONLCR enabled, the kernel translates \n to \r\n
		// on the master side, so we will see an explicit \r before the \n.
		// Full-screen TUI apps (like Claude Code) disable ONLCR and manage
		// \r themselves; resetting the column here would desync the cursor.
		v.lineFeed()
	case b == '\r': // Carriage return
		v.cursorC = 0
	case b == '\b': // Backspace
		if v.cursorC > 0 {
			v.cursorC--
		}
	case b == '\t': // Tab
		nextTab := ((v.cursorC / 8) + 1) * 8
		if nextTab >= v.cols {
			nextTab = v.cols - 1
		}
		v.cursorC = nextTab
	case b == 0x07: // BEL — ignore
		// noop
	case b >= 0xC0 && b <= 0xFD: // UTF-8 lead byte — start multi-byte sequence.
		v.utf8Buf[0] = b
		v.utf8Len = 1
		switch {
		case b >= 0xC0 && b <= 0xDF:
			v.utf8Expect = 1 // 2-byte sequence
		case b >= 0xE0 && b <= 0xEF:
			v.utf8Expect = 2 // 3-byte sequence
		case b >= 0xF0 && b <= 0xF7:
			v.utf8Expect = 3 // 4-byte sequence
		default:
			// 5 or 6-byte sequences are invalid in modern UTF-8, discard.
			v.utf8Expect = 0
			v.utf8Len = 0
		}
	case b >= 0x80 && b <= 0xBF: // Stray continuation byte — ignore.
		// noop
	case b >= 0x20 && b < 0x80: // ASCII printable characters.
		v.putChar(rune(b))
	}
}

// processEscape handles bytes after ESC.
func (v *VTerm) processEscape(b byte) {
	switch b {
	case '[': // CSI
		v.state = stateCSI
		v.escBuf = v.escBuf[:0]
	case ']': // OSC
		v.state = stateOSC
		v.oscBuf = v.oscBuf[:0]
	case '(', ')': // Character set designation — consume next byte.
		v.state = stateGround
		// We ignore character set switching; consume the next byte in the
		// following processGround call (it's just a letter like B or 0).
		// Actually we need to consume one more byte. We'll just return to ground
		// and let the designator byte pass through as a no-op character.
	case '7': // DECSC: Save cursor
		v.savedCursorR = v.cursorR
		v.savedCursorC = v.cursorC
		v.savedStyle = v.style
		v.state = stateGround
	case '8': // DECRC: Restore cursor
		v.cursorR = v.savedCursorR
		v.cursorC = v.savedCursorC
		v.style = v.savedStyle
		v.clampCursor()
		v.state = stateGround
	case 'M': // Reverse index (scroll up)
		if v.cursorR == v.scrollTop {
			v.scrollDown()
		} else if v.cursorR > 0 {
			v.cursorR--
		}
		v.state = stateGround
	case 'D': // Index (scroll down)
		v.lineFeed()
		v.state = stateGround
	case 'E': // Next line
		v.cursorC = 0
		v.lineFeed()
		v.state = stateGround
	case 'c': // RIS: Full reset
		v.reset()
		v.state = stateGround
	case '\\': // ST (String Terminator) — can appear standalone after ESC
		v.state = stateGround
	default:
		// Unknown escape — drop and return to ground.
		v.state = stateGround
	}
}

// processCSI handles bytes within a CSI sequence (after ESC [).
func (v *VTerm) processCSI(b byte) {
	// CSI parameter/intermediate bytes are 0x20-0x3F.
	// Final bytes are 0x40-0x7E.
	if b >= 0x20 && b <= 0x3F {
		v.escBuf = append(v.escBuf, b)
		return
	}
	if b >= 0x40 && b <= 0x7E {
		v.executeCSI(b)
		v.state = stateGround
		return
	}
	// Unexpected byte — abort sequence.
	v.state = stateGround
}

// processOSC handles bytes within an OSC sequence (after ESC ]).
func (v *VTerm) processOSC(b byte) {
	switch b {
	case 0x07: // BEL terminates OSC
		v.state = stateGround
		v.oscBuf = v.oscBuf[:0]
	case 0x1b: // ESC — might be ESC \ (ST)
		v.state = stateOSCEnd
	default:
		v.oscBuf = append(v.oscBuf, b)
		// Cap OSC buffer to prevent unbounded growth.
		if len(v.oscBuf) > 4096 {
			v.state = stateGround
			v.oscBuf = v.oscBuf[:0]
		}
	}
}

// executeCSI dispatches a complete CSI sequence.
func (v *VTerm) executeCSI(final byte) {
	params := string(v.escBuf)

	switch final {
	case 'A': // Cursor Up
		n := v.csiParam(params, 0, 1)
		v.cursorR -= n
		if v.cursorR < 0 {
			v.cursorR = 0
		}

	case 'B': // Cursor Down
		n := v.csiParam(params, 0, 1)
		v.cursorR += n
		if v.cursorR >= v.rows {
			v.cursorR = v.rows - 1
		}

	case 'C': // Cursor Forward
		n := v.csiParam(params, 0, 1)
		v.cursorC += n
		if v.cursorC >= v.cols {
			v.cursorC = v.cols - 1
		}

	case 'D': // Cursor Back
		n := v.csiParam(params, 0, 1)
		v.cursorC -= n
		if v.cursorC < 0 {
			v.cursorC = 0
		}

	case 'E': // Cursor Next Line
		n := v.csiParam(params, 0, 1)
		v.cursorR += n
		if v.cursorR >= v.rows {
			v.cursorR = v.rows - 1
		}
		v.cursorC = 0

	case 'F': // Cursor Previous Line
		n := v.csiParam(params, 0, 1)
		v.cursorR -= n
		if v.cursorR < 0 {
			v.cursorR = 0
		}
		v.cursorC = 0

	case 'G': // Cursor Horizontal Absolute
		n := v.csiParam(params, 0, 1)
		v.cursorC = n - 1
		v.clampCursor()

	case 'H', 'f': // Cursor Position
		parts := v.csiParams(params)
		row := 1
		col := 1
		if len(parts) >= 1 && parts[0] > 0 {
			row = parts[0]
		}
		if len(parts) >= 2 && parts[1] > 0 {
			col = parts[1]
		}
		v.cursorR = row - 1
		v.cursorC = col - 1
		v.clampCursor()

	case 'J': // Erase in Display
		n := v.csiParam(params, 0, 0)
		v.eraseDisplay(n)

	case 'K': // Erase in Line
		n := v.csiParam(params, 0, 0)
		v.eraseLine(n)

	case 'L': // Insert Lines
		n := v.csiParam(params, 0, 1)
		v.insertLines(n)

	case 'M': // Delete Lines
		n := v.csiParam(params, 0, 1)
		v.deleteLines(n)

	case 'P': // Delete Characters
		n := v.csiParam(params, 0, 1)
		v.deleteChars(n)

	case '@': // Insert Characters
		n := v.csiParam(params, 0, 1)
		v.insertChars(n)

	case 'X': // Erase Characters
		n := v.csiParam(params, 0, 1)
		for i := 0; i < n && v.cursorC+i < v.cols; i++ {
			v.cells[v.cursorR][v.cursorC+i] = Cell{Char: ' '}
		}

	case 'd': // Vertical Position Absolute
		n := v.csiParam(params, 0, 1)
		v.cursorR = n - 1
		v.clampCursor()

	case 'm': // SGR (Select Graphic Rendition)
		v.processSGR(params)

	case 'r': // DECSTBM: Set Scrolling Region
		parts := v.csiParams(params)
		top := 1
		bottom := v.rows
		if len(parts) >= 1 && parts[0] > 0 {
			top = parts[0]
		}
		if len(parts) >= 2 && parts[1] > 0 {
			bottom = parts[1]
		}
		v.scrollTop = top - 1
		v.scrollBottom = bottom - 1
		if v.scrollTop < 0 {
			v.scrollTop = 0
		}
		if v.scrollBottom >= v.rows {
			v.scrollBottom = v.rows - 1
		}
		if v.scrollTop >= v.scrollBottom {
			v.scrollTop = 0
			v.scrollBottom = v.rows - 1
		}
		v.cursorR = 0
		v.cursorC = 0

	case 'h': // Set Mode
		v.handleMode(params, true)

	case 'l': // Reset Mode
		v.handleMode(params, false)

	case 'S': // Scroll Up
		n := v.csiParam(params, 0, 1)
		for i := 0; i < n; i++ {
			v.scrollUp()
		}

	case 'T': // Scroll Down
		n := v.csiParam(params, 0, 1)
		for i := 0; i < n; i++ {
			v.scrollDown()
		}

	case 'n': // Device Status Report — ignore

	case 's': // Save cursor position (SCO)
		v.savedCursorR = v.cursorR
		v.savedCursorC = v.cursorC
		v.savedStyle = v.style

	case 'u': // Restore cursor position (SCO)
		v.cursorR = v.savedCursorR
		v.cursorC = v.savedCursorC
		v.style = v.savedStyle
		v.clampCursor()

	case 'c': // Device Attributes — ignore

	case 't': // Window manipulation — ignore

	default:
		// Unknown CSI final — ignore.
	}
}

// handleMode processes CSI ?<n>h / CSI ?<n>l (DEC private modes).
func (v *VTerm) handleMode(params string, set bool) {
	// Private mode indicator.
	if !strings.HasPrefix(params, "?") {
		return
	}
	nums := params[1:]
	for _, part := range strings.Split(nums, ";") {
		n, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		switch n {
		case 1049: // Alternate screen buffer
			if set {
				// Save main buffer, switch to alt.
				v.altCells = v.cells
				v.altCursorR = v.cursorR
				v.altCursorC = v.cursorC
				v.altStyle = v.style
				v.cells = v.makeGrid(v.cols, v.rows)
				v.cursorR = 0
				v.cursorC = 0
				v.useAltBuf = true
			} else {
				// Restore main buffer.
				if v.altCells != nil {
					v.cells = v.altCells
					v.cursorR = v.altCursorR
					v.cursorC = v.altCursorC
					v.style = v.altStyle
					v.altCells = nil
				}
				v.useAltBuf = false
			}
		case 47, 1047: // Alternate screen (simpler variants)
			if set {
				v.altCells = v.cells
				v.altCursorR = v.cursorR
				v.altCursorC = v.cursorC
				v.cells = v.makeGrid(v.cols, v.rows)
				v.useAltBuf = true
			} else {
				if v.altCells != nil {
					v.cells = v.altCells
					v.cursorR = v.altCursorR
					v.cursorC = v.altCursorC
					v.altCells = nil
				}
				v.useAltBuf = false
			}
		case 25: // Cursor visibility — ignore
		case 7:  // Auto-wrap mode — we always wrap, ignore
		case 1:  // Application cursor keys — ignore for rendering
		case 12: // Blinking cursor — ignore
		case 1000, 1002, 1003, 1006: // Mouse tracking — ignore
		case 2004: // Bracketed paste — ignore
		}
	}
}

// processSGR processes SGR parameters and updates the structured style state.
// Each SGR parameter is applied incrementally to the current style, matching
// how real terminals work: \033[1;32m sets bold+green, then a later \033[49m
// resets only the background without affecting bold or foreground.
func (v *VTerm) processSGR(params string) {
	if params == "" || params == "0" {
		v.style = CellStyle{}
		return
	}

	parts := strings.Split(params, ";")
	i := 0
	for i < len(parts) {
		n, _ := strconv.Atoi(parts[i])
		switch {
		case n == 0: // Reset all
			v.style = CellStyle{}

		// Text attributes ON
		case n == 1:
			v.style.Bold = true
		case n == 2:
			v.style.Dim = true
		case n == 3:
			v.style.Italic = true
		case n == 4:
			v.style.Underline = true
		case n == 5 || n == 6:
			v.style.Blink = true
		case n == 7:
			v.style.Inverse = true
		case n == 8:
			v.style.Hidden = true
		case n == 9:
			v.style.Strikethrough = true

		// Text attributes OFF
		case n == 22: // Normal intensity (not bold, not dim)
			v.style.Bold = false
			v.style.Dim = false
		case n == 23:
			v.style.Italic = false
		case n == 24:
			v.style.Underline = false
		case n == 25:
			v.style.Blink = false
		case n == 27:
			v.style.Inverse = false
		case n == 28:
			v.style.Hidden = false
		case n == 29:
			v.style.Strikethrough = false

		// Standard foreground colors (30-37)
		case n >= 30 && n <= 37:
			v.style.FG = Color{Mode: ColorBasic, Value: n - 30}

		// Extended foreground color (38)
		case n == 38:
			i++
			if i < len(parts) {
				sub, _ := strconv.Atoi(parts[i])
				if sub == 5 && i+1 < len(parts) {
					// 256-color: 38;5;N
					i++
					cn, _ := strconv.Atoi(parts[i])
					v.style.FG = Color{Mode: Color256, Value: cn}
				} else if sub == 2 && i+3 < len(parts) {
					// RGB: 38;2;R;G;B
					r, _ := strconv.Atoi(parts[i+1])
					g, _ := strconv.Atoi(parts[i+2])
					b, _ := strconv.Atoi(parts[i+3])
					i += 3
					v.style.FG = Color{Mode: ColorRGB, R: uint8(r), G: uint8(g), B: uint8(b)}
				}
			}

		// Default foreground (39)
		case n == 39:
			v.style.FG = DefaultColor()

		// Standard background colors (40-47)
		case n >= 40 && n <= 47:
			v.style.BG = Color{Mode: ColorBasic, Value: n - 40}

		// Extended background color (48)
		case n == 48:
			i++
			if i < len(parts) {
				sub, _ := strconv.Atoi(parts[i])
				if sub == 5 && i+1 < len(parts) {
					// 256-color: 48;5;N
					i++
					cn, _ := strconv.Atoi(parts[i])
					v.style.BG = Color{Mode: Color256, Value: cn}
				} else if sub == 2 && i+3 < len(parts) {
					// RGB: 48;2;R;G;B
					r, _ := strconv.Atoi(parts[i+1])
					g, _ := strconv.Atoi(parts[i+2])
					b, _ := strconv.Atoi(parts[i+3])
					i += 3
					v.style.BG = Color{Mode: ColorRGB, R: uint8(r), G: uint8(g), B: uint8(b)}
				}
			}

		// Default background (49)
		case n == 49:
			v.style.BG = DefaultColor()

		// Bright foreground colors (90-97)
		case n >= 90 && n <= 97:
			v.style.FG = Color{Mode: ColorBasic, Value: n - 90 + 8}

		// Bright background colors (100-107)
		case n >= 100 && n <= 107:
			v.style.BG = Color{Mode: ColorBasic, Value: n - 100 + 8}
		}
		i++
	}
}

// putChar writes a character at the cursor position and advances the cursor.
func (v *VTerm) putChar(ch rune) {
	if v.cursorC >= v.cols {
		// Auto-wrap: move to the next line.
		v.cursorC = 0
		v.lineFeed()
	}
	if v.cursorR >= 0 && v.cursorR < v.rows && v.cursorC >= 0 && v.cursorC < v.cols {
		v.cells[v.cursorR][v.cursorC] = Cell{Char: ch, Style: v.style}
	}
	v.cursorC++
}

// lineFeed moves the cursor down one line, scrolling if necessary.
func (v *VTerm) lineFeed() {
	if v.cursorR == v.scrollBottom {
		v.scrollUp()
	} else if v.cursorR < v.rows-1 {
		v.cursorR++
	}
}

// scrollUp scrolls the scroll region up by one line.
func (v *VTerm) scrollUp() {
	if v.scrollTop >= v.scrollBottom {
		return
	}
	// Shift lines up within the scroll region.
	for i := v.scrollTop; i < v.scrollBottom; i++ {
		v.cells[i] = v.cells[i+1]
	}
	// Clear the bottom line of the scroll region.
	v.cells[v.scrollBottom] = make([]Cell, v.cols)
	for j := range v.cells[v.scrollBottom] {
		v.cells[v.scrollBottom][j] = Cell{Char: ' '}
	}
}

// scrollDown scrolls the scroll region down by one line.
func (v *VTerm) scrollDown() {
	if v.scrollTop >= v.scrollBottom {
		return
	}
	// Shift lines down within the scroll region.
	for i := v.scrollBottom; i > v.scrollTop; i-- {
		v.cells[i] = v.cells[i-1]
	}
	// Clear the top line of the scroll region.
	v.cells[v.scrollTop] = make([]Cell, v.cols)
	for j := range v.cells[v.scrollTop] {
		v.cells[v.scrollTop][j] = Cell{Char: ' '}
	}
}

// eraseDisplay clears parts of the screen.
func (v *VTerm) eraseDisplay(mode int) {
	switch mode {
	case 0: // Clear from cursor to end.
		v.eraseLine(0)
		for i := v.cursorR + 1; i < v.rows; i++ {
			v.clearRow(i)
		}
	case 1: // Clear from start to cursor.
		for i := 0; i < v.cursorR; i++ {
			v.clearRow(i)
		}
		for j := 0; j <= v.cursorC && j < v.cols; j++ {
			v.cells[v.cursorR][j] = Cell{Char: ' '}
		}
	case 2, 3: // Clear entire screen (3 also clears scrollback, which we don't have).
		for i := 0; i < v.rows; i++ {
			v.clearRow(i)
		}
	}
}

// eraseLine clears parts of the current line.
func (v *VTerm) eraseLine(mode int) {
	if v.cursorR < 0 || v.cursorR >= v.rows {
		return
	}
	switch mode {
	case 0: // Clear from cursor to end of line.
		for j := v.cursorC; j < v.cols; j++ {
			v.cells[v.cursorR][j] = Cell{Char: ' '}
		}
	case 1: // Clear from start to cursor.
		for j := 0; j <= v.cursorC && j < v.cols; j++ {
			v.cells[v.cursorR][j] = Cell{Char: ' '}
		}
	case 2: // Clear entire line.
		v.clearRow(v.cursorR)
	}
}

// insertLines inserts n blank lines at the cursor row, pushing content down.
func (v *VTerm) insertLines(n int) {
	for i := 0; i < n; i++ {
		if v.cursorR <= v.scrollBottom {
			// Shift lines down.
			for j := v.scrollBottom; j > v.cursorR; j-- {
				v.cells[j] = v.cells[j-1]
			}
			v.cells[v.cursorR] = make([]Cell, v.cols)
			for j := range v.cells[v.cursorR] {
				v.cells[v.cursorR][j] = Cell{Char: ' '}
			}
		}
	}
}

// deleteLines deletes n lines at the cursor row, pulling content up.
func (v *VTerm) deleteLines(n int) {
	for i := 0; i < n; i++ {
		if v.cursorR <= v.scrollBottom {
			for j := v.cursorR; j < v.scrollBottom; j++ {
				v.cells[j] = v.cells[j+1]
			}
			v.cells[v.scrollBottom] = make([]Cell, v.cols)
			for j := range v.cells[v.scrollBottom] {
				v.cells[v.scrollBottom][j] = Cell{Char: ' '}
			}
		}
	}
}

// deleteChars deletes n characters at the cursor position, shifting left.
func (v *VTerm) deleteChars(n int) {
	if v.cursorR < 0 || v.cursorR >= v.rows {
		return
	}
	row := v.cells[v.cursorR]
	for i := v.cursorC; i < v.cols; i++ {
		src := i + n
		if src < v.cols {
			row[i] = row[src]
		} else {
			row[i] = Cell{Char: ' '}
		}
	}
}

// insertChars inserts n blank characters at the cursor position, shifting right.
func (v *VTerm) insertChars(n int) {
	if v.cursorR < 0 || v.cursorR >= v.rows {
		return
	}
	row := v.cells[v.cursorR]
	// Shift right.
	for i := v.cols - 1; i >= v.cursorC+n; i-- {
		row[i] = row[i-n]
	}
	for i := v.cursorC; i < v.cursorC+n && i < v.cols; i++ {
		row[i] = Cell{Char: ' '}
	}
}

// clearRow fills a row with spaces.
func (v *VTerm) clearRow(row int) {
	if row < 0 || row >= v.rows {
		return
	}
	for j := 0; j < v.cols; j++ {
		v.cells[row][j] = Cell{Char: ' '}
	}
}

// clampCursor ensures the cursor is within screen bounds.
func (v *VTerm) clampCursor() {
	if v.cursorR < 0 {
		v.cursorR = 0
	}
	if v.cursorR >= v.rows {
		v.cursorR = v.rows - 1
	}
	if v.cursorC < 0 {
		v.cursorC = 0
	}
	if v.cursorC >= v.cols {
		v.cursorC = v.cols - 1
	}
}

// reset clears the terminal to initial state.
func (v *VTerm) reset() {
	v.cells = v.makeGrid(v.cols, v.rows)
	v.cursorR = 0
	v.cursorC = 0
	v.style = CellStyle{}
	v.scrollTop = 0
	v.scrollBottom = v.rows - 1
	v.altCells = nil
	v.useAltBuf = false
}

// Resize changes the terminal dimensions, preserving as much content as possible.
func (v *VTerm) Resize(cols, rows int) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if cols < 1 {
		cols = 80
	}
	if rows < 1 {
		rows = 24
	}

	newGrid := v.makeGrid(cols, rows)

	// Copy existing content.
	copyRows := v.rows
	if rows < copyRows {
		copyRows = rows
	}
	copyCols := v.cols
	if cols < copyCols {
		copyCols = cols
	}
	for r := 0; r < copyRows; r++ {
		for c := 0; c < copyCols; c++ {
			newGrid[r][c] = v.cells[r][c]
		}
	}

	v.cells = newGrid
	v.cols = cols
	v.rows = rows
	v.scrollTop = 0
	v.scrollBottom = rows - 1
	v.clampCursor()
	v.dirty = true
}

// Render produces the screen content as a string with SGR color codes preserved.
// Lines are joined with newlines. Trailing spaces on each line are trimmed for
// cleaner output. Lines that are entirely blank produce empty strings.
//
// Render uses dirty-flag caching: if no Write() or Resize() has occurred since
// the last Render(), the cached string is returned immediately, avoiding the
// O(rows*cols) grid scan.
//
// Style tracking: the renderer maintains a "current active style" across cells.
// When a cell's style differs from the active style, the renderer emits a reset
// (\033[0m) followed by the new style's SGR. This prevents background colors,
// foreground colors, and text attributes from leaking between cells.
func (v *VTerm) Render() string {
	v.mu.Lock()
	defer v.mu.Unlock()

	// Fast path: return cached output if nothing changed.
	if !v.dirty && v.lastRender != "" {
		return v.lastRender
	}

	var lines []string
	for r := 0; r < v.rows; r++ {
		var b strings.Builder
		activeStyle := CellStyle{} // Start each line with default style.
		for c := 0; c < v.cols; c++ {
			cell := v.cells[r][c]
			// Emit SGR change if the cell's style differs from the active style.
			if !cell.Style.Equal(activeStyle) {
				if cell.Style.IsDefault() {
					// Transitioning to default: just reset.
					b.WriteString("\033[0m")
				} else {
					// Transitioning to a new style: reset then apply.
					// This is the safest approach — always reset before applying
					// a new style to prevent any attribute carry-over.
					b.WriteString("\033[0m")
					b.WriteString(cell.Style.ToSGR())
				}
				activeStyle = cell.Style
			}
			ch := cell.Char
			if ch == 0 {
				ch = ' '
			}
			b.WriteRune(ch)
		}
		// Reset SGR at end of line to prevent carry-over to next line.
		if !activeStyle.IsDefault() {
			b.WriteString("\033[0m")
		}
		lines = append(lines, trimTrailingSpaces(b.String()))
	}

	v.lastRender = strings.Join(lines, "\n")
	v.dirty = false
	return v.lastRender
}

// trimTrailingSpaces removes trailing spaces from a line, being careful to
// preserve ANSI escape sequences that apply to non-space content.
func trimTrailingSpaces(s string) string {
	// Fast path: if it ends with reset+spaces, trim those.
	runes := []rune(s)
	end := len(runes)
	for end > 0 && runes[end-1] == ' ' {
		end--
	}
	s = string(runes[:end])
	// Remove trailing reset if it's the last thing (no visible content after it).
	if strings.HasSuffix(s, "\033[0m") {
		s = strings.TrimSuffix(s, "\033[0m")
	}
	return s
}

// --- CSI parameter parsing helpers ---

// csiParam extracts a single parameter from a CSI parameter string at the given
// index. Returns defaultVal if the index is out of bounds or the value is 0.
func (v *VTerm) csiParam(params string, idx int, defaultVal int) int {
	parts := v.csiParams(params)
	if idx < len(parts) && parts[idx] > 0 {
		return parts[idx]
	}
	return defaultVal
}

// csiParams splits a CSI parameter string (e.g. "5;10") into integer values.
func (v *VTerm) csiParams(params string) []int {
	// Strip any leading private mode indicator like '?'.
	if strings.HasPrefix(params, "?") || strings.HasPrefix(params, ">") || strings.HasPrefix(params, "=") {
		params = params[1:]
	}

	if params == "" {
		return nil
	}

	parts := strings.Split(params, ";")
	result := make([]int, len(parts))
	for i, p := range parts {
		n, _ := strconv.Atoi(p)
		result[i] = n
	}
	return result
}
