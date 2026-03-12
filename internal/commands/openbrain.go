package commands

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

// OpenBrainCmd returns the "openbrain" command for watching MEMORY.md changes.
func OpenBrainCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "openbrain",
		Short:   "Memory file change watcher for dashboard pane",
		Long:    "Watch MEMORY.md files for changes. In --pane mode, streams updates with ANSI styling.",
		GroupID: "CORE",
		RunE: func(cmd *cobra.Command, args []string) error {
			pane, _ := cmd.Flags().GetBool("pane")
			projectDir, _ := cmd.Flags().GetString("project")
			jsonOut, _ := cmd.Root().Flags().GetBool("json")

			if pane {
				return runOpenBrainPane(cmd.Context(), projectDir)
			}

			return printOpenBrainSummary(projectDir, jsonOut)
		},
	}

	cmd.Flags().Bool("pane", false, "Dashboard pane mode (watch + stream ANSI)")
	cmd.Flags().String("project", "", "Override project directory for memory watch")

	return cmd
}

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

// runOpenBrainPane runs the OpenBrain pane in watch mode with ANSI output.
func runOpenBrainPane(ctx context.Context, projectDir string) error {
	if projectDir == "" {
		projectDir, _ = os.Getwd()
	}

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
		return runOpenBrainPoll(ctx, paths, hashes, sections, &recentEntries, maxRecent)
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

	// Initial render.
	renderOpenBrainPane(recentEntries, paths)

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
				renderOpenBrainPane(recentEntries, paths)
			}
		case <-watcher.Errors:
			// Ignore watcher errors — continue polling.
		case <-pollTicker.C:
			for _, p := range paths {
				processOpenBrainChange(p, paths, hashes, sections, &recentEntries, maxRecent)
			}
			renderOpenBrainPane(recentEntries, paths)
		}
	}
}

// runOpenBrainPoll is the polling fallback when fsnotify is unavailable.
func runOpenBrainPoll(ctx context.Context, paths []string, hashes map[string]string,
	sections map[string]map[string]string, recentEntries *[]memoryEntry, maxRecent int) error {

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	renderOpenBrainPane(*recentEntries, paths)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for _, p := range paths {
				processOpenBrainChange(p, paths, hashes, sections, recentEntries, maxRecent)
			}
			renderOpenBrainPane(*recentEntries, paths)
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
