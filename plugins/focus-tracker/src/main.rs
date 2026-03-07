use std::collections::BTreeMap;
use zellij_tile::prelude::*;

/// Zellij WASM plugin that tracks the focused agent workspace pane in a cmdr
/// dashboard tab and writes its project path to the per-tab CWD file.
///
/// When the user switches focus between agent workspace panes, this plugin
/// detects the focus change via PaneUpdate, determines the project path,
/// and writes it to /tmp/cmdr-<uid>-<hash>-cwd.
///
/// The fp-wrapper and lazygit-wrapper watch that CWD file via inotifywait
/// and restart their tools pointed at the new project directory.
///
/// Agent pane detection strategies (any match = agent pane):
/// 1. terminal_command containing the agent-wrapper marker path
/// 2. Pane title with "CMDR:<project_path>" prefix (set by agent-wrapper)
/// 3. Pane title starting with "_ " (set by gum launcher / Claude Code)
/// 4. terminal_command ending with "/claude" or equal to "claude"
///
/// Project path resolution (tried in order):
/// 1. Parse title for CMDR:<path> prefix (dynamically updated, reflects current project)
/// 2. Parse terminal_command for wrapper marker path (static, reflects launch-time project)
/// 3. For "_ " panes: async lookup via Claude session index files
/// 4. Fall back to configured project_dir (from layout plugin config)
///
/// No pgrep, no /proc scanning — just PaneInfo fields from the zellij API.

struct FocusTracker {
    /// 8-char hex tab hash from plugin config, used to construct the CWD file path.
    tab_hash: String,
    /// Project directory from plugin config, used as fallback for non-wrapper panes.
    project_dir: String,
    /// The last project path we wrote to the CWD file (to detect changes).
    last_project_path: String,
    /// The tab index our plugin lives in (discovered via PaneUpdate).
    own_tab_index: Option<usize>,
    /// Our own plugin pane ID (from get_plugin_ids).
    own_pane_id: u32,
    /// Whether we've written the initial CWD. On the first PaneUpdate where an
    /// agent pane is found, always write even if path matches last_project_path.
    initial_cwd_written: bool,
    /// The last focused pane title we saw, used to avoid re-firing resolve_path
    /// on every PaneUpdate when the title hasn't changed.
    last_focused_title: String,
    /// Whether we have an in-flight resolve_path command (avoid duplicates).
    resolve_in_flight: bool,
}

impl FocusTracker {
    /// Marker in terminal_command that identifies an agent-wrapper pane.
    const WRAPPER_MARKER: &'static str = "/.computecommander/scripts/cmdr-agent-wrapper.sh";

    /// Title prefix set by agent-wrapper via printf escape sequence.
    const TITLE_PREFIX: &'static str = "CMDR:";

    /// Title prefix for panes launched by gum launcher / Claude Code sessions.
    const SESSION_PREFIX: &'static str = "_ ";

    /// Sparkle prefix Claude adds to some pane titles.
    const SPARKLE_PREFIX: &'static str = "\u{2733} ";

    /// Extract the project path from a single pane's terminal_command.
    /// The command looks like: "bash /path/to/project/.computecommander/scripts/cmdr-agent-wrapper.sh HASH"
    /// The project path is everything before the wrapper marker.
    fn extract_path_from_command(cmd: &str) -> Option<String> {
        let marker_pos = cmd.find(Self::WRAPPER_MARKER)?;
        let path_portion = &cmd[..marker_pos];
        // The path starts at the last space before the marker (skipping "bash " prefix).
        let path_start = path_portion.rfind(' ').map(|i| i + 1).unwrap_or(0);
        let project_path = &cmd[path_start..marker_pos];
        if project_path.starts_with('/') {
            Some(project_path.to_string())
        } else {
            None
        }
    }

    /// Extract the project path from a pane's title if it has the CMDR: prefix.
    fn extract_path_from_title(title: &str) -> Option<String> {
        let path = title.strip_prefix(Self::TITLE_PREFIX)?;
        let path = path.trim();
        if path.starts_with('/') && !path.is_empty() {
            Some(path.to_string())
        } else {
            None
        }
    }

    /// Check if terminal_command refers to a claude binary.
    /// Matches "claude", "/path/to/claude", etc.
    fn is_claude_command(cmd: &str) -> bool {
        let bin = cmd.split_whitespace().next().unwrap_or("");
        bin == "claude" || bin.ends_with("/claude")
    }

    /// Extract the session name from a "_ " prefixed pane title.
    /// Strips the "_ " prefix and any leading sparkle emoji.
    fn extract_session_name(title: &str) -> Option<String> {
        let name = title.strip_prefix(Self::SESSION_PREFIX)?;
        let name = name.strip_prefix(Self::SPARKLE_PREFIX).unwrap_or(name);
        let name = name.trim();
        if name.is_empty() {
            None
        } else {
            Some(name.to_string())
        }
    }

    /// Check if a "_ " pane's session name matches the current project_dir's basename.
    fn session_name_matches_project_dir(&self, name: &str) -> bool {
        if self.project_dir.is_empty() {
            return false;
        }
        if let Some(basename) = self.project_dir.rsplit('/').next() {
            if !basename.is_empty() && basename == name {
                return true;
            }
        }
        false
    }

    /// Try to extract a project path from a pane using all available strategies.
    /// Falls back to the configured project_dir for panes that are identified
    /// as agent panes but don't carry path info (e.g., bare "claude" panes).
    ///
    /// For "_ " panes with a session name that differs from project_dir's basename,
    /// returns None to signal that an async resolve_path lookup should be fired.
    fn extract_project_path(&self, pane: &PaneInfo) -> Option<String> {
        // Strategy 1: Check title for CMDR:<path> prefix first.
        // The title is dynamically updated by the agent-wrapper when the agent
        // navigates to a new project, so it reflects the CURRENT project path.
        // This must take priority over terminal_command which only reflects the
        // launch-time project path embedded in the wrapper script path.
        if let Some(path) = Self::extract_path_from_title(&pane.title) {
            return Some(path);
        }

        // Strategy 2: Parse the terminal_command for the wrapper marker.
        // This is a fallback for panes whose title hasn't been set yet
        // (e.g., during initial load before the agent-wrapper runs printf).
        if let Some(cmd) = pane.terminal_command.as_deref() {
            if let Some(path) = Self::extract_path_from_command(cmd) {
                return Some(path);
            }
        }

        // Strategy 3: For "_ " titled panes, check if name matches current project.
        // If it does, use project_dir. If not, return None to trigger async resolve.
        if pane.title.starts_with(Self::SESSION_PREFIX) {
            if let Some(name) = Self::extract_session_name(&pane.title) {
                if self.session_name_matches_project_dir(&name) {
                    return Some(self.project_dir.clone());
                }
                // Name doesn't match — caller should fire async resolve_path.
                return None;
            }
            // No extractable name (just "_ ") — fall back to project_dir.
            if !self.project_dir.is_empty() {
                return Some(self.project_dir.clone());
            }
            return None;
        }

        // Strategy 4: For bare claude panes, use configured project_dir.
        if !self.project_dir.is_empty() {
            if let Some(cmd) = pane.terminal_command.as_deref() {
                if Self::is_claude_command(cmd) {
                    return Some(self.project_dir.clone());
                }
            }
        }

        None
    }

    /// Check if a pane is an agent workspace pane.
    fn is_agent_pane(pane: &PaneInfo) -> bool {
        if pane.is_plugin {
            return false;
        }
        // Check terminal_command for wrapper marker.
        if let Some(cmd) = pane.terminal_command.as_deref() {
            if cmd.contains(Self::WRAPPER_MARKER) {
                return true;
            }
        }
        // Check title for CMDR: prefix (agent-wrapper sets this).
        if pane.title.starts_with(Self::TITLE_PREFIX) {
            return true;
        }
        // Check title for "_ " prefix (gum launcher / Claude Code sessions).
        if pane.title.starts_with(Self::SESSION_PREFIX) {
            return true;
        }
        // Check if pane is running a claude binary directly.
        if let Some(cmd) = pane.terminal_command.as_deref() {
            if Self::is_claude_command(cmd) {
                return true;
            }
        }
        false
    }

    /// Check if the focused pane is a "_ " session pane that needs async resolution.
    /// Returns the session name if resolution should be fired.
    fn needs_async_resolve(&self, panes: &[PaneInfo]) -> Option<String> {
        let focused = panes.iter().find(|p| p.is_focused && !p.is_plugin)?;
        if !focused.title.starts_with(Self::SESSION_PREFIX) {
            return None;
        }
        let name = Self::extract_session_name(&focused.title)?;
        if self.session_name_matches_project_dir(&name) {
            return None;
        }
        Some(name)
    }

    /// Fire an async run_command to resolve a session name to a project path
    /// by searching Claude's session index files.
    fn fire_resolve_path(&mut self, session_name: &str) {
        let script = format!(
            r#"NAME={name}
for f in ~/.claude/projects/*/sessions-index.json; do
  [ -f "$f" ] || continue
  match=$(jq -r --arg name "$NAME" '.entries[] | select(.customTitle == $name or .summary == $name) | .projectPath' "$f" 2>/dev/null | head -1)
  if [ -n "$match" ] && [ -d "$match" ]; then
    echo "$match"
    exit 0
  fi
  match=$(jq -r --arg name "$NAME" '.entries[] | select((.projectPath | split("/") | last) == $name) | .projectPath' "$f" 2>/dev/null | head -1)
  if [ -n "$match" ] && [ -d "$match" ]; then
    echo "$match"
    exit 0
  fi
done"#,
            name = shlex_quote(session_name),
        );
        let parts = vec!["bash".to_string(), "-c".to_string(), script];
        let cmd_refs: Vec<&str> = parts.iter().map(|s| s.as_str()).collect();

        let mut ctx = BTreeMap::new();
        ctx.insert("action".to_string(), "resolve_path".to_string());

        self.resolve_in_flight = true;
        run_command(&cmd_refs, ctx);
    }

    /// Find the project path for the currently focused agent workspace pane.
    ///
    /// Returns the project path of the focused pane if it is an agent workspace,
    /// or falls back to the first agent pane found (for initial load when focus
    /// may be on a non-agent pane like the fp pane).
    fn find_focused_project(&self, panes: &[PaneInfo]) -> Option<String> {
        // First: check if the focused pane IS an agent pane.
        let focused = panes.iter().find(|p| p.is_focused && !p.is_plugin);
        if let Some(fp) = focused {
            if Self::is_agent_pane(fp) {
                return self.extract_project_path(fp);
            }
        }

        // Fallback: if focus is on a non-agent pane (fp, lazygit, status, etc.),
        // use the first agent pane found. This handles initial load and the case
        // where the user clicks on fp or lazygit (we keep showing the same project).
        panes.iter()
            .filter(|p| Self::is_agent_pane(p))
            .find_map(|p| self.extract_project_path(p))
    }

    /// Discover which tab index we live in by scanning the PaneManifest for our
    /// own pane ID among plugin panes.
    fn discover_own_tab(&self, manifest: &PaneManifest) -> Option<usize> {
        for (tab_idx, panes) in &manifest.panes {
            for pane in panes {
                if pane.is_plugin && pane.id == self.own_pane_id {
                    return Some(*tab_idx);
                }
            }
        }
        None
    }

    /// Build the shell command that atomically writes `path` to the CWD file.
    fn build_write_command(path: &str, tab_hash: &str) -> Vec<String> {
        let script = format!(
            r#"UID_PREFIX="/tmp/cmdr-$(id -u)"
TAB_FILE="$UID_PREFIX-{hash}-cwd"
ACTIVE_FILE="$UID_PREFIX-active-cwd"
TMP="$TAB_FILE.tmp.$$"
printf '%s' {path} > "$TMP"
mv -f "$TMP" "$TAB_FILE"
cp -f "$TAB_FILE" "$ACTIVE_FILE""#,
            hash = tab_hash,
            path = shlex_quote(path),
        );
        vec!["bash".to_string(), "-c".to_string(), script]
    }
}

/// Minimal shell quoting: wrap in single quotes, escaping embedded single quotes.
fn shlex_quote(s: &str) -> String {
    let escaped = s.replace('\'', "'\\''");
    format!("'{}'", escaped)
}

register_plugin!(FocusTracker);

impl ZellijPlugin for FocusTracker {
    fn load(&mut self, configuration: BTreeMap<String, String>) {
        self.tab_hash = configuration
            .get("tab_hash")
            .cloned()
            .unwrap_or_default();

        self.project_dir = configuration
            .get("project_dir")
            .cloned()
            .unwrap_or_default();

        let ids = get_plugin_ids();
        self.own_pane_id = ids.plugin_id;

        eprintln!(
            "focus-tracker[{}]: load() pane_id={} project_dir='{}' zellij_pid={}",
            self.tab_hash, self.own_pane_id, self.project_dir, ids.zellij_pid
        );

        request_permission(&[
            PermissionType::ReadApplicationState,
            PermissionType::RunCommands,
        ]);

        subscribe(&[
            EventType::PaneUpdate,
            EventType::RunCommandResult,
            EventType::PermissionRequestResult,
        ]);
    }

    fn update(&mut self, event: Event) -> bool {
        match event {
            Event::PaneUpdate(manifest) => {
                // Discover our tab on first PaneUpdate (or if not yet known).
                if self.own_tab_index.is_none() {
                    self.own_tab_index = self.discover_own_tab(&manifest);
                    if self.own_tab_index.is_none() {
                        eprintln!(
                            "focus-tracker[{}]: own_tab_index=None, own_pane_id={}, manifest tabs: {:?}",
                            self.tab_hash,
                            self.own_pane_id,
                            manifest.panes.keys().collect::<Vec<_>>()
                        );
                    } else {
                        eprintln!(
                            "focus-tracker[{}]: discovered own_tab_index={:?}",
                            self.tab_hash, self.own_tab_index
                        );
                    }
                }

                let tab_idx = match self.own_tab_index {
                    Some(idx) => idx,
                    None => return false,
                };

                if self.tab_hash.is_empty() {
                    eprintln!("focus-tracker: tab_hash is empty, skipping");
                    return false;
                }

                // Only look at panes in our own tab.
                let panes = match manifest.panes.get(&tab_idx) {
                    Some(p) => p,
                    None => {
                        eprintln!(
                            "focus-tracker[{}]: tab_idx={} not in manifest",
                            self.tab_hash, tab_idx
                        );
                        return false;
                    }
                };

                // Track the focused pane's title to detect changes.
                let focused_title = panes
                    .iter()
                    .find(|p| p.is_focused && !p.is_plugin)
                    .map(|p| p.title.clone())
                    .unwrap_or_default();

                let title_changed = focused_title != self.last_focused_title;
                if title_changed {
                    eprintln!(
                        "focus-tracker[{}]: title changed '{}' -> '{}'",
                        self.tab_hash, self.last_focused_title, focused_title
                    );
                }
                self.last_focused_title = focused_title.clone();

                // Check if the focused "_ " pane needs async path resolution.
                // Only fire when the title actually changed and no resolve is in flight.
                if title_changed && !self.resolve_in_flight {
                    if let Some(session_name) = self.needs_async_resolve(panes) {
                        eprintln!(
                            "focus-tracker[{}]: firing async resolve for '{}'",
                            self.tab_hash, session_name
                        );
                        self.fire_resolve_path(&session_name);
                        // Don't fall through to project_dir fallback yet —
                        // wait for the resolve result. But if we haven't written
                        // initial CWD yet, use project_dir as interim.
                        if !self.initial_cwd_written && !self.project_dir.is_empty() {
                            self.last_project_path = self.project_dir.clone();
                            self.initial_cwd_written = true;
                            let parts =
                                Self::build_write_command(&self.project_dir, &self.tab_hash);
                            let cmd_refs: Vec<&str> =
                                parts.iter().map(|s| s.as_str()).collect();
                            let mut ctx = BTreeMap::new();
                            ctx.insert("action".to_string(), "write_cwd".to_string());
                            run_command(&cmd_refs, ctx);
                        }
                        return false;
                    }
                }

                // Find the project path based on the focused pane.
                let project_path = match self.find_focused_project(panes) {
                    Some(p) => p,
                    None => {
                        // Log agent pane detection status for debugging.
                        if !self.initial_cwd_written {
                            let agent_count = panes.iter().filter(|p| Self::is_agent_pane(p)).count();
                            let focused = panes.iter().find(|p| p.is_focused && !p.is_plugin);
                            eprintln!(
                                "focus-tracker[{}]: find_focused_project=None, agent_panes={}, focused={:?}",
                                self.tab_hash,
                                agent_count,
                                focused.map(|p| format!(
                                    "id={} title='{}' cmd={:?} is_plugin={}",
                                    p.id, p.title, p.terminal_command, p.is_plugin
                                ))
                            );
                        }
                        return false;
                    }
                };

                let is_initial = !self.initial_cwd_written;

                // Write the CWD file on first update or when the project path changes.
                let path_changed = project_path != self.last_project_path;
                let should_write = is_initial || path_changed;

                if !should_write {
                    return false;
                }

                eprintln!(
                    "focus-tracker[{}]: WRITING cwd '{}' (initial={}, changed={})",
                    self.tab_hash, project_path, is_initial, path_changed
                );

                self.last_project_path = project_path.clone();
                self.initial_cwd_written = true;

                let parts = Self::build_write_command(&project_path, &self.tab_hash);
                let cmd_refs: Vec<&str> = parts.iter().map(|s| s.as_str()).collect();

                let mut ctx = BTreeMap::new();
                ctx.insert("action".to_string(), "write_cwd".to_string());

                run_command(&cmd_refs, ctx);
            }

            Event::RunCommandResult(exit_code, stdout, stderr, context) => {
                if let Some(action) = context.get("action") {
                    match action.as_str() {
                        "write_cwd" => {
                            let code_val = exit_code.unwrap_or(-1);
                            if code_val != 0 {
                                let err = String::from_utf8_lossy(&stderr);
                                eprintln!(
                                    "focus-tracker[{}]: write_cwd failed (exit {}): {}",
                                    self.tab_hash, code_val, err
                                );
                            } else {
                                eprintln!(
                                    "focus-tracker[{}]: write_cwd succeeded",
                                    self.tab_hash
                                );
                            }
                        }
                        "resolve_path" => {
                            self.resolve_in_flight = false;
                            let succeeded = exit_code.map(|c| c == 0).unwrap_or(false);
                            eprintln!(
                                "focus-tracker[{}]: resolve_path result: exit={:?} succeeded={}",
                                self.tab_hash, exit_code, succeeded
                            );
                            if !succeeded {
                                // Resolution failed — fall back to project_dir
                                // if we haven't written anything meaningful yet.
                                return false;
                            }
                            let path = String::from_utf8_lossy(&stdout)
                                .trim()
                                .to_string();
                            if path.is_empty() || !path.starts_with('/') {
                                return false;
                            }
                            // Only write if the resolved path differs from current.
                            if path == self.last_project_path {
                                return false;
                            }
                            self.last_project_path = path.clone();
                            self.initial_cwd_written = true;

                            let parts =
                                Self::build_write_command(&path, &self.tab_hash);
                            let cmd_refs: Vec<&str> =
                                parts.iter().map(|s| s.as_str()).collect();
                            let mut ctx = BTreeMap::new();
                            ctx.insert("action".to_string(), "write_cwd".to_string());
                            run_command(&cmd_refs, ctx);
                        }
                        _ => {}
                    }
                }
            }

            Event::PermissionRequestResult(result) => {
                eprintln!(
                    "focus-tracker[{}]: PermissionRequestResult: {:?}",
                    self.tab_hash, result
                );
            }

            _ => {}
        }
        false
    }

    fn render(&mut self, _rows: usize, _cols: usize) {
        print!("focus-tracker: watching");
    }
}

impl Default for FocusTracker {
    fn default() -> Self {
        Self {
            tab_hash: String::new(),
            project_dir: String::new(),
            last_project_path: String::new(),
            own_tab_index: None,
            own_pane_id: 0,
            initial_cwd_written: false,
            last_focused_title: String::new(),
            resolve_in_flight: false,
        }
    }
}
