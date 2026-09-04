package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const snapHeader = "# Snap — Local Checkpoint"

const snapRules = `# Snap — Local Checkpoint System

This project uses snap — a local checkpoint tool that saves, compares, and restores code instantly without Git commits. You MUST use it. These rules are non-negotiable.

## PREREQUISITE CHECK (every time, no exceptions):
Before running ANY snap command, check if .snap/ directory exists in the project root.
- If it exists: use snap as described below. No skipping, no excuses.
- If it does NOT exist: skip silently. Do NOT run snap init or ask the user to initialize.
- CRITICAL: Re-check EVERY time before making changes. Never cache from earlier in the session — the user may initialize mid-conversation.
- IMPORTANT: All snap commands must be run from the directory where .snap/ exists. If your working directory is a subdirectory, cd to the project root first or use absolute/relative paths from there. File paths in snap commands are relative to the .snap/ root, not your current directory.

## MANDATORY WORKFLOW — follow this every single time:
1. Before you are about to WRITE or EDIT any project file: save a checkpoint FIRST.
2. After completing a logical unit of work: save a checkpoint.
3. If something breaks or goes wrong: snap restore <id>

## IMPORTANT — When to save and when NOT to save:
Saving a checkpoint ONLY makes sense when you are about to MODIFY files (write, edit, delete, rename).
Do NOT save a checkpoint when you are:
- Just reading or viewing files
- Discussing, planning, or answering questions
- Writing to memory, notes, or non-project files
- Running read-only commands (git status, grep, ls, etc.)
- About to do a git commit/push (those are separate from snap)

"Before any file change" means: you are about to use a tool that WRITES to a project file. If no file will be modified, no checkpoint is needed. Do not create empty/useless checkpoints — keep the timeline clean and meaningful.

## CRITICAL — Choosing the right save type:
This is NOT optional. Use the CORRECT command based on how many files you are about to change:

| Files changing | Command | Example |
|---------------|---------|---------|
| 1 file | snap save-file <file> -m "msg" | snap save-file src/auth.go -m "before refactor" |
| 2 files | snap save-file <file1> <file2> -m "msg" | snap save-file src/a.go src/b.go -m "before fix" |
| 3+ files | snap save "msg" | snap save "before large refactor" |
| Risky/unknown scope | snap save "msg" | snap save "before experimental changes" |

WRONG: Using snap save (full project) when only editing 1 file — wasteful, clutters timeline.
WRONG: Using snap save-file when editing 5+ files — might miss some files.
RIGHT: Match the command to the scope of your change.

## Writing good checkpoint messages and descriptions:
Every checkpoint should have a clear message. Add a description (-d) when it adds value.

**Message** (required): What's happening. Use "before:" or "after:" prefix so the timeline reads naturally.
**Description** (-d flag, optional): Extra context — what's the current state, why does this matter, which approach is this. Think: "What would help me decide whether to restore here?"

Examples:
  snap save "before: add Redis caching" -d "API working without cache, avg 200ms response"
  snap save "after: add Redis caching" -d "cache hit reduces to 15ms, miss unchanged"
  snap save-file src/auth.go -m "before: fix token expiry" -d "tokens expire but refresh fails silently"
  snap save "before: try approach 2" -d "approach 1 worked but too slow, trying event-driven"

Don't write meaningless messages like "checkpoint" or "saving" — they add noise to the timeline.

## After finishing work:
- Finished editing 1 file → snap save-file <file> -m "after: what you did"
- Finished editing multiple files → snap save "after: what you did"
- Only save "after" when you actually changed something. If you read files and decided NOT to change anything, don't save.

## Continuous recording (large refactors, risky/exploratory work):
- Start before beginning: snap record start
- Stop when stable: snap record stop
- Check if active: snap record status
- Use this when: refactoring multiple files, letting an agent work autonomously, trying experimental approaches, or any situation where many changes happen quickly.
- When recording is active, you still save explicit checkpoints at logical milestones — recording captures everything in between.

## Restore — undo anything instantly:
- Full project restore: snap restore <id> (auto-saves current state first — you can never lose work)
- Single file restore: snap restore-file <id> <file> (only that file changes, everything else untouched)
- Mix and match: restore different files from different checkpoints to assemble the best state
- Example: snap restore-file 2 src/auth.go && snap restore-file 4 src/ui.tsx
- When to use: something broke, wrong approach taken, want to go back to a known-good state.
- You can always undo a restore — the current state is auto-saved before restoring.

## Diff & compare — understand what changed:
- See what changed since last save: snap status
- Compare a checkpoint vs current: snap diff <id>
- Compare two checkpoints: snap diff <id1> <id2>
- Full line-by-line diff: snap diff <id1> <id2> -f
- Diff a specific file between two checkpoints: snap diff <id1> <id2> <filepath>
- View file content at any checkpoint: snap show <id> <filepath>
- Use diff BEFORE restoring to verify what will change.
- Use diff to review your own work before telling the user "done".

## Pin — protect important states:
- Pin a known-good state: snap pin <id>
- Pinned checkpoints are NEVER auto-deleted by garbage collection
- Unpin when no longer needed: snap unpin <id>
- When to pin: before major refactors, before risky migrations, when handing off to another agent, any state you might want to return to days later.
- Don't pin everything — only truly important stable states.

## Watch — auto-protect critical files:
- Add: snap watch add <file>
- Remove: snap watch remove <file>
- List: snap watch list
- Any change to a watched file automatically creates a checkpoint
- Use for: config files, database migrations, env files, lock files, anything dangerous to lose or hard to recover.
- Suggest watching when you see the user working with critical config files.

## Rewind — time travel through recorded changes:
- Requires recording to be active (snap record start)
- Jump back: snap rewind "5 minutes ago"
- Specific time: snap rewind "2:30 PM" or snap rewind "14:30:05"
- Specific date: snap rewind "Aug 16 14:30"
- View timeline: snap timeline (shows what changed, when, and whether it was you or an agent)
- Use when: you don't know exactly when something broke, need to explore the timeline.

## Export & Import — share checkpoints:
- Export: snap export <id> (creates portable .snap file)
- Export with custom name: snap export <id> -o backup.snap
- Export encrypted: snap export <id> -p "password" (AES-256-GCM)
- Import: snap import <file.snap>
- Import encrypted: snap import <file.snap> -p "password"
- Use when: sharing state with teammate, backing up before cleanup, moving checkpoints between machines.

## Cleanup:
- Remove old/duplicate snapshots: snap clean
- Preview without deleting: snap clean --dry-run
- Auto-mode (no confirmation): snap clean --auto
- Delete specific checkpoint: snap delete <id>
- Automatic GC runs hourly when recording is active
- Suggest cleanup after long sessions with 20+ checkpoints.

## Search — find files and code across all checkpoints:
- snap search <query> — search file names across all checkpoints (substring match)
- snap search <query> --json — output as JSON
- snap grep <pattern> — search file contents across all checkpoints
- snap grep <pattern> -i — case-insensitive content search
- snap grep <pattern> --regex — regex pattern search
- snap grep <pattern> --json — output as JSON
- Use search to find a file that was deleted or renamed — it searches ALL checkpoints, not just the latest.
- Use grep to find code that was removed — if a function existed in checkpoint #2 but was deleted in #5, grep will find it in #2.
- These are powerful for recovering lost code, finding when something changed, or locating files across the project history.

## All commands reference:
- snap save "msg" — full project checkpoint (use for 3+ files)
- snap save "msg" -d "description" — checkpoint with extra context
- snap save-file <file> -m "msg" — single file checkpoint (use for 1-2 files)
- snap save-file <file1> <file2> -m "msg" — multi-file checkpoint
- snap list — all checkpoints (organized by category)
- snap status — what changed since last save
- snap diff <id> — compare checkpoint vs current
- snap diff <id1> <id2> — compare two checkpoints
- snap diff <id1> <id2> -f — full line-level diff
- snap diff <id1> <id2> <file> — diff specific file between checkpoints
- snap show <id> — list files in checkpoint
- snap show <id> <file> — view file content at checkpoint
- snap restore <id> — restore full project (auto-saves first)
- snap restore-file <id> <file> — restore one file
- snap delete <id> — delete a checkpoint
- snap pin <id> / snap unpin <id> — protect/unprotect
- snap record start/stop/status — continuous recording
- snap timeline — view recorded changes with timestamps
- snap rewind "<time>" — jump back in time
- snap watch add/remove/list — critical file auto-checkpoints
- snap search <query> — find files by name across all checkpoints
- snap grep <pattern> [-i] [--regex] — find content across all checkpoints
- snap export <id> [-o file] [-p password] — export as .snap file
- snap import <file> [-p password] — import a .snap file
- snap clean [--dry-run] [--auto] — garbage collection
- snap update-rules — refresh these instruction files to latest version
- snap setup-hooks — install Claude Code auto-save hook

## Decision guide — what to do when:
| Situation | Action |
|-----------|--------|
| About to edit 1-2 files | snap save-file <files> -m "before: ..." FIRST |
| About to edit 3+ files | snap save "before: ..." FIRST |
| Just reading/discussing/planning | Do NOT save — no checkpoint needed |
| Finished editing | snap save or snap save-file "after: ..." |
| Decided not to change anything | Do NOT save — nothing changed |
| Starting risky/experimental work | snap record start + snap save + snap pin |
| Something broke | snap restore <id> or snap restore-file <id> <file> |
| Need to compare states | snap diff <id1> <id2> |
| Want to see what changed | snap status |
| Looking for a deleted/renamed file | snap search <filename> |
| Looking for removed code | snap grep "function_name" |
| Critical config file exists | snap watch add <file> |
| Sharing state with someone | snap export <id> |
| Long session, lots of checkpoints | snap clean --auto |
| Before a big refactor | snap save + snap pin <id> |
| Agent making autonomous changes | snap record start (agent activity auto-detected) |
| Updating snap CLI version | snap update-rules (refreshes these instructions) |
`

func writeAgentInstructions(root string) {
	// CLAUDE.md
	claudeMdPath := filepath.Join(root, "CLAUDE.md")
	writeOrUpdateRulesFile(claudeMdPath, "CLAUDE.md")

	// .cursorrules
	cursorPath := filepath.Join(root, ".cursorrules")
	writeOrUpdateRulesFile(cursorPath, ".cursorrules")

	// .github/copilot-instructions.md
	copilotDir := filepath.Join(root, ".github")
	copilotPath := filepath.Join(copilotDir, "copilot-instructions.md")
	os.MkdirAll(copilotDir, 0755)
	writeOrUpdateRulesFile(copilotPath, ".github/copilot-instructions.md")
}

func writeOrUpdateRulesFile(path, displayName string) {
	if data, err := os.ReadFile(path); err == nil {
		content := string(data)
		if idx := strings.Index(content, snapHeader); idx != -1 {
			before := content[:idx]
			os.WriteFile(path, []byte(strings.TrimRight(before, "\n")+"\n\n"+snapRules), 0644)
			fmt.Printf("  Updated %s with latest snap rules\n", displayName)
		} else {
			os.WriteFile(path, append(data, []byte("\n\n"+snapRules)...), 0644)
			fmt.Printf("  Appended snap rules to %s\n", displayName)
		}
	} else {
		os.WriteFile(path, []byte(snapRules), 0644)
		fmt.Printf("  Created %s with snap rules\n", displayName)
	}
}
