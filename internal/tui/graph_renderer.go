package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/noko/computecommander/internal/trustgraph"
)

// graphNode holds position and metadata for a rendered graph node.
type graphNode struct {
	ID       string
	Label    string
	IsEntity bool
	X        int // grid column
	Y        int // grid row
	Degree   int
}

// graphEdge connects two rendered nodes with a labeled predicate.
type graphEdge struct {
	FromID string
	ToID   string
	Label  string
}

// GraphRenderer produces a terminal-renderable ASCII graph for ego-graph
// visualization. It uses a radial tree layout centered on a focus node,
// with first-hop neighbors arranged in cardinal and diagonal positions.
type GraphRenderer struct {
	theme *Theme
}

// NewGraphRenderer constructs a renderer with the given theme.
func NewGraphRenderer(theme *Theme) *GraphRenderer {
	return &GraphRenderer{theme: theme}
}

// Render produces lines of styled text representing the ego-graph of focusNode.
// triples should be the full set of triples; the renderer filters to 1-hop neighbors.
// width and height constrain the output grid.
func (gr *GraphRenderer) Render(triples []trustgraph.Triple, focusNodeID string, width, height int) string {
	if width < 20 || height < 5 {
		return "  (too small to render graph)"
	}

	// ── Build ego-graph: 1-hop neighborhood ──
	type neighbor struct {
		id       string
		label    string // display label
		isEntity bool
		predicate string
		outgoing  bool // true = focus -> neighbor, false = neighbor -> focus
		degree    int  // total edges in full graph
	}

	neighbors := make(map[string]*neighbor)
	degreeCounts := make(map[string]int)

	// First pass: count degrees.
	for _, t := range triples {
		sID := t.Subject.DisplayValue()
		oID := t.Object.DisplayValue()
		degreeCounts[sID]++
		degreeCounts[oID]++
	}

	// Second pass: find 1-hop neighbors of focus node.
	for _, t := range triples {
		sID := t.Subject.DisplayValue()
		oID := t.Object.DisplayValue()
		pLabel := t.Predicate.ShortLabel(16)

		if sID == focusNodeID && oID != focusNodeID {
			if _, exists := neighbors[oID]; !exists {
				neighbors[oID] = &neighbor{
					id:        oID,
					label:     t.Object.ShortLabel(16),
					isEntity:  t.Object.IsEntity(),
					predicate: pLabel,
					outgoing:  true,
					degree:    degreeCounts[oID],
				}
			}
		}
		if oID == focusNodeID && sID != focusNodeID {
			if _, exists := neighbors[sID]; !exists {
				neighbors[sID] = &neighbor{
					id:        sID,
					label:     t.Subject.ShortLabel(16),
					isEntity:  t.Subject.IsEntity(),
					predicate: pLabel,
					outgoing:  false,
					degree:    degreeCounts[sID],
				}
			}
		}
	}

	// Sort neighbors by degree (highest first for cardinal positions).
	sortedNeighbors := make([]*neighbor, 0, len(neighbors))
	for _, n := range neighbors {
		sortedNeighbors = append(sortedNeighbors, n)
	}
	sort.Slice(sortedNeighbors, func(i, j int) bool {
		return sortedNeighbors[i].degree > sortedNeighbors[j].degree
	})

	// ── Assign positions on grid ──
	// Center: focus node
	// Cardinal: N, E, S, W (top 4 neighbors by degree)
	// Diagonal: NE, SE, SW, NW (next 4)
	// Overflow: shown as "+N more" indicator

	centerX := width / 2
	centerY := height / 2

	// Radius for neighbor placement (in character columns/rows).
	radiusX := width/4 - 2
	if radiusX < 10 {
		radiusX = 10
	}
	radiusY := height/4 - 1
	if radiusY < 3 {
		radiusY = 3
	}

	// 8 positions: N, NE, E, SE, S, SW, W, NW
	type position struct {
		dx, dy int
	}
	positions := []position{
		{0, -radiusY},              // N
		{radiusX, 0},               // E
		{0, radiusY},               // S
		{-radiusX, 0},              // W
		{radiusX / 2, -radiusY / 2}, // NE
		{radiusX / 2, radiusY / 2},  // SE
		{-radiusX / 2, radiusY / 2}, // SW
		{-radiusX / 2, -radiusY / 2}, // NW
	}

	maxVisible := 8
	if len(sortedNeighbors) < maxVisible {
		maxVisible = len(sortedNeighbors)
	}

	// ── Build grid ──
	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Helper: place text on grid (centered at x,y).
	placeText := func(text string, cx, cy int) {
		startX := cx - len(text)/2
		for i, ch := range text {
			gx := startX + i
			if gx >= 0 && gx < width && cy >= 0 && cy < height {
				grid[cy][gx] = ch
			}
		}
	}

	// Helper: draw a simple line between two points.
	drawLine := func(x1, y1, x2, y2 int) {
		dx := x2 - x1
		dy := y2 - y1
		steps := int(math.Max(math.Abs(float64(dx)), math.Abs(float64(dy))))
		if steps == 0 {
			return
		}
		for i := 1; i < steps; i++ {
			px := x1 + dx*i/steps
			py := y1 + dy*i/steps
			if px >= 0 && px < width && py >= 0 && py < height {
				if grid[py][px] == ' ' {
					if dx == 0 {
						grid[py][px] = '|'
					} else if dy == 0 {
						grid[py][px] = '-'
					} else {
						grid[py][px] = '.'
					}
				}
			}
		}
	}

	// Place focus node.
	focusLabel := focusNodeID
	for _, t := range triples {
		if t.Subject.DisplayValue() == focusNodeID {
			focusLabel = t.Subject.ShortLabel(16)
			break
		}
		if t.Object.DisplayValue() == focusNodeID {
			focusLabel = t.Object.ShortLabel(16)
			break
		}
	}
	focusText := "[*" + focusLabel + "*]"
	placeText(focusText, centerX, centerY)

	// Place neighbor nodes and draw edges.
	for i := 0; i < maxVisible; i++ {
		n := sortedNeighbors[i]
		pos := positions[i]
		nx := centerX + pos.dx
		ny := centerY + pos.dy

		// Clamp to grid bounds.
		if nx < 2 {
			nx = 2
		}
		if nx >= width-2 {
			nx = width - 3
		}
		if ny < 0 {
			ny = 0
		}
		if ny >= height-1 {
			ny = height - 2
		}

		nodeText := "[" + n.label + "]"
		placeText(nodeText, nx, ny)

		// Draw edge line.
		drawLine(centerX, centerY, nx, ny)

		// Place predicate label at midpoint.
		midX := (centerX + nx) / 2
		midY := (centerY + ny) / 2
		predText := n.predicate
		if len(predText) > 12 {
			predText = predText[:10] + ".."
		}
		placeText(predText, midX, midY)
	}

	// Overflow indicator.
	if len(sortedNeighbors) > maxVisible {
		overflowText := fmt.Sprintf("[+%d more]", len(sortedNeighbors)-maxVisible)
		placeText(overflowText, centerX, height-1)
	}

	// ── Render with lipgloss styling ──
	focusStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50fa7b"))
	nodeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5DADE2"))
	literalStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB347"))
	edgeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))

	var lines []string
	for y := 0; y < height; y++ {
		line := string(grid[y])
		// Trim trailing whitespace for cleaner output.
		line = strings.TrimRight(line, " ")

		// Apply styling based on content patterns.
		// Focus node: [*...*]
		if strings.Contains(line, "[*") {
			line = styleFocusNode(line, focusStyle)
		}
		// Regular nodes: [...] but not focus
		if strings.Contains(line, "[") && !strings.Contains(line, "[*") {
			line = styleNodes(line, nodeStyle, literalStyle, dimStyle)
		}
		// Edge characters.
		line = styleEdgeChars(line, edgeStyle)

		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// styleFocusNode applies the focus style to [*...*] patterns.
func styleFocusNode(line string, style lipgloss.Style) string {
	start := strings.Index(line, "[*")
	if start < 0 {
		return line
	}
	end := strings.Index(line[start:], "*]")
	if end < 0 {
		return line
	}
	end += start + 2

	before := line[:start]
	focus := line[start:end]
	after := line[end:]

	return before + style.Render(focus) + after
}

// styleNodes applies node styling to [...] patterns.
func styleNodes(line string, entityStyle, literalStyle, dimStyle lipgloss.Style) string {
	result := line
	for strings.Contains(result, "[") {
		start := strings.Index(result, "[")
		end := strings.Index(result[start:], "]")
		if end < 0 {
			break
		}
		end += start + 1

		nodeName := result[start+1 : end-1]
		if strings.HasPrefix(nodeName, "+") && strings.HasSuffix(nodeName, "more") {
			// Overflow indicator.
			styled := dimStyle.Render(result[start:end])
			result = result[:start] + styled + result[end:]
			break
		}

		// Default to entity style.
		styled := entityStyle.Render(result[start:end])
		result = result[:start] + styled + result[end:]
		break // Only style first occurrence per pass to avoid offset issues.
	}
	return result
}

// styleEdgeChars applies dim styling to edge drawing characters (|, -, .).
func styleEdgeChars(line string, style lipgloss.Style) string {
	// Simple character-level replacement for edge characters.
	// Only style standalone edge chars (not inside styled nodes).
	var result strings.Builder
	for _, ch := range line {
		switch ch {
		case '|', '.':
			result.WriteString(style.Render(string(ch)))
		case '-':
			result.WriteString(style.Render(string(ch)))
		default:
			result.WriteRune(ch)
		}
	}
	return result.String()
}

// RenderTriplesView produces a scrollable table of triples.
func (gr *GraphRenderer) RenderTriplesView(triples []trustgraph.Triple, scrollOffset, cursor, width, height int) string {
	entityStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#5DADE2"))
	predStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50fa7b"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF"))

	var lines []string

	// Header.
	titleLine := fmt.Sprintf(" Triples (%d total, showing %d-%d)", len(triples), scrollOffset+1, minInt(scrollOffset+height-6, len(triples)))
	lines = append(lines, headerStyle.Render(titleLine))
	lines = append(lines, dimStyle.Render(strings.Repeat("─", width)))

	// Column widths.
	colS := (width - 6) / 3
	if colS < 10 {
		colS = 10
	}
	colP := colS
	colO := colS

	headerLine := fmt.Sprintf(" %-*s  %-*s  %-*s", colS, "Subject", colP, "Predicate", colO, "Object")
	lines = append(lines, headerStyle.Render(headerLine))
	lines = append(lines, dimStyle.Render(strings.Repeat("─", width)))

	// Visible rows.
	visibleRows := height - 6
	if visibleRows < 1 {
		visibleRows = 1
	}
	endIdx := scrollOffset + visibleRows
	if endIdx > len(triples) {
		endIdx = len(triples)
	}

	for i := scrollOffset; i < endIdx; i++ {
		t := triples[i]
		subj := truncateStr(t.Subject.ShortLabel(colS-2), colS-2)
		pred := truncateStr(t.Predicate.ShortLabel(colP-2), colP-2)
		obj := truncateStr(t.Object.ShortLabel(colO-2), colO-2)

		prefix := " "
		sStyle := entityStyle
		if i == cursor {
			prefix = ">"
			sStyle = cursorStyle
		}

		line := prefix + sStyle.Render(fmt.Sprintf("%-*s", colS, subj)) + "  " +
			predStyle.Render(fmt.Sprintf("%-*s", colP, pred)) + "  " +
			entityStyle.Render(fmt.Sprintf("%-*s", colO, obj))
		lines = append(lines, line)
	}

	// Footer.
	lines = append(lines, dimStyle.Render(strings.Repeat("─", width)))
	lines = append(lines, dimStyle.Render(" /:search  j/k:scroll  Enter:inspect  m:mode"))

	return strings.Join(lines, "\n")
}

// truncateStr shortens a string to maxLen, appending ".." if truncated.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-2] + ".."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
