package linkedin

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Scanner reads local project files to extract technical insights for post generation.
type Scanner struct {
	cfg Config
}

// NewScanner creates a Scanner with the given configuration.
func NewScanner(cfg Config) *Scanner {
	return &Scanner{cfg: cfg}
}

// ScanAll scans all configured projects and returns insights for each.
func (s *Scanner) ScanAll() ([]ProjectInsight, error) {
	var insights []ProjectInsight

	for name, path := range s.cfg.Projects {
		if path == "" {
			continue
		}
		insight, err := s.ScanProject(name, path)
		if err != nil {
			// Non-fatal: skip projects that can't be scanned.
			insight = &ProjectInsight{
				Project:     name,
				Path:        path,
				KeyPatterns: []string{fmt.Sprintf("scan error: %v", err)},
			}
		}
		insights = append(insights, *insight)
	}

	// Scan hooks directory separately.
	if s.cfg.HooksDir != "" {
		hookInsight, err := s.scanHooks(s.cfg.HooksDir)
		if err == nil {
			insights = append(insights, *hookInsight)
		}
	}

	return insights, nil
}

// ScanProject extracts insights from a single project directory.
func (s *Scanner) ScanProject(name, path string) (*ProjectInsight, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("project path %s: %w", path, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path %s is not a directory", path)
	}

	insight := &ProjectInsight{
		Project: name,
		Path:    path,
	}

	// Get recent git log (last 20 commits).
	insight.CommitLog = s.gitLog(path, 20)

	// Get recently modified files.
	insight.RecentFiles = s.recentFiles(path)

	// Read architecture docs if present.
	insight.Architecture = s.readArchDocs(path)

	// Extract key patterns from Go source.
	insight.KeyPatterns = s.extractPatterns(path)

	// Extract data points (counts, metrics).
	insight.DataPoints = s.extractDataPoints(path)

	return insight, nil
}

// gitLog returns the recent commit log for a project.
func (s *Scanner) gitLog(path string, count int) string {
	cmd := exec.Command("git", "-C", path, "log",
		fmt.Sprintf("--max-count=%d", count),
		"--oneline", "--no-decorate")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// recentFiles returns files modified in the last 7 days.
func (s *Scanner) recentFiles(path string) []string {
	cmd := exec.Command("git", "-C", path, "diff", "--name-only", "HEAD~10", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var files []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

// readArchDocs reads CLAUDE.md, README.md, and spec files for architecture context.
func (s *Scanner) readArchDocs(path string) string {
	var parts []string

	archFiles := []string{
		"CLAUDE.md",
		"README.md",
		filepath.Join("specs", "index.md"),
	}

	for _, f := range archFiles {
		full := filepath.Join(path, f)
		data, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		// Truncate to 2000 chars per file to stay within context budget.
		content := string(data)
		if len(content) > 2000 {
			content = content[:2000] + "\n... (truncated)"
		}
		parts = append(parts, fmt.Sprintf("=== %s ===\n%s", f, content))
	}

	return strings.Join(parts, "\n\n")
}

// extractPatterns looks for architectural patterns in Go source files.
func (s *Scanner) extractPatterns(path string) []string {
	patterns := make(map[string]bool)

	// Use rg for fast pattern detection.
	searches := map[string]string{
		"interface{}":   "interface definitions",
		"type.*Store":   "store pattern",
		"func.*Handler": "handler pattern",
		"go func":       "goroutine concurrency",
		"chan ":          "channel-based communication",
		"context.":      "context propagation",
		"embed.FS":      "embedded filesystem",
		"MCP":           "MCP integration",
	}

	for pattern, label := range searches {
		cmd := exec.Command("rg", "-l", "--max-count=1", pattern, path)
		if err := cmd.Run(); err == nil {
			patterns[label] = true
		}
	}

	var result []string
	for p := range patterns {
		result = append(result, p)
	}
	return result
}

// extractDataPoints extracts quantitative data from a project.
func (s *Scanner) extractDataPoints(path string) []string {
	var points []string

	// Count Go files.
	cmd := exec.Command("sh", "-c", fmt.Sprintf("find %s -name '*.go' -not -path '*/vendor/*' | wc -l", path))
	if out, err := cmd.Output(); err == nil {
		count := strings.TrimSpace(string(out))
		if count != "0" {
			points = append(points, fmt.Sprintf("%s Go files", count))
		}
	}

	// Count total lines of Go code.
	cmd = exec.Command("sh", "-c", fmt.Sprintf("find %s -name '*.go' -not -path '*/vendor/*' -exec cat {} + 2>/dev/null | wc -l", path))
	if out, err := cmd.Output(); err == nil {
		count := strings.TrimSpace(string(out))
		if count != "0" {
			points = append(points, fmt.Sprintf("%s lines of Go", count))
		}
	}

	// Count test files.
	cmd = exec.Command("sh", "-c", fmt.Sprintf("find %s -name '*_test.go' | wc -l", path))
	if out, err := cmd.Output(); err == nil {
		count := strings.TrimSpace(string(out))
		if count != "0" {
			points = append(points, fmt.Sprintf("%s test files", count))
		}
	}

	return points
}

// scanHooks scans the Claude Code hooks directory for insights.
func (s *Scanner) scanHooks(dir string) (*ProjectInsight, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("hooks dir %s: not accessible", dir)
	}

	insight := &ProjectInsight{
		Project: "Claude Code Hooks",
		Path:    dir,
	}

	// Count hook files.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var hookFiles []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".sh") || strings.HasSuffix(e.Name(), ".py")) {
			hookFiles = append(hookFiles, e.Name())
		}
	}

	insight.DataPoints = []string{fmt.Sprintf("%d hook scripts", len(hookFiles))}
	insight.RecentFiles = hookFiles

	// Extract hook categories.
	categories := make(map[string]int)
	for _, f := range hookFiles {
		parts := strings.SplitN(f, "-", 2)
		if len(parts) > 0 {
			categories[parts[0]]++
		}
	}

	for cat, count := range categories {
		insight.KeyPatterns = append(insight.KeyPatterns,
			fmt.Sprintf("%s hooks: %d", cat, count))
	}

	return insight, nil
}
