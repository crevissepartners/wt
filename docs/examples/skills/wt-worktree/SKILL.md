---
name: "wt-worktree"
description: "Shared Claude/Codex skill for wt-managed worktree delegation, parallel branch work, and draft-PR isolation. Register user-maintained copies from dotfiles into each tool loader path, and always prefer wt over raw git worktree commands; cleanup operations stay preview-first."
---

# WT Worktree

Use this skill when the user asks for `$wt-worktree` or when the task benefits from isolated branch work, delegated implementation, or parallel QA.

## App and tool registration

Keep the user-maintained skill source in dotfiles, then copy or symlink it into each tool's loader path.

```text
<dotfiles>/.claude/skills/wt-worktree/SKILL.md
<dotfiles>/.codex/skills/wt-worktree/SKILL.md
```

Tool loader paths:

```text
Claude Code: ~/.claude/skills/wt-worktree/SKILL.md
Codex:      $CODEX_HOME/skills/wt-worktree/SKILL.md
```

Do not install this user skill under `$CODEX_HOME/skills/.system/*`; that namespace is reserved for bundled Codex system skills.

## Core policy

- Prefer `wt` over raw `git worktree` commands.
- Create a named worktree per task or subtask.
- Keep destructive cleanup opt-in. Preview before cleanup when possible.
- When delegation is requested, create the worktree first, then run work inside it or hand that path to a subagent.

## Standard flow

1. Confirm the repo context and current branch state.
2. Check `wt` availability with `wt --version`.
3. Inspect existing worktrees with `wt list` when reuse might matter.
4. Create or resolve a worktree path with `wt path --create <branch>`.
5. Run commands in that worktree with `wt run <branch> -- <command>`.
6. Report the branch name, worktree path, and the exact command the user can rerun.

## Command patterns

- Create or resolve a worktree path: `wt path --create <branch>`
- List worktrees: `wt list`
- Run in a worktree: `wt run <branch> -- <command>`
- Resolve the path only: `wt goto <branch>`
- Cleanup candidates: `wt prune`, `wt cleanup`

## Git in worktree paths

When running git commands against a worktree path, agents should use `git -C <path> ...` instead of `cd <path> && git ...`.

- Why: `cd <path> && git ...` can trigger host harness checks around target-directory `.git/hooks/`.
- Examples:
  - `git -C "$(wt path <branch>)" log --oneline -5`
  - `git -C "$(wt path <branch>)" push -u origin <branch>`
- For non-git commands, prefer `wt run <branch> -- <cmd>`.

## Safety

- Do not delete worktrees, branches, or untracked files unless the user asked.
- If cleanup is relevant, show the target branch or path before executing it.
- If `wt` is unavailable, say that clearly and fall back to the equivalent manual flow only if the user still wants progress without `wt`.

## Delegation guidance

- Use short, traceable branch names tied to the task.
- Keep one concern per worktree when possible.
- For QA handoff, provide a one-line `wt run -- ...` command the user can execute immediately.
