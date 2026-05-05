# Setting Up Claude Code as a Mandatory Code Reviewer

This guide shows how to configure Claude Code to use `second-opinion` as a **hard gate** before it writes or commits any code. Every plan and every diff gets reviewed by an external LLM (e.g. GPT-5.5) before Claude proceeds.

## How it works

You add a `CLAUDE.md` file (globally or per-project) that instructs Claude to:
1. Submit its plan to `review_code` before writing a single line
2. Submit its uncommitted diff to `analyze_uncommitted_work` (or `review_code`) before suggesting a commit
3. Choose the `reasoning_effort` level based on how complex the task is

Claude reads `CLAUDE.md` at the start of every session and treats it as binding instructions.

---

## Prerequisites

1. `second-opinion` MCP server installed and registered in `~/.claude.json`:
   ```json
   {
     "mcpServers": {
       "second-opinion": {
         "type": "stdio",
         "command": "/path/to/second-opinion/bin/second-opinion"
       }
     }
   }
   ```

2. `~/.second-opinion.json` configured with your OpenAI key and preferred defaults:
   ```json
   {
     "default_provider": "openai",
     "temperature": 0.3,
     "max_tokens": 4096,
     "openai": {
       "api_key": "sk-...",
       "model": "gpt-5.5",
       "reasoning_effort": "medium"
     }
   }
   ```

---

## Global CLAUDE.md template

Place this at `~/.claude/CLAUDE.md` to enforce the reviewer in **every** Claude Code session:

```markdown
## Second-Opinion Code Review Requirement

Before implementing any changes, you MUST get approval from the external reviewer
(via the `second-opinion` MCP tools). This is a HARD GATE.

### Reasoning effort guide
Choose `reasoning_effort` based on the task:
- `"high"` — security reviews, architecture plans, large diffs (>200 lines)
- `"medium"` — routine code reviews, standard plan reviews
- `"low"` — trivial or purely mechanical changes

### Plan Review (before writing any code)
1. Describe your proposed approach as text and call `review_code` with it:
   - language="markdown", focus="all"
   - provider="openai", model="gpt-5.5"
   - reasoning_effort= (choose per guide above)
2. If the review flags blocking issues — revise your plan and resubmit.
3. STOP until you get a clean review. Architectural suggestions alone do not block.

### Code Review (after writing code)
4. Call `analyze_uncommitted_work` before suggesting a commit:
   - provider="openai", model="gpt-5.5"
   - reasoning_effort= (choose per guide above)
5. If `analyze_uncommitted_work` fails (repo path restriction), fall back to
   `review_code` with the git diff output.
6. Fix any flagged issues and re-run the review.
7. Only suggest committing after the review passes.

### When to skip (avoid unnecessary API calls)
- Typo fixes, comment-only changes, single-line whitespace edits
- File reads, searches, grep, or any exploration with no code changes
```

---

## Per-project CLAUDE.md template

Place this at `<your-repo>/CLAUDE.md` to enforce the reviewer only for a specific project.
Useful when you want stricter rules (e.g. always `"high"` effort) for sensitive codebases:

```markdown
## Code Review Gate

All non-trivial changes must pass an external review before being committed.

### Reasoning effort
Always use `reasoning_effort="high"` for this project — it handles billing/payments.

### Plan Review
Before writing code, call `review_code` with your plan:
- language="markdown", focus="security"
- provider="openai", model="gpt-5.5", reasoning_effort="high"

### Code Review
After writing code, call `review_code` with the git diff:
- language="diff", focus="security"
- provider="openai", model="gpt-5.5", reasoning_effort="high"

Fix all flagged issues before committing.
```

---

## Customising reasoning_effort defaults

The `reasoning_effort` in `~/.second-opinion.json` is the fallback when no per-call override is passed. Adjust it to match your typical workload:

| Workload | Recommended default |
|----------|---------------------|
| General product engineering | `"medium"` |
| Security-sensitive services | `"high"` |
| Internal tooling / scripts | `"low"` |

Per-call overrides always win over the config default, so even with a `"low"` default you can pass `reasoning_effort="high"` for specific security reviews.

---

## Testing the setup

After placing your `CLAUDE.md`, start a new Claude Code session and ask it to make a small code change. You should see it:

1. Call `review_code` with the plan before touching any files
2. Write the code
3. Call `analyze_uncommitted_work` (or `review_code` with the diff) before suggesting a commit

If Claude skips either step, check that your `CLAUDE.md` is in the right location and that the `second-opinion` MCP server is listed in `~/.claude.json`.
