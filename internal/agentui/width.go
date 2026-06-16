package agentui

import "strings"

// VisibleLen returns the count of visible runes in s, skipping ANSI CSI
// escape sequences (\033[…m). Good enough for our palette which only uses
// SGR sequences. Mirrors internal/commands/tg_summary.go:visibleLen.
func VisibleLen(s string) int {
	n := 0
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if c >= 0x40 && c <= 0x7e {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		c := s[i]
		// UTF-8 rune boundary: count only lead bytes (< 0x80 single-byte,
		// >= 0xC0 multibyte lead). Skip continuation bytes (0x80 - 0xBF).
		if c < 0x80 || c >= 0xc0 {
			n++
		}
		i++
	}
	return n
}

// Truncate cuts s to at most w visible columns. ANSI codes are preserved
// verbatim but excluded from the column count. Cuts on UTF-8 rune
// boundaries. If a cut happens inside an active SGR, a trailing "\033[0m"
// is appended to balance the escape.
//
// w <= 0 returns the empty string.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if VisibleLen(s) <= w {
		return s
	}
	var sb strings.Builder
	visible := 0
	sawSGR := false
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if c >= 0x40 && c <= 0x7e {
					j++
					break
				}
				j++
			}
			sb.WriteString(s[i:j])
			sawSGR = true
			i = j
			continue
		}
		if visible >= w {
			break
		}
		c := s[i]
		runeLen := 1
		switch {
		case c < 0x80:
			runeLen = 1
		case c >= 0xf0:
			runeLen = 4
		case c >= 0xe0:
			runeLen = 3
		case c >= 0xc0:
			runeLen = 2
		}
		if i+runeLen > len(s) {
			runeLen = len(s) - i
		}
		sb.WriteString(s[i : i+runeLen])
		visible++
		i += runeLen
	}
	if sawSGR {
		sb.WriteString("\033[0m")
	}
	return sb.String()
}

// StripANSI removes ANSI CSI sequences from s. Used by the NoColor renderer
// path defensively when assembling output from sub-helpers that might emit
// SGR sequences.
func StripANSI(s string) string {
	if !strings.ContainsRune(s, 0x1b) {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) {
				c := s[j]
				if c >= 0x40 && c <= 0x7e {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		sb.WriteByte(s[i])
		i++
	}
	return sb.String()
}
