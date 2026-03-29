package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// barColors is a gradient palette for bar chart entries.
// Index 0 = highest value (brightest), last = lowest (dimmest).
var barColors = []lipgloss.Color{
	lipgloss.Color("#00ff88"), // bright green (top)
	lipgloss.Color("#00dd77"),
	lipgloss.Color("#00cc99"), // teal
	lipgloss.Color("#00bbbb"),
	lipgloss.Color("#0099cc"), // cyan
	lipgloss.Color("#0077dd"),
	lipgloss.Color("#5566cc"), // blue
	lipgloss.Color("#6655aa"),
	lipgloss.Color("#775599"), // purple
	lipgloss.Color("#666688"),
	lipgloss.Color("#555577"), // dim (bottom)
	lipgloss.Color("#444466"),
	lipgloss.Color("#444455"),
	lipgloss.Color("#333344"),
	lipgloss.Color("#333333"), // dimmest
}

// contextBarEmpty is the color for the empty portion of the context bar.
var contextBarEmpty = lipgloss.Color("#333344")

// segmentBarColors defines the color bands for the context bar.
// Each filled segment gets colored based on its position relative to total capacity.
var segmentBarColors = []lipgloss.Color{
	lipgloss.Color("#00ff88"), // 0-25%: bright green
	lipgloss.Color("#00bbbb"), // 25-50%: teal
	lipgloss.Color("#ffaa00"), // 50-75%: amber
	lipgloss.Color("#ff5555"), // 75-100%: red
}

// BarEntry represents a single item in a bar chart.
type BarEntry struct {
	Label string
	Value int
}

// BarChart renders horizontal ASCII bar charts using Unicode block characters.
// Thread-safe for concurrent use.
type BarChart struct {
	Title   string
	MaxBars int  // maximum bars to display (0 = unlimited)
	BarChar rune // character for bar segments (default: U+2588 FULL BLOCK)
}

// NewBarChart creates a BarChart with sensible defaults.
func NewBarChart(title string, maxBars int) *BarChart {
	return &BarChart{
		Title:   title,
		MaxBars: maxBars,
		BarChar: '\u25A0', // ■ BLACK SQUARE
	}
}

// Render produces a multi-line ASCII bar chart string.
// width is the total available columns. height is the max number of lines.
// entries should be pre-sorted by the caller (typically by Value descending).
func (bc *BarChart) Render(entries []BarEntry, width, height int) string {
	if len(entries) == 0 {
		return "  (no data)"
	}
	if width < 20 {
		width = 20
	}

	barChar := bc.BarChar
	if barChar == 0 {
		barChar = '\u2588'
	}

	// Determine how many bars to show.
	maxBars := bc.MaxBars
	if maxBars <= 0 || maxBars > len(entries) {
		maxBars = len(entries)
	}
	// Also constrain by available height (leave 1 line for overflow indicator).
	if maxBars > height-1 {
		maxBars = height - 1
	}
	if maxBars < 1 {
		maxBars = 1
	}

	visible := entries
	if maxBars < len(entries) {
		visible = entries[:maxBars]
	}

	// Find max label length and max value for scaling.
	maxLabelLen := 0
	maxValue := 0
	for _, e := range visible {
		if len(e.Label) > maxLabelLen {
			maxLabelLen = len(e.Label)
		}
		if e.Value > maxValue {
			maxValue = e.Value
		}
	}
	// Cap label display length.
	if maxLabelLen > 20 {
		maxLabelLen = 20
	}

	// Bar area width: total - label - padding - value display.
	// Format: "  label  [bar] value"
	valueWidth := len(fmt.Sprintf("%d", maxValue))
	barAreaWidth := width - maxLabelLen - valueWidth - 6 // 2 prefix + 2 gaps + 2 padding
	if barAreaWidth < 5 {
		barAreaWidth = 5
	}

	var lines []string
	for i, e := range visible {
		label := e.Label
		if len(label) > 20 {
			label = label[:18] + ".."
		}

		// Scale bar length.
		barLen := 0
		if maxValue > 0 {
			barLen = (e.Value * barAreaWidth) / maxValue
		}
		if barLen < 1 && e.Value > 0 {
			barLen = 1
		}

		// Pick color from gradient based on rank position.
		colorIdx := i
		if colorIdx >= len(barColors) {
			colorIdx = len(barColors) - 1
		}
		barStyle := lipgloss.NewStyle().Foreground(barColors[colorIdx])
		labelStyle := lipgloss.NewStyle().Foreground(barColors[colorIdx])
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

		bar := barStyle.Render(strings.Repeat(string(barChar), barLen))
		line := fmt.Sprintf("  %s %s %s",
			labelStyle.Render(fmt.Sprintf("%-*s", maxLabelLen, label)),
			bar,
			valueStyle.Render(fmt.Sprintf("%*d", valueWidth, e.Value)))
		lines = append(lines, line)
	}

	// Overflow indicator.
	if maxBars < len(entries) {
		lines = append(lines, fmt.Sprintf("  ... +%d more", len(entries)-maxBars))
	}

	return strings.Join(lines, "\n")
}

// sparklineBlocks maps normalized values (0-7) to Unicode block elements.
// U+2581 LOWER ONE EIGHTH BLOCK through U+2588 FULL BLOCK.
var sparklineBlocks = [8]rune{
	'\u2581', // 0: lowest
	'\u2582',
	'\u2583',
	'\u2584',
	'\u2585',
	'\u2586',
	'\u2587',
	'\u2588', // 7: highest
}

// Sparkline renders a single-line sparkline chart from a ring buffer of values.
// Thread-safe for concurrent Push/Render.
type Sparkline struct {
	Title  string
	Window int // ring buffer capacity

	mu     sync.Mutex
	values []int
	head   int  // next write position
	full   bool // true when buffer has wrapped
}

// NewSparkline creates a Sparkline with the given window size.
func NewSparkline(title string, window int) *Sparkline {
	if window < 1 {
		window = 60
	}
	return &Sparkline{
		Title:  title,
		Window: window,
		values: make([]int, window),
	}
}

// Push adds a new data point to the ring buffer.
func (s *Sparkline) Push(value int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.values[s.head] = value
	s.head++
	if s.head >= s.Window {
		s.head = 0
		s.full = true
	}
}

// snapshot returns the values in chronological order.
func (s *Sparkline) snapshot() []int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.full {
		if s.head == 0 {
			return nil
		}
		result := make([]int, s.head)
		copy(result, s.values[:s.head])
		return result
	}

	result := make([]int, s.Window)
	// Oldest data is at s.head, newest is at s.head-1.
	copy(result, s.values[s.head:])
	copy(result[s.Window-s.head:], s.values[:s.head])
	return result
}

// Render produces a single-line sparkline string.
// width is the available character width for the spark characters.
func (s *Sparkline) Render(width int) string {
	data := s.snapshot()
	if len(data) == 0 {
		return s.Title + " (no data)"
	}

	// If we have more data points than width, sample evenly.
	if len(data) > width {
		sampled := make([]int, width)
		for i := range sampled {
			idx := i * len(data) / width
			sampled[i] = data[idx]
		}
		data = sampled
	}

	// Find min/max for scaling.
	minVal, maxVal := data[0], data[0]
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	var buf strings.Builder
	buf.WriteString(s.Title)
	buf.WriteString(" ")

	valRange := maxVal - minVal
	for _, v := range data {
		idx := 0
		if valRange > 0 {
			idx = ((v - minVal) * 7) / valRange
		} else {
			idx = 3 // middle block when all values are equal
		}
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		buf.WriteRune(sparklineBlocks[idx])
	}

	// Append current value.
	if len(data) > 0 {
		buf.WriteString(fmt.Sprintf(" %d", data[len(data)-1]))
	}

	return buf.String()
}

// ContextBar renders a progress-bar style indicator similar to the Claude Code
// context usage bar. Shows a filled portion (■) and an empty portion (□) with
// a label and percentage. Thread-safe.
type ContextBar struct {
	Label     string
	Max       int  // capacity (e.g. estimated max triples)
	FillChar  rune // default: ■ U+25A0
	EmptyChar rune // default: □ U+25A1
}

// NewContextBar creates a ContextBar with sensible defaults.
func NewContextBar(label string, max int) *ContextBar {
	return &ContextBar{
		Label:     label,
		Max:       max,
		FillChar:  '\u25A0', // ■
		EmptyChar: '\u25A1', // □
	}
}

// Render produces a single-line context bar string.
// current is the current value, width is the total available columns.
func (cb *ContextBar) Render(current, width int) string {
	fillChar := cb.FillChar
	if fillChar == 0 {
		fillChar = '\u25A0'
	}
	emptyChar := cb.EmptyChar
	if emptyChar == 0 {
		emptyChar = '\u25A1'
	}

	max := cb.Max
	if max <= 0 {
		max = 1
	}

	pct := (current * 100) / max
	if pct > 100 {
		pct = 100
	}

	// Format: "  label  ■ ■ ■ ■ □ □ □ □  current/max (pct%)"
	// Each segment is "■ " (char + space), so segments = barWidth / 2.
	suffix := fmt.Sprintf(" %d/%d (%d%%)", current, max, pct)
	prefix := fmt.Sprintf("  %s ", cb.Label)

	barWidth := width - len(prefix) - len(suffix) - 2 // padding
	if barWidth < 20 {
		barWidth = 20
	}

	// Each segment takes 2 chars (square + space).
	totalSegments := barWidth / 2
	filledSegments := (current * totalSegments) / max
	if filledSegments > totalSegments {
		filledSegments = totalSegments
	}
	if filledSegments < 0 {
		filledSegments = 0
	}
	emptySegments := totalSegments - filledSegments

	emptyStyle := lipgloss.NewStyle().Foreground(contextBarEmpty)
	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00ff88"))
	pctStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))

	// Build segmented bar: "■ ■ ■ □ □ □"
	var segments []string
	fc := string(fillChar)
	ec := string(emptyChar)
	for i := 0; i < filledSegments; i++ {
		// Determine which color band this segment falls in.
		bandIdx := (i * len(segmentBarColors)) / totalSegments
		if bandIdx >= len(segmentBarColors) {
			bandIdx = len(segmentBarColors) - 1
		}
		style := lipgloss.NewStyle().Foreground(segmentBarColors[bandIdx])
		segments = append(segments, style.Render(fc))
	}
	for i := 0; i < emptySegments; i++ {
		segments = append(segments, emptyStyle.Render(ec))
	}

	var buf strings.Builder
	buf.WriteString(labelStyle.Render(prefix))
	buf.WriteString(strings.Join(segments, " "))
	buf.WriteString(pctStyle.Render(suffix))

	return buf.String()
}
