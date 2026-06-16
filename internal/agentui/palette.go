package agentui

import "os"

// Palette holds ANSI escape sequences used by the renderers. The zero-value
// (every field empty string) is the NoColor mode — Sprintf-style writes of
// "%s%s%s" with palette fields collapse to plain text.
//
// Mirrors the palette struct in internal/commands/tg_summary.go but lives
// behind a stable package boundary so all three new subcommands (and the
// existing dashboard renderers, in a follow-up) share one source of truth.
type Palette struct {
	Bold   string
	Dim    string
	Reset  string
	Red    string
	Green  string
	Yellow string
	Cyan   string
	Purple string
}

// NewPalette returns the ANSI palette for the renderers. If noColor is true
// (or the NO_COLOR env var is set per https://no-color.org), every field is
// the empty string so format strings collapse to plain ASCII output.
func NewPalette(noColor bool) Palette {
	if noColor || NoColor() {
		return Palette{}
	}
	return Palette{
		Bold:   "\033[1m",
		Dim:    "\033[2m",
		Reset:  "\033[0m",
		Red:    "\033[31m",
		Green:  "\033[32m",
		Yellow: "\033[33m",
		Cyan:   "\033[36m",
		Purple: "\033[35m",
	}
}

// NoColor returns true when ANSI output should be suppressed because the
// NO_COLOR env var is non-empty (per https://no-color.org). Callers should
// also accept an explicit --no-color flag at the Cobra layer and pass it
// to NewPalette; this helper handles only the env-var side.
func NoColor() bool {
	return os.Getenv("NO_COLOR") != ""
}

// Hex24 returns the 24-bit foreground escape for the given hex color
// (e.g. "#50fa7b"), or the empty string when the palette is in NoColor mode
// (signaled by Reset == ""). Pairs with a trailing pal.Reset to balance the
// SGR.
//
// On parse failure returns the empty string. Mirrors the conversion done in
// internal/commands/pane.go:hexToRGB, kept here so the agentui package has
// no inbound dependency on internal/commands.
func (p Palette) Hex24(hex string) string {
	if p.Reset == "" {
		return ""
	}
	if len(hex) == 7 && hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) != 6 {
		return ""
	}
	r, ok1 := hexByte(hex[0:2])
	g, ok2 := hexByte(hex[2:4])
	b, ok3 := hexByte(hex[4:6])
	if !ok1 || !ok2 || !ok3 {
		return ""
	}
	return ansi24(r, g, b)
}

func hexByte(s string) (uint8, bool) {
	if len(s) != 2 {
		return 0, false
	}
	var out uint8
	for _, c := range s {
		out <<= 4
		switch {
		case c >= '0' && c <= '9':
			out |= uint8(c - '0')
		case c >= 'a' && c <= 'f':
			out |= uint8(c-'a') + 10
		case c >= 'A' && c <= 'F':
			out |= uint8(c-'A') + 10
		default:
			return 0, false
		}
	}
	return out, true
}

func ansi24(r, g, b uint8) string {
	// Allocate small buffer with itoa to avoid pulling in fmt for this hot path.
	buf := make([]byte, 0, 24)
	buf = append(buf, "\033[38;2;"...)
	buf = appendUint(buf, r)
	buf = append(buf, ';')
	buf = appendUint(buf, g)
	buf = append(buf, ';')
	buf = appendUint(buf, b)
	buf = append(buf, 'm')
	return string(buf)
}

func appendUint(dst []byte, v uint8) []byte {
	if v >= 100 {
		dst = append(dst, '0'+v/100)
		v %= 100
		dst = append(dst, '0'+v/10)
		dst = append(dst, '0'+v%10)
		return dst
	}
	if v >= 10 {
		dst = append(dst, '0'+v/10)
		dst = append(dst, '0'+v%10)
		return dst
	}
	dst = append(dst, '0'+v)
	return dst
}
