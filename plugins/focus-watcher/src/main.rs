//! focus-watcher: Fast Rust replacement for focus-watcher.sh
//!
//! Polls zellij for the focused pane's terminal ID, scans /proc to find the
//! foreground process on that pts device, resolves its CWD to a git root,
//! and writes the result to the per-tab CWD file.
//!
//! The fp-wrapper and lazygit-wrapper scripts watch this CWD file via
//! inotifywait for instant updates.

use std::env;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::sync::atomic::{AtomicBool, Ordering};
use std::thread;
use std::time::Duration;

static RUNNING: AtomicBool = AtomicBool::new(true);

fn main() {
    // Parse arguments.
    let args: Vec<String> = env::args().collect();
    let tab_hash = parse_arg(&args, "--tab-hash").unwrap_or_else(|| {
        // Positional: focus-watcher <TAB_HASH>
        args.get(1)
            .filter(|a| !a.starts_with('-'))
            .cloned()
            .unwrap_or_else(|| {
                eprintln!("Usage: focus-watcher --tab-hash <HASH> [--poll-ms <MS>]");
                std::process::exit(1);
            })
    });
    let poll_ms: u64 = parse_arg(&args, "--poll-ms")
        .and_then(|s| s.parse().ok())
        .unwrap_or(250);

    let uid = unsafe { libc::getuid() };
    let cwd_file = format!("/tmp/cmdr-{}-{}-cwd", uid, tab_hash);
    let home = env::var("HOME").unwrap_or_default();

    // Signal handling: clean exit on SIGTERM/SIGINT.
    setup_signal_handler();

    eprintln!(
        "focus-watcher: starting (tab_hash={}, poll={}ms, cwd_file={})",
        tab_hash, poll_ms, cwd_file
    );

    let debounce_count: u32 = parse_arg(&args, "--debounce-count")
        .and_then(|s| s.parse().ok())
        .unwrap_or(2);

    let poll_duration = Duration::from_millis(poll_ms);
    let mut last_cwd = String::new();
    let mut last_seen_pts: Option<u32> = None;
    let mut stable_count: u32 = 0;

    eprintln!(
        "focus-watcher: debounce_count={} (focus must be stable for {} polls before updating CWD)",
        debounce_count, debounce_count
    );

    while RUNNING.load(Ordering::Relaxed) {
        if let Some(pts_num) = focused_pane_pts() {
            // Debounce: only proceed after focus has been stable for N consecutive polls.
            // This filters out transient mouse-hover focus changes.
            if last_seen_pts == Some(pts_num) {
                stable_count = stable_count.saturating_add(1);
            } else {
                last_seen_pts = Some(pts_num);
                stable_count = 1;
            }

            if stable_count >= debounce_count {
                if let Some(cwd) = fg_cwd_for_pts(pts_num) {
                    // Resolve to git root.
                    let project = find_git_root(&cwd).unwrap_or(cwd);

                    // Skip dotfile/config directories ($HOME/.<something>).
                    if !is_dotdir(&project, &home) && project != last_cwd {
                        if write_cwd_atomic(&cwd_file, &project).is_ok() {
                            last_cwd = project;
                        }
                    }
                }
            }
        }
        thread::sleep(poll_duration);
    }

    eprintln!("focus-watcher: exiting cleanly");
}

/// Check if a path is under $HOME/.<something> (dotfile/config directory).
fn is_dotdir(path: &str, home: &str) -> bool {
    if home.is_empty() {
        return false;
    }
    if let Some(rest) = path.strip_prefix(home) {
        if let Some(after_slash) = rest.strip_prefix('/') {
            return after_slash.starts_with('.');
        }
    }
    false
}

/// Parse a named argument from the argument list.
fn parse_arg(args: &[String], flag: &str) -> Option<String> {
    args.iter()
        .position(|a| a == flag)
        .and_then(|i| args.get(i + 1).cloned())
}

/// Set up SIGTERM and SIGINT handlers to set RUNNING to false.
fn setup_signal_handler() {
    // Use a simple pipe-based approach for signal handling.
    unsafe {
        libc::signal(libc::SIGTERM, signal_handler as libc::sighandler_t);
        libc::signal(libc::SIGINT, signal_handler as libc::sighandler_t);
    }
}

extern "C" fn signal_handler(_sig: i32) {
    RUNNING.store(false, Ordering::Relaxed);
}

/// Get the pts number of the focused pane from `zellij action list-clients`.
/// Output format (after header):
///   CLIENT_ID  ZELLIJ_PANE_ID  RUNNING_COMMAND
/// Where ZELLIJ_PANE_ID is "terminal_N" and N is the pts minor number.
fn focused_pane_pts() -> Option<u32> {
    let output = Command::new("zellij")
        .args(["action", "list-clients"])
        .output()
        .ok()?;

    if !output.status.success() {
        return None;
    }

    let stdout = String::from_utf8_lossy(&output.stdout);
    for line in stdout.lines().skip(1) {
        let mut fields = line.split_whitespace();
        let client_id = fields.next()?;
        if client_id != "1" {
            continue;
        }
        let pane_id = fields.next()?;
        let pts_str = pane_id.strip_prefix("terminal_")?;
        return pts_str.parse().ok();
    }
    None
}

/// Scan /proc to find the foreground process on /dev/pts/<pts_num> and return its CWD.
///
/// A process is in the foreground group when its pgrp equals the tpgid of its tty.
/// The tty_nr for pts devices is makedev(136, pts_num) = 136*256 + pts_num.
fn fg_cwd_for_pts(pts_num: u32) -> Option<String> {
    let target_tty: i64 = 136 * 256 + pts_num as i64;
    let mut best_cwd = String::new();

    let proc_dir = match fs::read_dir("/proc") {
        Ok(d) => d,
        Err(_) => return None,
    };

    for entry in proc_dir.flatten() {
        let name = entry.file_name();
        let name_str = name.to_str().unwrap_or("");

        // Only process numeric directories (PIDs).
        if !name_str.bytes().all(|b| b.is_ascii_digit()) || name_str.is_empty() {
            continue;
        }

        let stat_path = entry.path().join("stat");
        let stat_content = match fs::read_to_string(&stat_path) {
            Ok(c) => c,
            Err(_) => continue,
        };

        // Parse /proc/PID/stat — the comm field is enclosed in parens and may contain
        // spaces and parens itself. Find the LAST ')' to skip past comm.
        let close_paren = match stat_content.rfind(')') {
            Some(pos) => pos,
            None => continue,
        };

        let after_comm = &stat_content[close_paren + 1..];
        let fields: Vec<&str> = after_comm.split_whitespace().collect();
        // Fields after ')': state(0) ppid(1) pgrp(2) session(3) tty_nr(4) tpgid(5) ...
        if fields.len() < 6 {
            continue;
        }

        let tty_nr: i64 = match fields[4].parse() {
            Ok(v) => v,
            Err(_) => continue,
        };
        if tty_nr != target_tty {
            continue;
        }

        let pgrp: i64 = match fields[2].parse() {
            Ok(v) => v,
            Err(_) => continue,
        };
        let tpgid: i64 = match fields[5].parse() {
            Ok(v) => v,
            Err(_) => continue,
        };

        // Only foreground process group.
        if pgrp != tpgid {
            continue;
        }

        // Read the CWD symlink.
        let cwd_link = entry.path().join("cwd");
        let cwd = match fs::read_link(&cwd_link) {
            Ok(p) => p.to_string_lossy().to_string(),
            Err(_) => continue,
        };

        // Verify directory exists.
        if !Path::new(&cwd).is_dir() {
            continue;
        }

        // Prefer the longest (most specific) CWD.
        if cwd.len() > best_cwd.len() {
            best_cwd = cwd;
        }
    }

    if best_cwd.is_empty() {
        None
    } else {
        Some(best_cwd)
    }
}

/// Walk up from `dir` looking for a `.git` directory or file (for worktrees).
/// Returns the git root directory, or None if not in a git repo.
fn find_git_root(dir: &str) -> Option<String> {
    let mut path = PathBuf::from(dir);
    loop {
        let git_path = path.join(".git");
        if git_path.exists() {
            return Some(path.to_string_lossy().to_string());
        }
        if !path.pop() {
            return None;
        }
    }
}

/// Atomically write `content` to `path` via a temp file + rename.
fn write_cwd_atomic(path: &str, content: &str) -> io::Result<()> {
    let tmp = format!("{}.tmp.{}", path, std::process::id());
    fs::write(&tmp, content)?;
    fs::rename(&tmp, path)?;

    // Also write to the active-cwd file (global fallback).
    // Extract the prefix: /tmp/cmdr-<uid>-
    if let Some(hash_start) = path.rfind('-') {
        // path = /tmp/cmdr-1000-abcd1234-cwd, we want /tmp/cmdr-1000-active-cwd
        // Find second-to-last hyphen before the hash.
        let prefix = &path[..hash_start]; // /tmp/cmdr-1000-abcd1234
        if let Some(uid_end) = prefix.rfind('-') {
            let uid_prefix = &path[..uid_end]; // /tmp/cmdr-1000
            let active_file = format!("{}-active-cwd", uid_prefix);
            let active_tmp = format!("{}.tmp.{}", active_file, std::process::id());
            let _ = fs::write(&active_tmp, content);
            let _ = fs::rename(&active_tmp, &active_file);
        }
    }

    Ok(())
}

// We use libc for getuid() and signal handling — declare the dependency.
#[allow(non_camel_case_types)]
mod libc {
    extern "C" {
        pub fn getuid() -> u32;
        pub fn signal(signum: i32, handler: usize) -> usize;
    }

    pub type sighandler_t = usize;

    pub const SIGTERM: i32 = 15;
    pub const SIGINT: i32 = 2;
}
