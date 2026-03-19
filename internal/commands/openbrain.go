package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

// ─── Knowledge entry types ───────────────────────────────────────────────────

// validEntryTypes lists the allowed entry_type values for openbrain_entries.
var validEntryTypes = map[string]bool{
	"decision":  true,
	"discovery": true,
	"warning":   true,
	"solution":  true,
	"context":   true,
}

// knowledgeEntry represents a row from the openbrain_entries table.
type knowledgeEntry struct {
	ID          int64  `json:"id"`
	ProjectName string `json:"project_name,omitempty"`
	EntryType   string `json:"type"`
	Summary     string `json:"summary"`
	Detail      string `json:"detail,omitempty"`
	Runtime     string `json:"runtime"`
	AgentName   string `json:"agent_name,omitempty"`
	Tags        string `json:"tags,omitempty"`
	CreatedAt   string `json:"created_at"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Age         string `json:"age,omitempty"`
}

// ─── Color coding (T5) ──────────────────────────────────────────────────────

// runtimeColor returns the ANSI color code for a given runtime.
func runtimeColor(runtime string) string {
	switch strings.ToLower(runtime) {
	case "claude":
		return "\033[34m" // blue
	case "pi":
		return "\033[35m" // magenta
	case "gemini":
		return "\033[36m" // cyan
	case "codex":
		return "\033[32m" // green
	case "goose":
		return "\033[33m" // yellow
	default:
		return "\033[2m" // dim
	}
}

// entryTypeGlyph returns the display glyph and ANSI color for an entry type.
func entryTypeGlyph(entryType string) (glyph string, color string) {
	switch entryType {
	case "decision":
		return "D", "\033[1;37m" // bold white
	case "discovery":
		return "?", "\033[36m" // cyan
	case "warning":
		return "!", "\033[1;33m" // bold yellow
	case "solution":
		return "S", "\033[32m" // green
	case "context":
		return "~", "\033[2m" // dim
	default:
		return "·", "\033[2m"
	}
}

// formatAge returns a human-readable age string like "2h ago", "1d ago".
func formatAge(createdAt string) string {
	t, err := time.Parse("2006-01-02 15:04:05", createdAt)
	if err != nil {
		// Try RFC3339 format.
		t, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return ""
		}
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ─── Project name derivation ─────────────────────────────────────────────────

// deriveProjectName determines the project name using the spec precedence:
// 1. Nearest parent with .computecommander/ directory
// 2. Git repo root basename
// 3. Current working directory basename
func deriveProjectName() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}

	// Walk up looking for .computecommander/.
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".computecommander")); err == nil {
			return filepath.Base(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Try git repo root.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		root := strings.TrimSpace(string(out))
		if root != "" {
			return filepath.Base(root)
		}
	}

	// Fallback to cwd basename.
	return filepath.Base(cwd)
}

// ─── TTL parsing ─────────────────────────────────────────────────────────────

// parseTTL parses a duration string like "7d", "24h", "30m" into a time.Duration.
func parseTTL(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty TTL")
	}
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, fmt.Errorf("invalid TTL days: %w", err)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	// Fall back to Go's time.ParseDuration for h, m, s.
	return time.ParseDuration(s)
}

// ─── OpenBrainCmd ────────────────────────────────────────────────────────────

// OpenBrainCmd returns the "openbrain" command for watching MEMORY.md changes.
func OpenBrainCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "openbrain",
		Short:   "Shared knowledge store and memory watcher for dashboard pane",
		Long:    "Watch MEMORY.md files for changes. Manage knowledge entries (write/read/prune). In --pane mode, streams updates with ANSI styling.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			pane, _ := cmd.Flags().GetBool("pane")
			projectDir, _ := cmd.Flags().GetString("project")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")
			showAgents, _ := cmd.Flags().GetBool("agents")
			noAgents, _ := cmd.Flags().GetBool("no-agents")
			agentLimit, _ := cmd.Flags().GetInt("agent-limit")

			// In pane mode, agents are shown by default unless --no-agents is set.
			if pane && !noAgents {
				showAgents = true
			}
			if noAgents {
				showAgents = false
			}

			if pane {
				return runOpenBrainPane(cmd.Context(), app, projectDir, showAgents, agentLimit)
			}

			return printOpenBrainSummary(projectDir, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Dashboard pane mode (watch + stream ANSI)")
	cmd.Flags().String("project", "", "Override project directory for memory watch")
	cmd.Flags().Bool("agents", false, "Include agent lifecycle events (default: true in --pane)")
	cmd.Flags().Bool("no-agents", false, "Suppress agent lifecycle events")
	cmd.Flags().Int("agent-limit", 5, "Max recent agent events to display")

	// Add subcommands.
	cmd.AddCommand(openBrainWriteCmd(app))
	cmd.AddCommand(openBrainReadCmd(app))
	cmd.AddCommand(openBrainPruneCmd(app))

	return cmd
}

// ─── Write subcommand (T2) ───────────────────────────────────────────────────

func openBrainWriteCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write",
		Short: "Write a knowledge entry to OpenBrain",
		Long:  "Insert a knowledge entry (decision, discovery, warning, solution, context) into the shared OpenBrain store.",
		RunE: func(cmd *cobra.Command, args []string) error {
			entryType, _ := cmd.Flags().GetString("type")
			summary, _ := cmd.Flags().GetString("summary")
			detail, _ := cmd.Flags().GetString("detail")
			project, _ := cmd.Flags().GetString("project")
			runtime, _ := cmd.Flags().GetString("runtime")
			agentName, _ := cmd.Flags().GetString("agent")
			tags, _ := cmd.Flags().GetString("tags")
			ttl, _ := cmd.Flags().GetString("ttl")

			// Validate entry type.
			if !validEntryTypes[entryType] {
				return fmt.Errorf("invalid entry type %q; must be one of: decision, discovery, warning, solution, context", entryType)
			}

			// Validate summary.
			if summary == "" {
				return fmt.Errorf("--summary is required")
			}

			// Derive project name if not provided.
			if project == "" {
				project = deriveProjectName()
			}

			// Default runtime from env or "claude".
			if runtime == "" {
				runtime = os.Getenv("CMDR_RUNTIME")
				if runtime == "" {
					runtime = "claude"
				}
			}

			// Parse TTL if provided.
			var expiresAt *string
			if ttl != "" {
				dur, err := parseTTL(ttl)
				if err != nil {
					return fmt.Errorf("invalid --ttl %q: %w", ttl, err)
				}
				t := time.Now().Add(dur).UTC().Format("2006-01-02 15:04:05")
				expiresAt = &t
			}

			if app == nil || app.DB == nil {
				return fmt.Errorf("database not available")
			}

			ctx := cmd.Context()

			// INSERT OR IGNORE for dedup on (project_name, entry_type, summary).
			query := `INSERT OR IGNORE INTO openbrain_entries
				(project_name, entry_type, summary, detail, runtime, agent_name, tags, expires_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

			err := app.DB.Exec(ctx, query,
				project, entryType, summary, detail, runtime, agentName, tags, expiresAt)
			if err != nil {
				return fmt.Errorf("write failed: %w", err)
			}

			fmt.Fprintf(os.Stderr, "ok: [%s] %s\n", entryType, summary)
			return nil
		},
	}

	cmd.Flags().String("type", "", "Entry type: decision, discovery, warning, solution, context (required)")
	cmd.Flags().String("summary", "", "One-line summary (max 80 chars, required)")
	cmd.Flags().String("detail", "", "Optional longer explanation (max 256 chars)")
	cmd.Flags().String("project", "", "Project name (default: auto-detected from cwd)")
	cmd.Flags().String("runtime", "", "Runtime that produced this entry (default: $CMDR_RUNTIME or claude)")
	cmd.Flags().String("agent", "", "Agent name that produced this entry")
	cmd.Flags().String("tags", "", "Comma-separated tags for future filtering")
	cmd.Flags().String("ttl", "", "Time-to-live duration (e.g., 7d, 24h); entry auto-expires after this")

	_ = cmd.MarkFlagRequired("type")
	_ = cmd.MarkFlagRequired("summary")

	return cmd
}

// ─── Read subcommand (T3) ────────────────────────────────────────────────────

func openBrainReadCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "read",
		Short: "Read recent knowledge entries from OpenBrain",
		Long:  "Query knowledge entries for the current project. Output in text (for context injection) or JSON mode.",
		RunE: func(cmd *cobra.Command, args []string) error {
			project, _ := cmd.Flags().GetString("project")
			limit, _ := cmd.Flags().GetInt("limit")
			since, _ := cmd.Flags().GetString("since")
			types, _ := cmd.Flags().GetString("types")
			jsonOut, _ := cmd.Flags().GetBool("json")

			if project == "" {
				project = deriveProjectName()
			}

			if app == nil || app.DB == nil {
				return fmt.Errorf("database not available")
			}

			ctx := cmd.Context()

			// Auto-prune expired entries first.
			_ = pruneExpiredEntries(ctx, app)

			entries, err := queryKnowledgeEntries(ctx, app, project, limit, since, types)
			if err != nil {
				return fmt.Errorf("read failed: %w", err)
			}

			if jsonOut {
				return json.NewEncoder(os.Stdout).Encode(map[string]any{
					"success": true,
					"command": "openbrain read",
					"project": project,
					"count":   len(entries),
					"entries": entries,
				})
			}

			// Text mode output for context injection.
			if len(entries) == 0 {
				fmt.Printf("## Recent OpenBrain Entries (%s)\n\nNo entries found.\n", project)
				return nil
			}

			fmt.Printf("## Recent OpenBrain Entries (%s)\n\n", project)
			for _, e := range entries {
				age := formatAge(e.CreatedAt)
				fmt.Printf("[%s] %s (%s) %s\n", e.EntryType, age, e.Runtime, e.Summary)
				if e.Detail != "" {
					fmt.Printf("  → %s\n", e.Detail)
				}
			}
			return nil
		},
	}

	cmd.Flags().String("project", "", "Project name (default: auto-detected from cwd)")
	cmd.Flags().Int("limit", 20, "Max entries to return")
	cmd.Flags().String("since", "72h", "Only entries from the last duration (e.g., 72h, 7d)")
	cmd.Flags().String("types", "", "Comma-separated entry types to filter (default: all)")
	cmd.Flags().Bool("json", false, "Output in JSON format for programmatic consumption")

	return cmd
}

// queryKnowledgeEntries queries the openbrain_entries table with filters.
func queryKnowledgeEntries(ctx context.Context, app *App, project string, limit int, since string, types string) ([]knowledgeEntry, error) {
	if app == nil || app.DB == nil {
		return nil, fmt.Errorf("database not available")
	}

	// Build the since threshold.
	var sinceTime time.Time
	if since != "" {
		dur, err := parseTTL(since)
		if err != nil {
			return nil, fmt.Errorf("invalid --since %q: %w", since, err)
		}
		sinceTime = time.Now().Add(-dur)
	}

	// Build query.
	var queryParts []string
	var queryArgs []any

	queryParts = append(queryParts, "project_name = ?")
	queryArgs = append(queryArgs, project)

	if !sinceTime.IsZero() {
		queryParts = append(queryParts, "created_at >= ?")
		queryArgs = append(queryArgs, sinceTime.UTC().Format("2006-01-02 15:04:05"))
	}

	if types != "" {
		typeList := strings.Split(types, ",")
		placeholders := make([]string, len(typeList))
		for i, t := range typeList {
			placeholders[i] = "?"
			queryArgs = append(queryArgs, strings.TrimSpace(t))
		}
		queryParts = append(queryParts, fmt.Sprintf("entry_type IN (%s)", strings.Join(placeholders, ",")))
	}

	if limit <= 0 {
		limit = 20
	}
	queryArgs = append(queryArgs, limit)

	query := fmt.Sprintf(
		`SELECT id, project_name, entry_type, summary, COALESCE(detail, ''), runtime, COALESCE(agent_name, ''), COALESCE(tags, ''), created_at, COALESCE(expires_at, '')
		FROM openbrain_entries
		WHERE %s
		ORDER BY created_at DESC
		LIMIT ?`,
		strings.Join(queryParts, " AND "),
	)

	rows, err := app.DB.Query(ctx, query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []knowledgeEntry
	for rows.Next() {
		var e knowledgeEntry
		if err := rows.Scan(&e.ID, &e.ProjectName, &e.EntryType, &e.Summary, &e.Detail, &e.Runtime, &e.AgentName, &e.Tags, &e.CreatedAt, &e.ExpiresAt); err != nil {
			continue
		}
		e.Age = formatAge(e.CreatedAt)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ─── Prune subcommand (T8) ──────────────────────────────────────────────────

func openBrainPruneCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove old or expired knowledge entries",
		Long:  "Delete entries older than the specified duration and any entries past their TTL.",
		RunE: func(cmd *cobra.Command, args []string) error {
			olderThan, _ := cmd.Flags().GetString("older-than")
			project, _ := cmd.Flags().GetString("project")

			if app == nil || app.DB == nil {
				return fmt.Errorf("database not available")
			}

			ctx := cmd.Context()

			// Always prune expired entries.
			expiredCount, err := pruneExpiredEntriesCount(ctx, app)
			if err != nil {
				return fmt.Errorf("prune expired: %w", err)
			}

			var agedCount int64
			if olderThan != "" {
				dur, err := parseTTL(olderThan)
				if err != nil {
					return fmt.Errorf("invalid --older-than %q: %w", olderThan, err)
				}
				cutoff := time.Now().Add(-dur).UTC().Format("2006-01-02 15:04:05")

				query := "DELETE FROM openbrain_entries WHERE created_at < ?"
				var queryArgs []any
				queryArgs = append(queryArgs, cutoff)

				if project != "" {
					query += " AND project_name = ?"
					queryArgs = append(queryArgs, project)
				}

				// We can't get affected rows easily with the DB interface,
				// so just execute and report success.
				if err := app.DB.Exec(ctx, query, queryArgs...); err != nil {
					return fmt.Errorf("prune old entries: %w", err)
				}
				// Report that we attempted the prune.
				agedCount = -1 // indicates "unknown count"
			}

			if agedCount == -1 {
				fmt.Fprintf(os.Stderr, "pruned expired: %d, pruned old: (done)\n", expiredCount)
			} else {
				fmt.Fprintf(os.Stderr, "pruned expired: %d\n", expiredCount)
			}
			return nil
		},
	}

	cmd.Flags().String("older-than", "", "Delete entries older than this duration (e.g., 7d, 30d)")
	cmd.Flags().String("project", "", "Limit pruning to a specific project")

	return cmd
}

// pruneExpiredEntries deletes entries where expires_at < now. Returns error only.
func pruneExpiredEntries(ctx context.Context, app *App) error {
	if app == nil || app.DB == nil {
		return nil
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	return app.DB.Exec(ctx,
		"DELETE FROM openbrain_entries WHERE expires_at IS NOT NULL AND expires_at < ?", now)
}

// pruneExpiredEntriesCount deletes expired entries and returns approximate count.
func pruneExpiredEntriesCount(ctx context.Context, app *App) (int64, error) {
	if app == nil || app.DB == nil {
		return 0, nil
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")

	// Count first, then delete.
	var count int64
	row := app.DB.QueryRow(ctx,
		"SELECT COUNT(*) FROM openbrain_entries WHERE expires_at IS NOT NULL AND expires_at < ?", now)
	if err := row.Scan(&count); err != nil {
		return 0, err
	}

	if count > 0 {
		if err := app.DB.Exec(ctx,
			"DELETE FROM openbrain_entries WHERE expires_at IS NOT NULL AND expires_at < ?", now); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// ─── Knowledge section renderer (T4) ────────────────────────────────────────

// renderKnowledgeSection renders the Knowledge section of the OpenBrain pane.
func renderKnowledgeSection(ctx context.Context, app *App, project string) {
	if app == nil || app.DB == nil {
		return
	}

	entries, err := queryKnowledgeEntries(ctx, app, project, 10, "72h", "")
	if err != nil || len(entries) == 0 {
		return
	}

	fmt.Printf("\n\033[2m─────────────────────────────────────────────\033[0m\n")
	fmt.Printf("\033[1;35m Knowledge \033[0m")
	fmt.Printf(" \033[2m(%d entries, last 72h)\033[0m\n", len(entries))

	for _, e := range entries {
		glyph, glyphColor := entryTypeGlyph(e.EntryType)
		age := formatAge(e.CreatedAt)
		rtColor := runtimeColor(e.Runtime)

		fmt.Printf(" %s%s\033[0m  %-7s %-45s %s%s\033[0m\n",
			glyphColor, glyph,
			age,
			truncate(e.Summary, 45),
			rtColor, truncate(e.Runtime, 8),
		)
	}
}

// renderAgentActivityDimmed renders a collapsed Activity section (last 3 events, dimmed).
func renderAgentActivityDimmed(entries []agentActivityEntry) {
	if len(entries) == 0 {
		return
	}

	// Show at most 3 entries in dim text.
	max := 3
	if len(entries) < max {
		max = len(entries)
	}

	fmt.Printf("\n\033[2m─────────────────────────────────────────────\033[0m\n")
	fmt.Printf("\033[2m Activity \033[0m")
	fmt.Printf(" \033[2m(%d recent)\033[0m\n", max)

	for i := 0; i < max; i++ {
		e := entries[i]
		shortType := e.EventType
		if idx := strings.LastIndex(shortType, "."); idx >= 0 {
			shortType = shortType[idx+1:]
		}
		shortTime := ""
		if len(e.Timestamp) >= 16 {
			shortTime = e.Timestamp[11:16]
		}

		fmt.Printf("\033[2m  %-12s %-14s %-8s %s\033[0m\n",
			shortType,
			truncate(e.AgentName, 14),
			truncate(e.Runtime, 8),
			shortTime,
		)
	}
}

// ─── Existing code (memory watcher, unchanged) ──────────────────────────────

// memoryEntry records a change detected in a MEMORY.md file.
type memoryEntry struct {
	File      string `json:"file"`
	Section   string `json:"section"`
	Operation string `json:"operation"`
	Timestamp string `json:"timestamp"`
	Preview   string `json:"preview"`
}

// openBrainMemoryDirs returns all memory directories to watch (across all projects).
func openBrainMemoryDirs() []string {
	home, _ := os.UserHomeDir()
	seen := make(map[string]bool)
	var dirs []string

	addDir := func(d string) {
		if !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}

	// Global Claude memory directory.
	globalDir := filepath.Join(home, ".claude")
	addDir(globalDir)

	// All project memory directories (regardless of session).
	projDir := filepath.Join(home, ".claude", "projects")
	if entries, err := os.ReadDir(projDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				memDir := filepath.Join(projDir, e.Name(), "memory")
				if info, err := os.Stat(memDir); err == nil && info.IsDir() {
					addDir(memDir)
				}
				// Also watch MEMORY.md at project root level.
				addDir(filepath.Join(projDir, e.Name()))
			}
		}
	}

	return dirs
}

// openBrainMemoryPaths returns all memory .md files across all projects.
func openBrainMemoryPaths(projectDir string) []string {
	home, _ := os.UserHomeDir()
	seen := make(map[string]bool)
	var paths []string

	addPath := func(p string) {
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	// Global Claude memory.
	addPath(filepath.Join(home, ".claude", "MEMORY.md"))

	// All project memory directories and their .md files (regardless of session).
	projDir := filepath.Join(home, ".claude", "projects")
	if entries, err := os.ReadDir(projDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// MEMORY.md at project level.
			addPath(filepath.Join(projDir, e.Name(), "MEMORY.md"))

			// All .md files in the memory/ subdirectory.
			memDir := filepath.Join(projDir, e.Name(), "memory")
			if mdFiles, err := os.ReadDir(memDir); err == nil {
				for _, mf := range mdFiles {
					if !mf.IsDir() && strings.HasSuffix(mf.Name(), ".md") {
						addPath(filepath.Join(memDir, mf.Name()))
					}
				}
			}
		}
	}

	return paths
}

// hashFileContent returns the SHA-256 hash of a file's content.
func hashFileContent(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

// extractSections parses MEMORY.md into heading -> content map.
func extractSections(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	sections := make(map[string]string)
	var currentHeading string
	var currentContent strings.Builder

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			if currentHeading != "" {
				sections[currentHeading] = currentContent.String()
			}
			currentHeading = strings.TrimSpace(line)
			currentContent.Reset()
		} else {
			currentContent.WriteString(line)
			currentContent.WriteString("\n")
		}
	}
	if currentHeading != "" {
		sections[currentHeading] = currentContent.String()
	}

	return sections
}

// diffSections compares old and new section maps, returning memory entries for changes.
func diffSections(path string, oldSections, newSections map[string]string) []memoryEntry {
	var entries []memoryEntry
	now := time.Now().Format(time.RFC3339)

	// Check for added or modified sections.
	for heading, newContent := range newSections {
		oldContent, existed := oldSections[heading]
		if !existed {
			entries = append(entries, memoryEntry{
				File:      path,
				Section:   heading,
				Operation: "added",
				Timestamp: now,
				Preview:   truncate(strings.TrimSpace(newContent), 80),
			})
		} else if oldContent != newContent {
			entries = append(entries, memoryEntry{
				File:      path,
				Section:   heading,
				Operation: "modified",
				Timestamp: now,
				Preview:   truncate(strings.TrimSpace(newContent), 80),
			})
		}
	}

	// Check for deleted sections.
	for heading := range oldSections {
		if _, exists := newSections[heading]; !exists {
			entries = append(entries, memoryEntry{
				File:      path,
				Section:   heading,
				Operation: "deleted",
				Timestamp: now,
				Preview:   "",
			})
		}
	}

	return entries
}

// printOpenBrainSummary prints a one-shot snapshot of memory files.
func printOpenBrainSummary(projectDir string, jsonOut bool) error {
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	paths := openBrainMemoryPaths(projectDir)
	var entries []memoryEntry

	for _, p := range paths {
		sections := extractSections(p)
		for heading, content := range sections {
			entries = append(entries, memoryEntry{
				File:      p,
				Section:   heading,
				Operation: "present",
				Timestamp: time.Now().Format(time.RFC3339),
				Preview:   truncate(strings.TrimSpace(content), 80),
			})
		}
	}

	if jsonOut {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"success": true,
			"command": "openbrain",
			"entries": entries,
			"count":   len(entries),
		})
	}

	if len(entries) == 0 {
		fmt.Println("No memory files found.")
		return nil
	}

	fmt.Printf("%-40s %-30s %-10s\n", "FILE", "SECTION", "OP")
	for _, e := range entries {
		fmt.Printf("%-40s %-30s %-10s\n",
			truncate(e.File, 40),
			truncate(e.Section, 30),
			e.Operation,
		)
	}
	return nil
}

// agentActivityEntry represents an agent lifecycle event for the Activity section.
type agentActivityEntry struct {
	EventType  string
	AgentName  string
	Runtime    string
	Capability string
	Timestamp  string
}

// queryAgentActivity queries recent agent lifecycle events from the events table.
func queryAgentActivity(ctx context.Context, app *App, limit int) []agentActivityEntry {
	if app == nil || app.DB == nil {
		return nil
	}

	query := `SELECT event_type, agent_name, COALESCE(data, ''), created_at
		FROM events
		WHERE event_type LIKE 'agent.%'
		ORDER BY created_at DESC
		LIMIT ?`

	rows, err := app.DB.Query(ctx, query, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []agentActivityEntry
	for rows.Next() {
		var e agentActivityEntry
		var data string
		if err := rows.Scan(&e.EventType, &e.AgentName, &data, &e.Timestamp); err != nil {
			continue
		}
		// Parse runtime and capability from data field (format: "runtime=X capability=Y").
		for _, part := range strings.Split(data, " ") {
			kv := strings.SplitN(part, "=", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "runtime":
				e.Runtime = kv[1]
			case "capability":
				e.Capability = kv[1]
			}
		}
		entries = append(entries, e)
	}
	return entries
}

// renderAgentActivity renders the Activity section of the OpenBrain pane (full version).
func renderAgentActivity(entries []agentActivityEntry) {
	if len(entries) == 0 {
		return
	}

	fmt.Printf("\n\033[2m─────────────────────────────────────────────\033[0m\n")
	fmt.Printf("\033[1;35m Activity \033[0m")
	fmt.Printf(" \033[2m(%d/%d)\033[0m\n", len(entries), len(entries))

	for _, e := range entries {
		// Extract short event type (e.g., "agent.registered" -> "registered").
		shortType := e.EventType
		if idx := strings.LastIndex(shortType, "."); idx >= 0 {
			shortType = shortType[idx+1:]
		}

		// Extract short time (HH:MM).
		shortTime := ""
		if len(e.Timestamp) >= 16 {
			shortTime = e.Timestamp[11:16]
		}

		// Color the event type.
		var typeColor string
		switch shortType {
		case "registered":
			typeColor = "\033[32m" // green
		case "working":
			typeColor = "\033[36m" // cyan
		case "completed", "deregistered":
			typeColor = "\033[33m" // yellow
		case "stalled":
			typeColor = "\033[31m" // red
		default:
			typeColor = "\033[2m"
		}

		fmt.Printf("  %s%-12s\033[0m %-14s %-8s %-10s %s\n",
			typeColor, shortType,
			truncate(e.AgentName, 14),
			truncate(e.Runtime, 8),
			truncate(e.Capability, 10),
			shortTime,
		)
	}
}

// runOpenBrainPane runs the OpenBrain pane in watch mode with ANSI output.
func runOpenBrainPane(ctx context.Context, app *App, projectDir string, showAgents bool, agentLimit int) error {
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

	// Derive project name for knowledge queries.
	projectName := deriveProjectName()

	// Track content hashes and sections per file.
	hashes := make(map[string]string)
	sections := make(map[string]map[string]string)
	var recentEntries []memoryEntry
	const maxRecent = 20

	paths := openBrainMemoryPaths(projectDir)

	// Snapshot initial state and populate recentEntries with existing sections.
	now := time.Now().Format(time.RFC3339)
	for _, p := range paths {
		hashes[p] = hashFileContent(p)
		sections[p] = extractSections(p)
		for heading, content := range sections[p] {
			recentEntries = append(recentEntries, memoryEntry{
				File:      p,
				Section:   heading,
				Operation: "present",
				Timestamp: now,
				Preview:   truncate(strings.TrimSpace(content), 80),
			})
		}
	}
	if len(recentEntries) > maxRecent {
		recentEntries = recentEntries[len(recentEntries)-maxRecent:]
	}

	// Try fsnotify, fall back to polling.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return runOpenBrainPoll(ctx, app, projectName, paths, hashes, sections, &recentEntries, maxRecent, showAgents, agentLimit)
	}
	defer watcher.Close()

	// Watch all memory directories (catches new files from any session).
	for _, dir := range openBrainMemoryDirs() {
		_ = watcher.Add(dir) // Best effort — missing dirs are fine.
	}
	// Also watch directories containing known memory files.
	for _, p := range paths {
		dir := filepath.Dir(p)
		_ = watcher.Add(dir)
	}

	// Render helper that includes Knowledge section and dimmed Activity.
	renderAll := func() {
		renderOpenBrainPane(recentEntries, paths)
		renderKnowledgeSection(ctx, app, projectName)
		if showAgents {
			agentEvents := queryAgentActivity(ctx, app, agentLimit)
			renderAgentActivityDimmed(agentEvents)
		}
	}

	// Initial render.
	renderAll()

	// Polling fallback ticker for CWD changes.
	pollTicker := time.NewTicker(5 * time.Second)
	defer pollTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Re-scan paths to pick up newly created memory files.
				if event.Has(fsnotify.Create) && strings.HasSuffix(event.Name, ".md") {
					paths = openBrainMemoryPaths("")
				}
				processOpenBrainChange(event.Name, paths, hashes, sections, &recentEntries, maxRecent)
				renderAll()
			}
		case <-watcher.Errors:
			// Ignore watcher errors — continue polling.
		case <-pollTicker.C:
			for _, p := range paths {
				processOpenBrainChange(p, paths, hashes, sections, &recentEntries, maxRecent)
			}
			renderAll()
		}
	}
}

// runOpenBrainPoll is the polling fallback when fsnotify is unavailable.
func runOpenBrainPoll(ctx context.Context, app *App, projectName string, paths []string, hashes map[string]string,
	sections map[string]map[string]string, recentEntries *[]memoryEntry, maxRecent int,
	showAgents bool, agentLimit int) error {

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	renderOpenBrainPane(*recentEntries, paths)
	renderKnowledgeSection(ctx, app, projectName)
	if showAgents {
		agentEvents := queryAgentActivity(ctx, app, agentLimit)
		renderAgentActivityDimmed(agentEvents)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, p := range paths {
				processOpenBrainChange(p, paths, hashes, sections, recentEntries, maxRecent)
			}
			renderOpenBrainPane(*recentEntries, paths)
			renderKnowledgeSection(ctx, app, projectName)
			if showAgents {
				agentEvents := queryAgentActivity(ctx, app, agentLimit)
				renderAgentActivityDimmed(agentEvents)
			}
		}
	}
}

// processOpenBrainChange checks a single file for changes and appends entries.
func processOpenBrainChange(changedFile string, paths []string, hashes map[string]string,
	sections map[string]map[string]string, recentEntries *[]memoryEntry, maxRecent int) {

	// Only process files we're tracking.
	tracked := false
	for _, p := range paths {
		if p == changedFile {
			tracked = true
			break
		}
	}
	if !tracked {
		return
	}

	newHash := hashFileContent(changedFile)
	if newHash == hashes[changedFile] || newHash == "" {
		return
	}

	oldSections := sections[changedFile]
	if oldSections == nil {
		oldSections = make(map[string]string)
	}
	newSections := extractSections(changedFile)

	diffs := diffSections(changedFile, oldSections, newSections)
	if len(diffs) > 0 {
		*recentEntries = append(*recentEntries, diffs...)
		// Trim to maxRecent.
		if len(*recentEntries) > maxRecent {
			*recentEntries = (*recentEntries)[len(*recentEntries)-maxRecent:]
		}
	}

	hashes[changedFile] = newHash
	sections[changedFile] = newSections
}

// renderOpenBrainPane renders one frame of the OpenBrain pane.
func renderOpenBrainPane(entries []memoryEntry, watchedPaths []string) {
	// Clear screen and move cursor to top.
	fmt.Print("\033[2J\033[H")

	// Header.
	fmt.Print("\033[1;35m Memory \033[0m")
	fmt.Printf(" \033[2m%s\033[0m", time.Now().Format("15:04:05"))

	// Count existing files.
	existing := 0
	for _, p := range watchedPaths {
		if _, err := os.Stat(p); err == nil {
			existing++
		}
	}
	fmt.Printf(" \033[2m(%d files)\033[0m\n", existing)

	if len(entries) == 0 {
		fmt.Print("\033[2m Watching for changes...\033[0m\n")
		return
	}

	// Show recent entries (newest first).
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		opColor := openBrainOpColor(e.Operation)
		// Show shortened file path.
		shortFile := filepath.Base(filepath.Dir(e.File)) + "/" + filepath.Base(e.File)
		fmt.Printf(" %s%-8s\033[0m \033[2m%s\033[0m %s\n",
			opColor, e.Operation, shortFile, truncate(e.Section, 30))
		if e.Preview != "" {
			fmt.Printf("          \033[2m%s\033[0m\n", truncate(e.Preview, 60))
		}
	}
}

// openBrainOpColor returns ANSI color for an operation type.
func openBrainOpColor(op string) string {
	switch op {
	case "added":
		return "\033[32m" // green
	case "modified":
		return "\033[33m" // yellow
	case "deleted":
		return "\033[31m" // red
	default:
		return "\033[2m" // dim
	}
}
