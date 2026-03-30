#!/usr/bin/env python3
"""LinkedIn post quality evaluator for autoresearch optimization loop.

Scores the output of `cmdr linkedin generate --dry-run` using Claude Haiku
as the judge. Outputs a single line: `post_quality: <float>` on a 0-10 scale.

Usage (as autoresearch experiment script):
  autoresearch run \
    --script ./eval_linkedin.py \
    --target-file internal/linkedin/content.go \
    --metric post_quality \
    --direction maximize
"""

import json
import re
import subprocess
import sys

SCORING_PROMPT = """\
You are an expert at evaluating LinkedIn posts for a technical audience (senior engineers, tech leads, CTOs).
Score the following LinkedIn post on each dimension from 0 to 10. Be strict — a 10 is exceptional, a 5 is mediocre.

## Post to Evaluate
{post}

## Scoring Dimensions

Score each dimension 0-10:

**hook_strength** (weight: 0.20)
Does the opening line immediately grab attention?
10 = Provocative question or surprising claim that makes you stop scrolling
5  = Mildly interesting but generic opener
0  = "Today I want to talk about..." or similar weak opening

**structure** (weight: 0.15)
Visual clarity, whitespace, sections, readability in a feed context.
10 = Clear hierarchy: hook, problem, body with line breaks, CTA — easy to skim
5  = Some structure but walls of text or no visual breaks
0  = Single unbroken paragraph

**specificity** (weight: 0.20)
Real technical details, concrete numbers, named tools/patterns — not platitudes.
10 = Names specific tools, real metrics, exact architecture patterns
5  = Mix of concrete and vague
0  = "AI is transforming everything" level of generality

**storytelling** (weight: 0.15)
Problem → solution → insight arc. Does it tell a story?
10 = Clear narrative: here's the problem, here's what we built, here's what I learned
5  = Has elements of a story but jumps around
0  = Bullet list of facts with no narrative thread

**call_to_action** (weight: 0.10)
Does it end with an engagement driver?
10 = Specific question that professionals can answer from their own experience
5  = Generic "what do you think?" or weak invitation
0  = No CTA, post just ends

**length_score** (weight: 0.10)
Optimal LinkedIn range is 150-300 words for feed engagement.
10 = 150-300 words
7  = 100-150 words or 300-400 words (slightly outside optimal)
4  = 50-100 words or 400-600 words
0  = Under 50 words or over 600 words

**technical_depth** (weight: 0.10)
Does it demonstrate genuine engineering knowledge vs surface-level buzzwords?
10 = Shows real architectural thinking, trade-offs, implementation specifics
5  = Some technical content mixed with buzzwords
0  = Pure buzzword soup ("leveraging AI to synergize...")

## Instructions
Respond with ONLY a JSON object, no markdown fences, no explanation:
{{"hook_strength": <0-10>, "structure": <0-10>, "specificity": <0-10>, "storytelling": <0-10>, "call_to_action": <0-10>, "length_score": <0-10>, "technical_depth": <0-10>}}
"""

WEIGHTS = {
    "hook_strength": 0.20,
    "structure": 0.15,
    "specificity": 0.20,
    "storytelling": 0.15,
    "call_to_action": 0.10,
    "length_score": 0.10,
    "technical_depth": 0.10,
}


def build_project() -> bool:
    """Returns True if the build succeeds."""
    result = subprocess.run(
        ["go", "build", "./..."],
        cwd="/home/n0ko/Programs/ai/computeCommander",
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        print(result.stdout, file=sys.stderr)
        print(result.stderr, file=sys.stderr)
        return False
    return True


def generate_post() -> str | None:
    """Run cmdr linkedin generate --dry-run and return the post text."""
    result = subprocess.run(
        ["/home/n0ko/.local/bin/cmdr", "linkedin", "generate", "--dry-run"],
        capture_output=True,
        text=True,
        timeout=300,
    )
    if result.returncode != 0:
        print(f"generate failed: {result.stderr}", file=sys.stderr)
        return None
    return result.stdout.strip()


def extract_post_body(raw: str) -> str:
    """Strip the TITLE/DIAGRAM/TARGET header lines, return just the post content."""
    # The generator outputs: TITLE: ...\nDIAGRAM: ...\nTARGET: ...\n---\n<post>
    parts = raw.split("---", 1)
    if len(parts) == 2:
        return parts[1].strip()
    return raw.strip()


def score_post(post: str) -> float:
    """Use Claude Haiku via the claude CLI to score the post. Returns weighted average 0-10."""
    prompt = SCORING_PROMPT.format(post=post)

    result = subprocess.run(
        ["claude", "-p", prompt, "--model", "claude-haiku-4-5-20251001"],
        capture_output=True,
        text=True,
        timeout=120,
    )
    if result.returncode != 0:
        print(f"claude CLI failed: {result.stderr}", file=sys.stderr)
        return 0.0

    raw = result.stdout.strip()

    try:
        scores = json.loads(raw)
    except json.JSONDecodeError:
        # Attempt to extract JSON from the response.
        match = re.search(r"\{[^}]+\}", raw, re.DOTALL)
        if not match:
            print(f"Failed to parse scores from: {raw}", file=sys.stderr)
            return 0.0
        scores = json.loads(match.group())

    weighted = sum(scores.get(k, 0) * w for k, w in WEIGHTS.items())

    # Log individual scores for debugging.
    for dim, w in WEIGHTS.items():
        print(f"  {dim}: {scores.get(dim, 0):.1f} (weight {w})", file=sys.stderr)

    return round(weighted, 4)


def main() -> None:
    if not build_project():
        print("post_quality: 0.0")
        return

    raw = generate_post()
    if not raw:
        print("post_quality: 0.0")
        return

    post = extract_post_body(raw)
    if not post:
        print("post_quality: 0.0")
        return

    score = score_post(post)
    print(f"post_quality: {score}")


if __name__ == "__main__":
    main()
