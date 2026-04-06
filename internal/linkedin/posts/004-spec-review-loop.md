# How I Made AI Write Its Own Requirements — And Then Argue With Itself Until They're Right

## Post Blurb (Feed Teaser)

Most AI-generated code fails before the first line is written.

Not because the model is stupid. Because nobody told it what "done" means. The prompt was vague. The scope was undefined. The success criteria were vibes. So the AI did what any intelligent system does with ambiguous input: it guessed. And it guessed wrong.

I built a system where AI agents write their own formal specifications — 19-section declarative documents with data models, agent assignments, dependency graphs, and machine-verifiable success criteria — and then a second AI agent tears those specs apart across seven review dimensions until every ambiguity is eliminated. Here is how it works.

---

## The Specification Problem Nobody Talks About

The AI coding discourse is obsessed with the wrong bottleneck.

We debate model intelligence. We compare benchmarks. We argue about whether Claude or GPT writes better React components. Meanwhile, the actual failure mode in production AI workflows has nothing to do with model capability.

It is specification failure.

I tracked my own AI-assisted development over six weeks. The pattern was consistent: when a task had a clear, detailed specification — concrete inputs, expected outputs, defined scope boundaries, explicit success criteria — the AI produced usable output on the first pass roughly 85% of the time. When the task was a vague prompt like "add authentication to the API" or "refactor the database layer," the first-pass success rate dropped to around 30%.

The model did not get dumber between those two scenarios. The input got worse.

This is the specification gap. The distance between what you mean and what you say. Humans bridge this gap unconsciously — we fill in assumptions, infer context, apply domain knowledge we did not articulate. AI models do the same thing, except their assumptions are drawn from training data averages rather than your specific codebase, your architectural decisions, and your team's conventions.

The solution is not better models. It is better specifications.

---

## The Spec-Builder Agent: Interrogate, Then Declare

The spec-builder is a specialist agent that does one thing: transform raw human prompts into comprehensive, unambiguous specifications. It never writes code. It never executes tasks. It produces documents that other agents consume and execute deterministically.

The process starts with interrogation.

When you give the spec-builder a task — say, "build a webhook ingestion pipeline for Datadog alerts" — it does not immediately start writing. It first reads a gold-standard template (a 19-section structural blueprint covering everything from data models to failure modes) and then analyzes your prompt against a gap checklist:

- Is there a named project or system? Yes — webhook pipeline.
- Is there a concrete action? Yes — build.
- Are there scope boundaries? Partially — Datadog alerts, but which alert types? All of them? Only specific monitors?
- Is the storage format specified? No.
- Are the entities and their fields defined? No.
- Is the interface surface described? No.
- What consumes this pipeline? Unknown.

For every gap, the spec-builder halts and asks. Not vague "tell me more" questions — structured interrogation with suggested defaults. "Storage format is unspecified. Suggested default: JSONL for git-native storage. PostgreSQL if you need relational queries. Which do you prefer, or should I use JSONL?"

This interrogation phase is the difference between a specification and a wish list. The spec-builder will not proceed with ambiguity. Every decision gets made before the document is written, not discovered during implementation.

### The 19-Section Format

After gaps are resolved, the spec-builder produces a document following a rigid 19-section structure. Fourteen design sections, five execution sections:

**Design Sections (1-14):**

1. **Title + Summary** — One-line description with runtime and key properties
2. **Why** — Pain points with specifics, not philosophy
3. **Design Principles** — Hard rules, numbered, with concrete elaboration
4. **On-Disk Format** — ASCII directory trees with annotated file purposes
5. **Data Model** — TypeScript interfaces with typed fields, lifecycle diagrams, enum tables
6. **CLI** — Every command with flags, aligned columns, required/optional annotations
7. **JSON Output Format** — Separate blocks for success, error, and list responses
8. **Concurrency Model** — Lock strategy, atomic write patterns, conflict resolution
9. **Migration** — Current-vs-target comparison tables, one-time migration steps
10. **Integration** — How consumers interact, agent-facing commands, hooks integration
11. **What It Does NOT Do** — Explicit anti-scope to prevent creep
12. **Tech Stack** — Every choice justified in one line
13. **Project Infrastructure** — Directory structure, CI, version management, scripts
14. **Estimated Size** — File counts and LOC ranges per area

**Execution Sections (15-19):**

15. **Task Manifest** — Table with ID, agent assignment, file scope (read/write), dependencies, and verify command for every task
16. **Dependency Graph** — Phase groupings showing parallelism and sequencing
17. **Target State** — Every file created, modified, or deleted
18. **Verification Plan** — Per-task checks, integration check, and rollback procedure
19. **Success Criteria** — Machine-verifiable checkbox items, every one a shell-checkable predicate

The cardinal rule: never remove, only reformat and add. Every piece of information from the original prompt appears in the final spec — restructured into the appropriate section, expanded with implementation details, enriched with ASCII diagrams and TypeScript interfaces, but never omitted.

The result is a document where a supervisor agent can orchestrate an entire swarm without additional context from the human. The spec is self-contained. Every decision is made. Every file is scoped. Every task has a named agent and a verification command.

---

## The Spec-Reviewer Agent: Seven Dimensions of Scrutiny

A specification is only as good as its review.

The spec-reviewer is a second specialist agent that reads the spec-builder's output and evaluates it across seven dimensions. It never modifies the spec. It reads and reports only. Its sole output is a structured review document with findings classified by severity.

### Dimension 1: Completeness

Are all 19 sections present and substantive? Not just headers — actual content. A Data Model section with a single untyped field is flagged. A Success Criteria section with subjective language ("code is well-structured") is flagged. A Task Manifest with missing verify commands is flagged.

### Dimension 2: Clarity

Detect ambiguous language that leads to divergent implementations. The reviewer scans for hedge words in Design Principles ("should try to," "might," "ideally"), unmeasurable success criteria ("clean," "appropriate," "reasonable"), vague task descriptions ("handle various," "improve," "optimize"), and ambiguous scope boundaries ("and more," "etc.," "as needed").

Each hedge word is a potential branch point where two workers could implement the same spec differently. The reviewer eliminates those branch points.

### Dimension 3: Correctness

Verify facts and logical validity. Are the agent names in the Task Manifest from the known roster? Are file paths syntactically valid? Is the dependency graph acyclic? Are TypeScript interfaces syntactically correct? Are verify commands valid shell commands?

### Dimension 4: Consistency

Cross-reference sections for contradictions. Does every file in the Task Manifest's write-scope appear in Target State? Does every file in Target State appear in at least one Task Manifest row? Do agent assignments match between the manifest and the assignments table? Does the tech stack align with the verification commands?

This dimension catches the class of bugs where Section 5 says one thing and Section 17 says another — contradictions that a human reviewer might miss across a 3,000-word document.

### Dimension 5: SDLC Alignment

Evaluate the spec's outcomes against predicate types from the intent engineering system. Every success criterion should map to a testable predicate: `contains_pattern`, `structural_check`, `count_check`, `ast_check`, `semantic_check`, or `negation_check`. Criteria that cannot be mapped to a predicate get flagged — they will fail the intent gate later, so better to catch them now.

### Dimension 6: Actionability

Can a supervisor execute this spec without asking questions? Every task has a verify command. No two tasks write to the same file without dependency ordering. No circular execution paths. A recommended directive is specified. All files in read-scope either exist on disk or are created by a predecessor task. No unresolved open questions block execution.

### Dimension 7: Rebuild Fidelity (conditional)

For rebuild specifications — where the goal is to reproduce an existing codebase byte-for-byte — the reviewer checks that every target file has its complete content in a code block, SHA-256 checksums exist for every file, whitespace-critical files have base64-encoded versions, and all intentional artifacts (typos, deprecated formats, stale documentation) are documented.

### The Finding Format

Every finding is structured:

```
{DIMENSION_PREFIX}-{NUMBER} [{SEVERITY}] — {title}
Section: {section name}
Issue: {detailed explanation quoting the problematic text}
Suggestion: {concrete fix recommendation}
```

Severity levels: **CRITICAL** blocks swarm execution. **WARNING** may cause worker divergence. **INFO** is an improvement suggestion.

The verdict logic: zero critical findings and zero warnings is PASS. Zero critical findings with warnings is PASS WITH WARNINGS. Any critical finding is FAIL.

---

## The Review Loop: Build, Review, Fix, Repeat

The spec-builder and spec-reviewer do not operate in isolation. They operate in a loop.

The `/sr` command (Spec-Review-Execute) orchestrates the full cycle:

```
Phase 0: Codebase Context Discovery
  Read agentic_instructions.md files for domain context

Phase 1: Generate Spec
  spec-builder reads template + context + prompt → writes SPEC.md

Phase 2: Validate Structure
  validator checks all 19 sections present, no cycles, no conflicts

Phase 3: Review (iterative, max 3 iterations)
  LOOP:
    spec-reviewer reviews SPEC.md → writes SPEC-REVIEW.md
    If PASS: exit loop
    If FAIL:
      spec-reviewer writes SPEC-REVIEW-feedback.md
      spec-builder reads feedback → revises SPEC.md
      Increment iteration counter
      Re-review
  END LOOP

Phase 4: Execute (if --execute flag)
  /swarm consumes SPEC.md and dispatches agents
```

The critical mechanism is the feedback file. When the spec-reviewer finds critical issues, it produces `SPEC-REVIEW-feedback.md` — a structured document that separates critical fixes (must address) from warnings (should address), each with concrete fix instructions. The spec-builder reads this feedback file alongside the existing SPEC.md and produces a revised version.

Then the spec-reviewer reviews again. A fresh pass, same seven dimensions, same severity thresholds.

The loop caps at three iterations. In practice, most specs converge in one or two iterations. The first review catches structural gaps and ambiguous language. The rebuild addresses those. The second review typically passes or passes with minor warnings.

What makes this work is the separation of concerns. The spec-builder is optimized for generation — it knows the template, understands gap analysis, and produces comprehensive documents. The spec-reviewer is optimized for evaluation — it knows the seven dimensions, understands cross-referencing, and produces structured findings. Neither agent tries to do both. They argue with each other through structured documents until the output is clean.

---

## Context Injection: The Intelligence Multiplier

A spec-builder operating in a vacuum produces generic specifications. A spec-builder with domain context produces specifications that match your actual codebase.

This is where context injection enters the pipeline.

Before the spec-builder receives a prompt, the context injection system (`context-inject.py`) classifies the task and injects domain-specific context. The classification determines which `agentic_instructions.md` files are relevant — these are per-directory architecture documents that describe the tech stack, naming conventions, key files, patterns, and scope boundaries for each part of the codebase.

When the spec-builder generates a spec for "add a Jira integration pane to the dashboard," it does not guess about the dashboard's architecture. It reads the agentic instructions for `internal/tui/`, `internal/zellij/`, and `internal/commands/` and knows:

- The dashboard is a zellij KDL layout, not a bubbletea TUI
- Panes run native processes, not embedded PTYs
- The layout is generated by `GenerateLayout()` in Go, not static KDL files
- The existing pane pattern uses `cmdr <command> --pane` with a `--json` flag for structured output
- New panes need entries in the SQLite schema, the layout generator, and the hook bridge

This context flows directly into the spec's Design Principles, On-Disk Format, Integration, and Task Manifest sections. The agent assignments are informed by what actually exists — the spec-builder assigns `unix-coder` for Go implementation, `code-review` for architecture review, not because those are defaults, but because the context injection told it this is a Go project with bubbletea components and zellij layout generation.

Without context injection, the spec-builder would produce a valid but generic specification. With context injection, it produces a specification grounded in the reality of the codebase.

---

## Intent Engineering: The Objective Gate

Context injection tells the spec-builder what exists. Intent engineering tells it what must be true.

The intent system (`intent-engineer.py`) maintains two sets of objectives: personal standards and organizational standards. Personal standards include: tests pass for all new code, no hardcoded secrets, code follows Go conventions, functions have clear error handling, no unused imports. Organizational standards include: no security vulnerabilities, API endpoints validate input, database queries are parameterized, changes are backward compatible.

Every specification's success criteria are scored against these objectives. The intent classifier reads each criterion and maps it to a predicate type:

- "Tests pass" → `test_execution` predicate (confidence: 0.90)
- "No hardcoded secrets" → `negation_check` predicate (confidence: 0.85)
- "API validates input" → `structural_check` predicate (confidence: 0.80)
- "Code is well-structured" → `semantic_check` predicate (confidence: 0.30 — AMBIGUOUS)

That last one — "code is well-structured" — would be flagged. Confidence below 0.50 triggers interrogation. The system asks: "What does 'well-structured' mean? Define it as a shell-checkable predicate." This forces the spec to replace subjective language with measurable outcomes before any agent writes a line of code.

The intent gate operates at two points: before the spec-builder starts (ensuring the prompt has testable objectives) and after the spec is complete (ensuring the success criteria align with organizational standards). A spec that passes the review loop but fails the intent gate does not proceed to execution.

This creates a verification chain:

```
Human Prompt
  → Intent Gate (objectives alignment check)
    → Context Injection (domain knowledge enrichment)
      → Spec-Builder (19-section specification)
        → Validator (structural integrity)
          → Spec-Reviewer (7-dimension quality review)
            → Review Loop (iterative refinement)
              → Intent Gate (final objectives check)
                → Swarm Execution (agents execute the spec)
```

Every stage has a defined failure mode and a defined recovery path. No stage is skipped. No stage operates on vibes.

---

## The Architecture: How It All Connects

```
+------------------+     +-------------------+     +------------------+
|   Human Prompt   |---->| context-inject.py |---->|  Enriched Prompt |
+------------------+     | Classifies task,  |     | + domain context |
                          | injects aid files |     | + agentic scope  |
                          +-------------------+     +--------+---------+
                                                             |
                          +-------------------+              v
                          | intent-engineer   |     +------------------+
                          | Gates against     |<--->|  spec-builder    |
                          | objectives:       |     | Reads template   |
                          | - personal        |     | Interrogates     |
                          | - organizational  |     | Writes SPEC.md   |
                          +-------------------+     +--------+---------+
                                                             |
                                                             v
                                                    +------------------+
                                                    |   validator      |
                                                    | 10 structural    |
                                                    | integrity checks |
                                                    +--------+---------+
                                                             |
                                    +------------------------+
                                    |
                                    v
+------------------+       +------------------+
| SPEC-REVIEW-     |<------| spec-reviewer    |
| feedback.md      |       | 7 dimensions     |
| Critical fixes   |       | COMP CLAR CORR   |
| Warning fixes    |       | CONS SDLC ACTN   |
+--------+---------+       | RBLD (rebuild)   |
         |                  +--------+---------+
         |                           |
         |     FAIL?                 v
         +---------->  SPEC-REVIEW.md (verdict)
                            |
                    PASS?   |
                            v
                   +------------------+
                   | /swarm --spec    |
                   | Dispatches agents|
                   | per Task Manifest|
                   +------------------+
```

The loop between spec-reviewer feedback and spec-builder revision is where the real value compounds. Each iteration reduces ambiguity. Each iteration tightens the specification. By the time the spec reaches the swarm, it is a deterministic execution plan — not a suggestion.

---

## Why This Matters: Determinism from Probabilistic Systems

The AI industry has a dirty secret: most AI-generated code is not deterministic. Give the same prompt to the same model twice and you get different output. Change the temperature, the system prompt, the context window contents — the output shifts.

This is fine for creative writing. It is catastrophic for production software.

The spec-review loop addresses this by moving the non-determinism upstream. The spec-builder might produce slightly different specifications on different runs — that is acceptable, because the spec-reviewer will catch any ambiguity, any gap, any inconsistency. The review loop converges on a deterministic specification regardless of the non-deterministic generation process.

Once the specification is locked, the execution is deterministic. Task T1 is assigned to `unix-coder`. It reads files A and B. It writes file C. Its verify command is `go test ./internal/auth/...`. Either the test passes or it fails. There is no room for interpretation.

This is the difference between:

- "AI wrote some code" (unverified, unspecified, unreproducible)
- "AI produced a verified specification, reviewed it across seven dimensions, iterated until clean, and executed it under supervision with machine-verifiable success criteria" (deterministic, auditable, reproducible)

The specification is the contract. The review loop is the enforcement. The intent gate is the alignment check. Together, they turn probabilistic language models into deterministic engineering tools.

---

## What's Next

The spec-review loop is production infrastructure that I use daily, but the roadmap has three expansions:

**Spec Diffing** — When the review loop iterates, the spec-builder currently rewrites the entire SPEC.md. The next evolution produces a structured diff showing exactly what changed between iterations, making it easier to verify that feedback was addressed without re-reading the entire document.

**Cross-Spec Dependency Detection** — When multiple specs exist for the same codebase, the system will detect file-scope overlaps and dependency conflicts between specs before any execution begins. Two specs that both write to `internal/auth/middleware.go` should know about each other.

**Spec-to-Test Pipeline** — The Success Criteria section already contains machine-verifiable predicates. The next step is generating actual test files from those predicates — turning the spec directly into a test harness that the implementation must satisfy. The spec becomes the test, not just the plan.

---

*The spec-review loop is part of a broader AI agent harness that includes persistent memory (OpenBrain), operational control (computeCommander), and behavioral guardrails (hooks + intent). Articles #1-3 in this series cover the other layers.*

*How are you handling specification quality in your AI workflows? Are your agents executing against formal specs or raw prompts? I am genuinely curious about the gap between "AI wrote code" and "AI executed a verified plan." Drop a comment or reach out.*

---

#AI #SystemDesign #SpecificationEngineering #ContextEngineering #AgentArchitecture #DevOps #QualityAssurance #SoftwareEngineering #DeveloperTools #CodeQuality
