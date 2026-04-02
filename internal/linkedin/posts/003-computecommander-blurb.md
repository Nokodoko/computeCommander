# Post #3: computeCommander -- The Missing Layer Between You and Your AI Agents

## Feed Teaser (Post Blurb)

You gave your AI agents intelligence. You gave them memory. But who is watching them?

When I run six AI agents in parallel -- each on a different branch, each modifying different files, each burning tokens at different rates -- I need more than a terminal window. I need a cockpit.

I built that cockpit. It is called computeCommander, and it turns a swarm of AI agents into something you can actually operate.

Real-time agent status. Inter-agent mail. A merge queue so branches do not collide. A zellij dashboard that gives you the same operational visibility you expect from any distributed system.

AI agents are the new microservices. They deserve orchestration infrastructure.

Full article in the comments.

#AI #AgentOrchestration #SystemDesign #DevOps #DeveloperTools

---

## Article Architecture (Structural Outline)

### 1. Hook: The Invisible Agent Problem
- What happens when you scale past one agent
- The questions you cannot answer without infrastructure

### 2. computeCommander: The Physical Harness
- Fork of overstory (credit jaymin West)
- Three harness metaphor: cockpit + flight computer + flight recorder

### 3. Layer 1: The Dashboard (Zellij Layout)
- Why zellij over bubbletea PTY embedding
- Layout anatomy: 7 panes, each a native process
- Dynamic KDL generation per session

### 4. Layer 2: The Hook Bridge
- Claude Code lifecycle hooks as the nervous system
- Data flow: SubagentStart -> cmdr-bridge.sh -> hook-bridge (Go) -> SQLite -> fsnotify -> dashboard
- Per-session state isolation (the cleanup bomb story)

### 5. Layer 3: The Mail System
- Structured inter-agent messaging
- Automatic lifecycle notifications
- Message types and priority routing

### 6. Layer 4: The Merge Queue
- Branch collision prevention
- FIFO ordering with task/agent attribution
- Dashboard visibility

### 7. The Three Harness Model (Connecting the Series)
- Soft harness (rayne/hooks/intent) = flight computer
- Memory (OpenBrain) = flight recorder
- Physical harness (computeCommander) = cockpit
- Hook bridge as the nervous system

### 8. Why This Matters
- AI agents are the new microservices
- DevOps lessons apply to agent orchestration
- Better tooling > better models for operations

### 9. Built on Overstory (Credit)
- jaymin West's foundation
- What was extended

### 10. What's Next
- Multi-machine fleet management
- Automated conflict resolution
- Agent cost tracking
