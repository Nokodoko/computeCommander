package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/noko/computecommander/internal/trustgraph"
)

// captureStdout redirects os.Stdout for the duration of fn and returns
// whatever was written. Used to assert on rendered ANSI text without
// having to refactor production code to take an io.Writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()

	fn()
	_ = w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}

// stripANSI removes ANSI CSI sequences so we can assert on visible text only.
func stripANSI(s string) string {
	var sb strings.Builder
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

// TestEmitDisconnected_NoDuplicateBanner pins the dashboard fix: the body of a
// tg-summary frame in the disconnected state MUST NOT re-emit a "TG ·
// disconnected" banner — the embedding harness frame title already shows that
// signal, and a duplicate inside the body is exactly what the user reported.
//
// Asserts:
//
//	(1) "TG · disconnected" is NOT present anywhere in the body.
//	(2) The hint line is present and is the first body line.
//	(3) The trailer is "updated <now>" as the last body line.
//	(4) Exactly `lines` lines of output (trailing newline trimmed).
func TestEmitDisconnected_NoDuplicateBanner(t *testing.T) {
	pal := newPalette(true) // noColor=true so assertions can be string-exact

	out := captureStdout(t, func() {
		emitDisconnected(5, pal, "gateway unreachable: http://10.0.0.1:8088", "21:51:47")
	})

	plain := stripANSI(out)

	// (1) No duplicate banner.
	if strings.Contains(plain, "TG · disconnected") {
		t.Errorf("emitDisconnected must NOT emit a duplicate 'TG · disconnected' body banner\n--- got ---\n%s\n--- end ---", plain)
	}
	// Also reject the older "TG  disconnected" (no middle-dot) shape just in case.
	if strings.Contains(plain, "TG  disconnected") {
		t.Errorf("emitDisconnected must NOT emit a 'TG  disconnected' body banner\n--- got ---\n%s\n--- end ---", plain)
	}

	// (2) Hint line is first.
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	if got, want := len(lines), 5; got != want {
		t.Fatalf("expected %d lines, got %d:\n%s", want, got, plain)
	}
	if !strings.Contains(lines[0], "gateway unreachable: http://10.0.0.1:8088") {
		t.Errorf("first body line should be the diagnostic hint, got %q", lines[0])
	}

	// (3) Trailer.
	if want := "updated 21:51:47"; lines[len(lines)-1] != want {
		t.Errorf("last body line should be %q, got %q", want, lines[len(lines)-1])
	}
}

// TestEmitDisconnected_TrailerEdgeCases pins the padding contract: regardless
// of how short the disconnected hint is, the output is always exactly `lines`
// lines so the embedding frame does not reflow between renders.
func TestEmitDisconnected_TrailerEdgeCases(t *testing.T) {
	pal := newPalette(true)

	for _, lc := range []int{1, 2, 3, 5, 8} {
		t.Run("", func(t *testing.T) {
			out := captureStdout(t, func() {
				emitDisconnected(lc, pal, "gateway unreachable: http://10.0.0.1:8088", "00:00:00")
			})
			plain := strings.TrimRight(stripANSI(out), "\n")
			if plain == "" && lc > 0 {
				t.Fatalf("expected %d lines for lines=%d, got empty output", lc, lc)
			}
			lines := strings.Split(plain, "\n")
			if got, want := len(lines), lc; got != want {
				t.Errorf("lines=%d: expected %d output lines, got %d:\n%s", lc, want, got, plain)
			}
			// Trailer is always the last line when lines >= 1.
			if lc >= 1 {
				if got := lines[len(lines)-1]; got != "updated 00:00:00" {
					t.Errorf("lines=%d: last line should be the trailer, got %q", lc, got)
				}
			}
		})
	}
}

// makeChainTriples produces n triples of the shape
//
//	urn:s<i> --urn:p--> urn:o<i>
//
// so that each triple contributes one fresh subject node + one fresh entity
// object node. With n triples that yields 2n distinct nodes and n edges —
// exactly the invariants summarizeTriples must report.
func makeChainTriples(n int) []trustgraph.Triple {
	out := make([]trustgraph.Triple, n)
	for i := 0; i < n; i++ {
		out[i] = trustgraph.Triple{
			Subject:   trustgraph.NewIRITerm(fmt.Sprintf("urn:s%d", i)),
			Predicate: trustgraph.NewIRITerm("urn:p"),
			Object:    trustgraph.NewIRITerm(fmt.Sprintf("urn:o%d", i)),
		}
	}
	return out
}

// TestSummarizeTriples_CountsFromLiveResult pins the dynamic-counts contract
// that keeps the lewis tg pane from regressing to the mac reference's stale
// hardcoded counts: node/edge counts are derived from the LIVE result set on
// every call, never frozen. 903 triples in => 903 edges + 1806 nodes out.
func TestSummarizeTriples_CountsFromLiveResult(t *testing.T) {
	triples := makeChainTriples(903)

	nodeCount, edgeCount, top := summarizeTriples(triples)

	if got, want := edgeCount, 903; got != want {
		t.Errorf("edgeCount: got %d, want %d (must reflect FULL live result)", got, want)
	}
	if got, want := nodeCount, 1806; got != want {
		t.Errorf("nodeCount: got %d, want %d (each chain triple contributes 2 fresh nodes)", got, want)
	}
	if len(top) != 1806 {
		t.Errorf("top slice length: got %d, want 1806 (one entry per distinct node)", len(top))
	}
	// Spot-check sort: all entries have degree 1, so the slice should be
	// label-ascending.
	for i := 1; i < len(top); i++ {
		if top[i-1].degree < top[i].degree {
			t.Fatalf("top not sorted by degree desc at i=%d: %+v before %+v", i, top[i-1], top[i])
		}
		if top[i-1].degree == top[i].degree && top[i-1].label > top[i].label {
			t.Fatalf("top tie-break not label-asc at i=%d: %q before %q", i, top[i-1].label, top[i].label)
		}
	}
}

// TestSummarizeTriples_ExactCountsNoSuffix is the regression guard for the
// lewis dynamic-counts fix: even when the query result reaches the configured
// Limit, summarizeTriples reports the EXACT counts from the live result — there
// is no bucketing, floor, or "+" suffix. This matches tg-list, which prints
// len(nodes)/len(triples) verbatim, so the two commands agree on totals.
func TestSummarizeTriples_ExactCountsNoSuffix(t *testing.T) {
	const limit = 50
	triples := makeChainTriples(limit)

	nodeCount, edgeCount, _ := summarizeTriples(triples)

	if got, want := edgeCount, limit; got != want {
		t.Errorf("edgeCount: got %d, want %d (EXACT live total, no floor/cap)", got, want)
	}
	if got, want := nodeCount, 2*limit; got != want {
		t.Errorf("nodeCount: got %d, want %d (EXACT live total, no floor/cap)", got, want)
	}
}

// TestFormatCount pins the decoupled-count "+" suffix contract restored after
// the 973aff7 regression: when a count reaches the count-query ceiling the
// query saturated (true total may be higher) so a "+" suffix is appended to
// signal a lower bound; below the ceiling the exact count is shown verbatim.
//
// This is the behaviour that makes `cmdr tg-summary` render "10000+ edges"
// against the live graph instead of a misleading exact "10000 edges", while a
// node total under the ceiling (~3847) is shown exactly with no suffix.
func TestFormatCount(t *testing.T) {
	const ceiling = 10000
	cases := []struct {
		name    string
		n       int
		ceiling int
		want    string
	}{
		{"under ceiling shows exact", 3847, ceiling, "3847"},
		{"at ceiling saturates", 10000, ceiling, "10000+"},
		{"over ceiling saturates", 10001, ceiling, "10001+"},
		{"one below ceiling exact", 9999, ceiling, "9999"},
		{"zero exact", 0, ceiling, "0"},
		{"non-positive ceiling never suffixes", 10000, 0, "10000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCount(tc.n, tc.ceiling); got != tc.want {
				t.Errorf("formatCount(%d, %d) = %q, want %q", tc.n, tc.ceiling, got, tc.want)
			}
		})
	}
}

// TestSummarizeTriples_Empty pins the empty-input case: zero triples report
// zero nodes and zero edges with no panic and no degenerate suffix.
func TestSummarizeTriples_Empty(t *testing.T) {
	nodeCount, edgeCount, top := summarizeTriples(nil)
	if nodeCount != 0 || edgeCount != 0 {
		t.Errorf("empty input: got %d nodes / %d edges, want 0 / 0", nodeCount, edgeCount)
	}
	if len(top) != 0 {
		t.Errorf("empty input: got %d top-nodes, want 0", len(top))
	}
}
