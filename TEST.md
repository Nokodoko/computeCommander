# ComputeCommander Test Run

## Setup

```bash
# Navigate to a test project (or create one)
mkdir -p ~/Programs/ai/cc-test-project
cd ~/Programs/ai/cc-test-project
git init

# Add cc to PATH (or use full path)
export PATH="$HOME/Programs/ai/computeCommander:$PATH"
```

## Test 1: Initialize Project

```bash
cc init

# Verify output:
# - .computecommander/ directory created
# - config.yaml present
# - agents/, hooks/, specs/, worktrees/, logs/ dirs exist

cat .computecommander/config.yaml
```

## Test 2: Check Status (Empty Fleet)

```bash
cc status

# Should show: "No active agents"
```

## Test 3: Spawn a Scout Agent

```bash
# Create a simple task spec
cat > .computecommander/specs/scout-test.md << 'EOF'
# Scout Task: Explore Repository

Explore this repository and report:
1. File structure
2. Languages used
3. Entry points
4. Potential areas for improvement

Write findings to SCOUT_REPORT.md
EOF

# Spawn scout agent
cc sling scout-test \
  --capability scout \
  --runtime claude \
  --name scout-alpha \
  --spec .computecommander/specs/scout-test.md
```

## Test 4: Monitor Fleet

```bash
# Check status
cc status

# Should show scout-alpha as "working" or "booting"

# Try the dashboard (if Gum is installed)
cc dashboard
```

## Test 5: Check Mail

```bash
# Check for messages
cc mail check

# List all messages
cc mail list
```

## Test 6: Nudge Agent (if stalled)

```bash
cc nudge scout-alpha --soft
```

## Test 7: Stop Agent

```bash
cc stop scout-alpha

cc status
# Should show no active agents
```

## Test 8: Full Integration Test

```bash
# Create a mini project to orchestrate
cd ~/Programs/ai/cc-test-project

# Create a simple Go file to work with
cat > main.go << 'EOF'
package main

import "fmt"

func main() {
    fmt.Println("Hello from cc-test-project")
}
EOF

cat > go.mod << 'EOF'
module cc-test-project
go 1.22
EOF

# Now spawn a builder to add a feature
cat > .computecommander/specs/add-greeting.md << 'EOF'
# Task: Add Greeting Function

Add a Greet(name string) function to main.go that returns
"Hello, {name}!" and update main() to use it.

Add a test file main_test.go with tests for Greet().

Run `go test` to verify.
EOF

cc sling add-greeting \
  --capability builder \
  --runtime claude \
  --name builder-01 \
  --spec .computecommander/specs/add-greeting.md

# Watch it work
cc status --watch

# When done, check the result
cat main.go
cat main_test.go
go test -v
```

## Test 9: Coordinator Mode (Full Orchestration)

```bash
# Start the coordinator
cc coordinator start

# Coordinator will manage everything from here
# Check its status
cc coordinator status
```

## Test 10: Cleanup

```bash
# Clean up worktrees
cc worktree clean --completed

# Check final state
cc status
cc worktree list
```

---

## Quick Smoke Test (Copy-Paste Ready)

```bash
cd ~/Programs/ai
mkdir -p cc-smoke-test && cd cc-smoke-test
git init
~/Programs/ai/computeCommander/cc init
~/Programs/ai/computeCommander/cc status
cat .computecommander/config.yaml
echo "✅ ComputeCommander is working!"
```

---

## Expected Issues (Known Limitations v0.1.0)

1. **Zellij required** — `cc sling` needs Zellij running
2. **Claude CLI required** — Default runtime is Claude Code
3. **Git repo required** — Must be in a git repository
4. **No Postgres yet** — Uses SQLite by default (which is fine for testing)

## Troubleshooting

```bash
# Check if Zellij is running
zellij list-sessions

# Start Zellij if needed
zellij

# Check cc can find config
cc config show

# Verify runtime is available
which claude
```
