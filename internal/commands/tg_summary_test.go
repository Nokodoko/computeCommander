package commands

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
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

// TestResolveTGGatewayURL_FallbackToMonty pins the gateway-resolution contract
// used by tg-summary / tg-list / tg --pane. The previous bug was that
// tg-summary called trustgraph.New(cfg.GatewayURL, …) directly, picking up an
// empty config or a dead localhost default. The resolver is now the single
// source of truth.
func TestResolveTGGatewayURL_FallbackToMonty(t *testing.T) {
	// Clear env so the test is deterministic.
	t.Setenv("TG_URL", "")
	t.Setenv("TRUSTGRAPH_URL", "")

	got := resolveTGGatewayURL(nil, "")
	if want := "http://10.0.0.1:8088"; got != want {
		t.Errorf("empty config => %q, want %q (monty WG fallback)", got, want)
	}

	got = resolveTGGatewayURL(nil, "http://example:1234")
	if want := "http://example:1234"; got != want {
		t.Errorf("non-empty config => %q, want %q", got, want)
	}

	t.Setenv("TG_URL", "http://from-env:9999")
	got = resolveTGGatewayURL(nil, "http://config:1111")
	if want := "http://from-env:9999"; got != want {
		t.Errorf("TG_URL must win over config: got %q, want %q", got, want)
	}
}
