package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// zellijSessionInfoDir is the directory where zellij persists per-session
// metadata and layout snapshots. Exposed for testing.
var zellijSessionInfoDir = func() string {
	if dir := os.Getenv("ZELLIJ_CACHE_DIR"); dir != "" {
		return filepath.Join(dir, "session_info")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache", "zellij", "contract_version_1", "session_info")
	}
	return ""
}

// claudePrefix marks Claude Code pane titles. Claude Code overrides the
// terminal title to "✳ <customTitle>" once it starts (see MEMORY.md).
const claudePrefix = "✳ "

// claudeFallbackTitle is the bare title Claude Code sets when no customTitle
// has been registered yet.
const claudeFallbackTitle = "✳ Claude Code"

// recoveredPane is one pane extracted from a saved session-metadata.kdl.
type recoveredPane struct {
	ID              int    `json:"id"`
	Title           string `json:"title"`
	IsFloating      bool   `json:"is_floating"`
	IsSuppressed    bool   `json:"is_suppressed"`
	IsFocused       bool   `json:"is_focused"`
	Exited          bool   `json:"exited"`
	TerminalCommand string `json:"terminal_command,omitempty"`
}

// recoveredSession is the parsed view of a frozen zellij session.
type recoveredSession struct {
	Name      string          `json:"name"`
	InfoDir   string          `json:"info_dir"`
	TabHashes []string        `json:"tab_hashes"`
	Panes     []recoveredPane `json:"panes"`
}

// matchedClaudeSession is a Claude session correlated with one of the panes.
type matchedClaudeSession struct {
	SessionID    string  `json:"session_id"`
	ProjectPath  string  `json:"project_path"`
	SessionName  string  `json:"session_name"`
	Modified     float64 `json:"modified"`
	MatchedTitle string  `json:"matched_title,omitempty"`
	MatchSource  string  `json:"match_source"` // "title" | "cwd" | "tab-cwd"
}

// RecoverCmd returns the "recover" command for unfreezing a stale zellij
// session and enumerating the Claude Code sessions that lived inside it.
//
// Default behaviour is a SAFE DRY-RUN: it prints the panes, the tab hashes,
// and any Claude sessions that match. No destructive zellij action runs
// unless --force is set together with an explicit --strategy.
func RecoverCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recover <session-name>",
		Short: "Recover a frozen zellij session and list its Claude sessions",
		Long: `Recover a frozen-but-not-EXITED zellij session.

Default mode is a safe dry-run: parses the saved session metadata under
~/.cache/zellij/contract_version_1/session_info/<name>/, lists the panes
(Claude panes are marked with ✳), extracts dashboard tab hashes from
fp-wrapper / lazygit-wrapper terminal_commands, and cross-references
~/.claude/projects/*/sessions-index.json to print the Claude sessions
that lived inside the zellij session. Nothing destructive runs.

Pass --force together with --strategy=kill-clients to SIGTERM stale
attach clients, or --strategy=kill-session to fully kill the daemon and
let zellij resurrect from the saved layout on the next attach.`,
		Args:    cobra.ExactArgs(1),
		GroupID: "LIFECYCLE",
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			listOnly, _ := cmd.Flags().GetBool("list-only")
			force, _ := cmd.Flags().GetBool("force")
			strategy, _ := cmd.Flags().GetString("strategy")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			rec, err := parseRecoveredSession(name)
			if err != nil {
				if jsonOut {
					return json.NewEncoder(os.Stdout).Encode(map[string]any{
						"success": false,
						"command": "recover",
						"error":   err.Error(),
					})
				}
				return err
			}

			alive, status := probeZellijSession(name)
			matches := matchClaudeSessions(rec)

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success":         true,
					"command":         "recover",
					"session":         rec,
					"daemon_alive":    alive,
					"daemon_status":   status,
					"claude_sessions": matches,
					"would_act":       force && !listOnly,
					"strategy":        strategy,
				})
			}

			printRecoverReport(os.Stdout, rec, alive, status, matches)

			if listOnly {
				return nil
			}
			if !force {
				fmt.Fprintln(os.Stdout, "\nDry-run only. Re-run with --force --strategy=kill-clients|kill-session to act.")
				return nil
			}

			actions, actErr := runRecoveryStrategy(name, strategy)
			for _, a := range actions {
				fmt.Fprintln(os.Stdout, a)
			}
			if actErr != nil {
				return actErr
			}
			return nil
		},
	}

	cmd.Flags().Bool("list-only", false, "Only enumerate panes and Claude sessions; do not act even with --force")
	cmd.Flags().Bool("force", false, "Skip confirmation and execute the recovery strategy")
	cmd.Flags().String("strategy", "auto", "Recovery strategy: auto|kill-clients|kill-session")
	return cmd
}

// parseRecoveredSession reads ~/.cache/zellij/.../session_info/<name>/session-metadata.kdl
// and returns a structured view. Returns an error if the directory is missing.
func parseRecoveredSession(name string) (*recoveredSession, error) {
	if name == "" {
		return nil, errors.New("session name is required")
	}
	infoRoot := zellijSessionInfoDir()
	if infoRoot == "" {
		return nil, errors.New("could not resolve zellij session_info directory")
	}
	infoDir := filepath.Join(infoRoot, name)
	if _, err := os.Stat(infoDir); err != nil {
		return nil, fmt.Errorf("session_info for %q not found at %s: %w", name, infoDir, err)
	}

	metaPath := filepath.Join(infoDir, "session-metadata.kdl")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("read session-metadata.kdl: %w", err)
	}

	panes := parsePanesFromMetadata(string(data))
	tabHashes := extractTabHashesFromPanes(panes)

	return &recoveredSession{
		Name:      name,
		InfoDir:   infoDir,
		TabHashes: tabHashes,
		Panes:     panes,
	}, nil
}

// panePropRe matches a single property line inside a `pane { ... }` block.
// Examples it captures:
//
//	id 123                      -> ("id", "123")
//	is_focused true             -> ("is_focused", "true")
//	title "✳ Claude Code"       -> ("title", "✳ Claude Code")
//	terminal_command "bash ..." -> ("terminal_command", "bash ...")
var panePropRe = regexp.MustCompile(`^\s*([a-z_]+)\s+(?:"([^"]*)"|([^"\s][^\s]*))\s*$`)

// parsePanesFromMetadata is a permissive line-based parser for the
// `pane { ... }` blocks inside session-metadata.kdl. It does not attempt
// to be a full KDL parser; the format is regular and stable enough for
// line-based extraction.
func parsePanesFromMetadata(s string) []recoveredPane {
	var panes []recoveredPane
	var cur *recoveredPane
	depth := 0

	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		// Track entry into a pane block. We only treat top-level
		// `pane {` lines (not the broader `panes {` wrapper) as the
		// start of a pane record.
		if line == "pane {" || strings.HasPrefix(line, "pane {") {
			if cur == nil {
				cur = &recoveredPane{}
				depth = 1
				continue
			}
			depth++
			continue
		}
		if cur == nil {
			continue
		}

		// Track braces inside a pane (some panes contain nested blocks).
		if strings.HasSuffix(line, "{") && !strings.HasPrefix(line, "pane ") {
			depth++
			continue
		}
		if line == "}" {
			depth--
			if depth == 0 {
				panes = append(panes, *cur)
				cur = nil
			}
			continue
		}

		// Property line.
		m := panePropRe.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		key := m[1]
		val := m[2]
		if val == "" {
			val = m[3]
		}
		switch key {
		case "id":
			if n, err := strconv.Atoi(val); err == nil {
				cur.ID = n
			}
		case "title":
			cur.Title = val
		case "is_floating":
			cur.IsFloating = val == "true"
		case "is_suppressed":
			cur.IsSuppressed = val == "true"
		case "is_focused":
			cur.IsFocused = val == "true"
		case "exited":
			cur.Exited = val == "true"
		case "terminal_command":
			cur.TerminalCommand = val
		}
	}
	return panes
}

// tabHashRe captures the trailing 8-hex-char tab hash from fp-wrapper.sh
// and lazygit-wrapper.sh terminal_commands.
var tabHashRe = regexp.MustCompile(`(?:fp-wrapper\.sh|lazygit-wrapper\.sh)\s+\S+\s+([0-9a-f]{8})\b`)

// extractTabHashesFromPanes scans every pane's terminal_command for the
// per-tab hash that fp-wrapper / lazygit-wrapper write into the CWD file
// path. Returns a deduplicated, sorted slice.
func extractTabHashesFromPanes(panes []recoveredPane) []string {
	seen := make(map[string]struct{})
	for _, p := range panes {
		if p.TerminalCommand == "" {
			continue
		}
		for _, m := range tabHashRe.FindAllStringSubmatch(p.TerminalCommand, -1) {
			seen[m[1]] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// matchClaudeSessions correlates the recovered panes with Claude sessions
// from ~/.claude/projects/*/sessions-index.json. Three sources, in priority:
//  1. "title"   — pane title equals "✳ <customTitle>" of an indexed session.
//  2. "tab-cwd" — a tab hash maps to /tmp/cmdr-<uid>-<hash>-cwd whose
//     contents match the project path of an indexed session.
//  3. "cwd"     — the focused floating pane's title is the bare
//     "✳ Claude Code" placeholder; we fall back to the latest session
//     for the project pointed to by any active CWD file.
//
// Sessions are deduplicated by SessionID and sorted newest-first.
func matchClaudeSessions(rec *recoveredSession) []matchedClaudeSession {
	all, err := listClaudeSessions()
	if err != nil || len(all) == 0 {
		return nil
	}

	customByTitle := make(map[string][]claudeSession)
	byProjectPath := make(map[string][]claudeSession)
	for _, s := range all {
		// Strip the "✳ " prefix Claude Code adds so we can match against
		// pane titles directly.
		key := claudePrefix + s.SessionName
		customByTitle[key] = append(customByTitle[key], s)
		byProjectPath[s.ProjectPath] = append(byProjectPath[s.ProjectPath], s)
	}

	out := make([]matchedClaudeSession, 0)
	dedup := make(map[string]bool)

	addMatch := func(s claudeSession, source, matched string) {
		if dedup[s.SessionID] {
			return
		}
		dedup[s.SessionID] = true
		out = append(out, matchedClaudeSession{
			SessionID:    s.SessionID,
			ProjectPath:  s.ProjectPath,
			SessionName:  s.SessionName,
			Modified:     s.Modified,
			MatchedTitle: matched,
			MatchSource:  source,
		})
	}

	// 1) Title match.
	for _, p := range rec.Panes {
		if p.Title == "" || p.Title == claudeFallbackTitle {
			continue
		}
		if !strings.HasPrefix(p.Title, claudePrefix) {
			continue
		}
		if cands, ok := customByTitle[p.Title]; ok {
			// Prefer the most recently modified session with that title.
			sort.Slice(cands, func(i, j int) bool { return cands[i].Modified > cands[j].Modified })
			addMatch(cands[0], "title", p.Title)
		}
	}

	// 2) Tab-CWD match: read /tmp/cmdr-<uid>-<hash>-cwd for each tab hash
	//    and look up the project path. The CWD file may be live (current
	//    dashboard) or stale — both are useful here, since the user wants
	//    to know which projects had panes in the frozen session.
	uid := os.Getuid()
	for _, hash := range rec.TabHashes {
		path := fmt.Sprintf("/tmp/cmdr-%d-%s-cwd", uid, hash)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		project := strings.TrimSpace(string(data))
		if project == "" {
			continue
		}
		cands := byProjectPath[project]
		sort.Slice(cands, func(i, j int) bool { return cands[i].Modified > cands[j].Modified })
		for i, s := range cands {
			// Cap at the 3 newest sessions per tab-CWD to avoid flooding
			// the report with months-old sessions for a busy project.
			if i >= 3 {
				break
			}
			addMatch(s, "tab-cwd", "")
		}
	}

	// 3) Bare-Claude fallback: if any pane is the placeholder
	//    "✳ Claude Code" title, surface the newest session for each
	//    discovered tab-cwd project (already covered above).

	sort.Slice(out, func(i, j int) bool { return out[i].Modified > out[j].Modified })
	return out
}

// probeZellijSession runs `zellij list-sessions` and looks for `name`.
// Returns whether the daemon registers the session and a one-line status.
func probeZellijSession(name string) (bool, string) {
	out, err := exec.Command("zellij", "list-sessions").CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("zellij list-sessions failed: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, name) {
			continue
		}
		switch {
		case strings.Contains(line, "EXITED"):
			return false, "EXITED (attach to resurrect from saved layout)"
		case strings.Contains(line, "(current)"):
			return true, "current session"
		default:
			return true, "alive (no EXITED tag)"
		}
	}
	return false, "not present in zellij list-sessions"
}

// runRecoveryStrategy executes a recovery strategy. Returns one log line per
// action it performed plus an error if any sub-step failed. The caller is
// responsible for confirming destructive actions.
func runRecoveryStrategy(name, strategy string) ([]string, error) {
	var log []string
	switch strategy {
	case "", "auto":
		// Auto: poke the daemon, then SIGTERM stuck attach clients. Never
		// kill-session on its own — the daemon may still be healthy and
		// the user is mid-flow.
		log = append(log, "[auto] probing daemon with `dump-screen`")
		if err := pokeDaemon(name); err != nil {
			log = append(log, fmt.Sprintf("[auto] daemon probe failed: %v", err))
		} else {
			log = append(log, "[auto] daemon probe ok")
		}
		killed, err := killStuckClients(name)
		log = append(log, fmt.Sprintf("[auto] terminated %d stuck attach clients", killed))
		return log, err
	case "kill-clients":
		killed, err := killStuckClients(name)
		log = append(log, fmt.Sprintf("[kill-clients] terminated %d stuck attach clients", killed))
		return log, err
	case "kill-session":
		log = append(log, fmt.Sprintf("[kill-session] running `zellij kill-session %s`", name))
		out, err := exec.Command("zellij", "kill-session", name).CombinedOutput()
		log = append(log, strings.TrimSpace(string(out)))
		if err != nil {
			return log, fmt.Errorf("kill-session: %w", err)
		}
		log = append(log, fmt.Sprintf("[kill-session] %q killed; reattach to resurrect from saved layout", name))
		return log, nil
	default:
		return nil, fmt.Errorf("unknown strategy %q (allowed: auto, kill-clients, kill-session)", strategy)
	}
}

// pokeDaemon runs a fast, non-destructive `dump-screen` action against the
// session to test whether the daemon is responsive. Bounded by 2s timeout.
func pokeDaemon(name string) error {
	cmd := exec.Command("zellij", "--session", name, "action", "dump-screen", "/dev/null")
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		return errors.New("dump-screen timed out after 2s (daemon may be wedged)")
	}
}

// killStuckClients SIGTERMs any `zellij ... attach <name>` processes. A
// frozen session is often a wedged client, not a wedged daemon; killing
// the client lets the user re-attach cleanly.
func killStuckClients(name string) (int, error) {
	out, err := exec.Command("pgrep", "-af", "zellij").Output()
	if err != nil {
		// pgrep returns 1 if no matches.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return 0, nil
		}
		return 0, fmt.Errorf("pgrep zellij: %w", err)
	}
	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.Contains(line, "attach") || !strings.Contains(line, name) {
			continue
		}
		// Skip our own PID just in case.
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil || pid == os.Getpid() {
			continue
		}
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		if err := proc.Signal(os.Interrupt); err == nil {
			count++
		}
	}
	return count, nil
}

// printRecoverReport renders a human-readable report of the recovery dry-run.
func printRecoverReport(w *os.File, rec *recoveredSession, alive bool, status string, matches []matchedClaudeSession) {
	fmt.Fprintf(w, "Session:        %s\n", rec.Name)
	fmt.Fprintf(w, "Info dir:       %s\n", rec.InfoDir)
	fmt.Fprintf(w, "Daemon status:  %s (alive=%t)\n", status, alive)
	fmt.Fprintf(w, "Tab hashes:     %s\n", joinOrNone(rec.TabHashes))
	fmt.Fprintf(w, "Panes:          %d total\n", len(rec.Panes))

	claudePanes := claudePaneTitles(rec.Panes)
	fmt.Fprintf(w, "Claude panes:   %d\n", len(claudePanes))
	for _, t := range claudePanes {
		fmt.Fprintf(w, "  - %s\n", t)
	}

	fmt.Fprintf(w, "\nClaude sessions (%d match%s):\n", len(matches), plural(len(matches), "", "es"))
	if len(matches) == 0 {
		fmt.Fprintln(w, "  (none — no titles matched a sessions-index.json entry, and no /tmp tab-CWD files were readable)")
		return
	}
	fmt.Fprintf(w, "  %-36s %-7s %-19s %s\n", "SESSION_ID", "SOURCE", "MODIFIED", "PROJECT / TITLE")
	for _, m := range matches {
		mod := time.Unix(int64(m.Modified), 0).Format("2006-01-02 15:04:05")
		title := m.SessionName
		if len(title) > 60 {
			title = title[:60]
		}
		fmt.Fprintf(w, "  %-36s %-7s %-19s %s — %s\n", m.SessionID, m.MatchSource, mod, m.ProjectPath, title)
	}
}

func claudePaneTitles(panes []recoveredPane) []string {
	var out []string
	for _, p := range panes {
		if strings.HasPrefix(p.Title, claudePrefix) {
			out = append(out, p.Title)
		}
	}
	return out
}

func joinOrNone(xs []string) string {
	if len(xs) == 0 {
		return "(none)"
	}
	return strings.Join(xs, ", ")
}

func plural(n int, singular, pluralSuffix string) string {
	if n == 1 {
		return singular
	}
	return pluralSuffix
}
