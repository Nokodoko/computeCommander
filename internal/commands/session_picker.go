package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// activeCmdrTab represents a running cmdr dashboard tab discovered via its CWD file.
type activeCmdrTab struct {
	Hash       string // 8-char md5 hash (the tab identifier)
	ProjectDir string // current project directory from the CWD file
}

// discoverActiveTabs scans /tmp/cmdr-<uid>-*-cwd files to find all running
// cmdr dashboard tabs. Each tab's agent-wrapper writes a CWD file with a
// hash derived from its INITIAL_DIR, so we can discover them and write
// the correct per-tab switch file.
//
// Validates liveness by checking if an inotifywait process is watching
// the CWD file (fp-wrapper and lazygit-wrapper use inotifywait).
func discoverActiveTabs() []activeCmdrTab {
	uid := os.Getuid()
	pattern := fmt.Sprintf("/tmp/cmdr-%d-*-cwd", uid)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	// The "active-cwd" file is not a tab — skip it.
	var tabs []activeCmdrTab
	for _, path := range matches {
		base := filepath.Base(path)
		// Format: cmdr-<uid>-<hash>-cwd
		// Skip the special active-cwd file.
		if strings.Contains(base, "active-cwd") {
			continue
		}
		// Extract the hash from the filename.
		prefix := fmt.Sprintf("cmdr-%d-", uid)
		rest := strings.TrimPrefix(base, prefix)
		hash := strings.TrimSuffix(rest, "-cwd")
		if hash == "" || hash == rest {
			continue
		}
		// Read the project dir from the file.
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		projectDir := strings.TrimSpace(string(data))
		if projectDir == "" {
			continue
		}
		// Validate liveness: check if an inotifywait process is watching this file.
		// The fp-wrapper and lazygit-wrapper both use inotifywait on the CWD file.
		if !isTabAlive(path) {
			continue
		}
		tabs = append(tabs, activeCmdrTab{Hash: hash, ProjectDir: projectDir})
	}
	return tabs
}

// isTabAlive checks if a dashboard tab is still running by looking for
// inotifywait processes watching the given CWD file path.
func isTabAlive(cwdFilePath string) bool {
	// Use pgrep to find inotifywait processes, then check if any watch this file.
	out, err := exec.Command("pgrep", "-a", "inotifywait").Output()
	if err != nil {
		// pgrep not found or no matches — assume alive to avoid false negatives.
		return true
	}
	return strings.Contains(string(out), cwdFilePath)
}

// SessionsCmd returns the "sessions" command for picking and switching Claude sessions.
func SessionsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "sessions",
		Short:   "Pick a Claude session to resume in the agent pane",
		Long:    "List all Claude Code sessions, select one via gum filter, and switch the agent pane to resume it.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := listClaudeSessions()
			if err != nil {
				return fmt.Errorf("list sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No Claude sessions found.")
				return nil
			}

			// Build display lines for gum filter.
			displayLines := make([]string, len(sessions))
			for i, s := range sessions {
				name := strings.ReplaceAll(s.SessionName, "\n", " ")
				displayLines[i] = name
			}

			// Run gum filter.
			selected, err := gumFilter(displayLines, "Claude Sessions")
			if err != nil {
				return fmt.Errorf("gum filter: %w", err)
			}
			if selected == "" {
				return nil // user cancelled
			}

			// Find the matching session.
			var match *claudeSession
			for i, line := range displayLines {
				if line == selected {
					match = &sessions[i]
					break
				}
			}
			if match == nil {
				return fmt.Errorf("session not found: %s", selected)
			}

			// Determine which dashboard tab to target.
			home, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return fmt.Errorf("get home directory: %w", homeErr)
			}
			switchDir := filepath.Join(home, ".computecommander")
			if err := os.MkdirAll(switchDir, 0o755); err != nil {
				return fmt.Errorf("create switch dir: %w", err)
			}
			content := fmt.Sprintf("%s\n%s\n", match.ProjectPath, match.SessionID)

			// Discover all active cmdr dashboard tabs.
			tabs := discoverActiveTabs()

			var targetHash string

			// Best: check CMDR_TAB_HASH env var (set by wrapper scripts,
			// available when running from a cmdr pane directly).
			if envHash := os.Getenv("CMDR_TAB_HASH"); envHash != "" {
				for _, tab := range tabs {
					if tab.Hash == envHash {
						targetHash = envHash
						break
					}
				}
			}

			// One tab: use it directly. Multiple tabs: always show picker.
			// CWD matching is unreliable (split panes may have a CWD that
			// matches a different tab), so we don't attempt it.
			if targetHash == "" && len(tabs) == 1 {
				targetHash = tabs[0].Hash
			} else if targetHash == "" && len(tabs) > 1 {
				tabLines := make([]string, len(tabs))
				for i, tab := range tabs {
					tabLines[i] = fmt.Sprintf("[%s] %s", tab.Hash, filepath.Base(tab.ProjectDir))
				}
				picked, err := gumFilter(tabLines, "Which dashboard tab?")
				if err != nil {
					return fmt.Errorf("tab picker: %w", err)
				}
				if picked == "" {
					return nil // user cancelled
				}
				for _, tab := range tabs {
					label := fmt.Sprintf("[%s] %s", tab.Hash, filepath.Base(tab.ProjectDir))
					if label == picked {
						targetHash = tab.Hash
						break
					}
				}
			}

			// Write the per-tab switch file for the targeted tab.
			if targetHash != "" {
				perTabPath := filepath.Join(switchDir, "session-switch-"+targetHash)
				if err := os.WriteFile(perTabPath, []byte(content), 0o644); err != nil {
					return fmt.Errorf("write per-tab switch file: %w", err)
				}
			}

			// Also write global switch file as fallback.
			globalSwitchPath := filepath.Join(switchDir, "session-switch")
			if err := os.WriteFile(globalSwitchPath, []byte(content), 0o644); err != nil {
				return fmt.Errorf("write switch file: %w", err)
			}

			fmt.Printf("Switching to session: %s\n", match.SessionName)
			fmt.Printf("Project: %s\n", match.ProjectPath)
			fmt.Printf("Session: %s\n", match.SessionID)
			return nil
		},
	}
}

type claudeSession struct {
	SessionID   string
	ProjectPath string
	SessionName string
	Modified    float64
}

func listClaudeSessions() ([]claudeSession, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	projectsDir := filepath.Join(home, ".claude", "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []claudeSession
	indexedSessions := make(map[string]bool)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(projectsDir, entry.Name())

		// Try sessions-index.json first.
		indexPath := filepath.Join(projectDir, "sessions-index.json")
		if data, err := os.ReadFile(indexPath); err == nil {
			var index struct {
				Entries []struct {
					SessionID   string  `json:"sessionId"`
					ProjectPath string  `json:"projectPath"`
					CustomTitle string  `json:"customTitle"`
					Summary     string  `json:"summary"`
					FirstPrompt string  `json:"firstPrompt"`
					Modified    float64 `json:"modified"`
				} `json:"entries"`
			}
			if json.Unmarshal(data, &index) == nil {
				for _, e := range index.Entries {
					if e.ProjectPath == "" || e.SessionID == "" {
						continue
					}
					name := e.CustomTitle
					if name == "" {
						name = e.Summary
					}
					if name == "" && len(e.FirstPrompt) > 60 {
						name = e.FirstPrompt[:60]
					} else if name == "" {
						name = e.FirstPrompt
					}
					if name == "" {
						name = e.SessionID
					}
					indexedSessions[e.SessionID] = true
					sessions = append(sessions, claudeSession{
						SessionID:   e.SessionID,
						ProjectPath: e.ProjectPath,
						SessionName: name,
						Modified:    e.Modified,
					})
				}
			}
		}

		// Scan .jsonl files for sessions not in index.
		jsonlFiles, _ := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
		for _, jf := range jsonlFiles {
			sessionID := strings.TrimSuffix(filepath.Base(jf), ".jsonl")
			if strings.HasPrefix(sessionID, "agent-") || indexedSessions[sessionID] {
				continue
			}
			s := parseSessionJSONL(jf)
			if s != nil && len(s.SessionName) >= 3 {
				sessions = append(sessions, *s)
			}
		}
	}

	// Sort by modified (most recent first).
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified > sessions[j].Modified
	})

	return sessions, nil
}

func parseSessionJSONL(path string) *claudeSession {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	sessionID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	var customTitle, firstPrompt, projectPath string

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}

		if entry["type"] == "custom-title" {
			if t, ok := entry["customTitle"].(string); ok {
				customTitle = t
			}
		}

		if entry["type"] == "user" && projectPath == "" {
			if cwd, ok := entry["cwd"].(string); ok {
				projectPath = cwd
			}
		}

		if entry["type"] == "user" && firstPrompt == "" {
			if msg, ok := entry["message"].(map[string]any); ok {
				if content, ok := msg["content"].([]any); ok {
					for _, item := range content {
						if m, ok := item.(map[string]any); ok && m["type"] == "text" {
							if text, ok := m["text"].(string); ok {
								if len(text) > 60 {
									firstPrompt = text[:60]
								} else {
									firstPrompt = text
								}
								break
							}
						}
					}
				}
			}
		}
	}

	info, _ := os.Stat(path)
	modified := float64(0)
	if info != nil {
		modified = float64(info.ModTime().Unix())
	}

	name := customTitle
	if name == "" {
		name = firstPrompt
	}
	if name == "" {
		name = sessionID
	}

	if projectPath == "" {
		return nil
	}

	return &claudeSession{
		SessionID:   sessionID,
		ProjectPath: projectPath,
		SessionName: name,
		Modified:    modified,
	}
}

func gumFilter(items []string, header string) (string, error) {
	input := strings.Join(items, "\n")

	cmd := exec.Command("gum", "filter", "--header", header)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		// Exit code 130 = user cancelled (Ctrl+C)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return "", nil
		}
		// Exit code 1 = no selection
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", err
	}

	return strings.TrimSpace(string(output)), nil
}
