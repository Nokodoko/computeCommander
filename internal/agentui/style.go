package agentui

// BoxStyle is the set of separator/punctuation characters used by the
// renderers. ASCII fallback is used in NoColor mode so the output is
// strictly 7-bit ASCII; the styled variant is the Unicode middle-dot and
// box-drawing horizontal line that match the existing tg-summary frame.
type BoxStyle struct {
	Sep  string // mid-line separator, e.g. " · " styled or "  " plain
	HBar string // horizontal bar, e.g. "──" styled or "--" plain
	Bul  string // bullet, e.g. "• " styled or "* " plain
}

// NewBoxStyle returns the right BoxStyle for the given palette. When the
// palette is in NoColor mode (Reset == ""), ASCII fallback characters are
// used so the output contains zero non-ASCII bytes. Otherwise the styled
// Unicode characters match the look of tg-summary.
func NewBoxStyle(p Palette) BoxStyle {
	if p.Reset == "" {
		return BoxStyle{
			Sep:  "  ",
			HBar: "--",
			Bul:  "* ",
		}
	}
	return BoxStyle{
		Sep:  " · ",
		HBar: "──",
		Bul:  "• ",
	}
}
