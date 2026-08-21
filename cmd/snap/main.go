package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nihalkumar/snap/internal/delta"
	"github.com/nihalkumar/snap/internal/diff"
	"github.com/nihalkumar/snap/internal/recorder"
	"github.com/nihalkumar/snap/internal/snapshot"
	"github.com/nihalkumar/snap/internal/store"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "init":
		cmdInit()
	case "save":
		cmdSave()
	case "list", "ls":
		cmdList()
	case "restore":
		cmdRestore()
	case "diff":
		cmdDiff()
	case "show":
		cmdShow()
	case "delete", "rm":
		cmdDelete()
	case "status":
		cmdStatus()
	case "pin":
		cmdPin(true)
	case "unpin":
		cmdPin(false)
	case "save-file":
		cmdSaveFile()
	case "restore-file":
		cmdRestoreFile()
	case "watch":
		cmdWatch()
	case "clean":
		cmdClean()
	case "export":
		cmdExport()
	case "import":
		cmdImport()
	case "record":
		cmdRecord()
	case "rewind":
		cmdRewind()
	case "timeline":
		cmdTimeline()
	case "update-rules":
		cmdUpdateRules()
	case "setup-hooks":
		setupClaudeHook()
	case "version":
		fmt.Printf("snap v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func cmdInit() {
	root, err := os.Getwd()
	if err != nil {
		fatal("get working directory: %v", err)
	}

	// Check for --setup-hooks flag
	wantHooks := false
	for _, arg := range os.Args[2:] {
		if arg == "--setup-hooks" {
			wantHooks = true
		}
	}

	engine := snapshot.NewEngine(root)
	if engine.IsInitialized() {
		fixed, err := engine.Repair()
		if err != nil {
			fatal("repair: %v", err)
		}
		if fixed > 0 {
			fmt.Printf("Repaired .snap/ structure (%d broken references removed)\n", fixed)
		} else {
			fmt.Println("Already initialized. Structure verified ✓")
		}
		writeAgentInstructions(root)
		fmt.Println("  Agent instruction files updated to latest rules.")
		if wantHooks {
			setupClaudeHook()
		}
		return
	}

	if err := engine.Init(); err != nil {
		fatal("initialize: %v", err)
	}

	fmt.Printf("Initialized snap in %s/.snap/\n", root)

	// Auto-save initial state
	snap, err := engine.Save("initial state", false)
	if err == nil {
		fmt.Printf("  Saved initial checkpoint #%d (%d files)\n", snap.ID, snap.FileCount)
	}

	// Create agent instruction files
	writeAgentInstructions(root)

	if wantHooks {
		setupClaudeHook()
	}

	fmt.Println("\n  Agents will now auto-save before/after changes.")
	fmt.Println("  Manual commands:")
	fmt.Println("    snap save \"message\"     — explicit checkpoint")
	fmt.Println("    snap list               — view all checkpoints")
	fmt.Println("    snap restore <id>       — rollback")
	if !wantHooks {
		fmt.Println("\n  To install Claude Code auto-save hook:")
		fmt.Println("    snap setup-hooks")
	}
}

func writeAgentInstructions(root string) {
	snapRules := `# Snap — Local Checkpoint System

This project uses snap — a local checkpoint tool that saves, compares, and restores code instantly without Git commits. You MUST use it. These rules are non-negotiable.

## PREREQUISITE CHECK (every time, no exceptions):
Before running ANY snap command, check if .snap/ directory exists in the project root.
- If it exists: use snap as described below. No skipping, no excuses.
- If it does NOT exist: skip silently. Do NOT run snap init or ask the user to initialize.
- CRITICAL: Re-check EVERY time before making changes. Never cache from earlier in the session — the user may initialize mid-conversation.

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
| Critical config file exists | snap watch add <file> |
| Sharing state with someone | snap export <id> |
| Long session, lots of checkpoints | snap clean --auto |
| Before a big refactor | snap save + snap pin <id> |
| Agent making autonomous changes | snap record start (agent activity auto-detected) |
| Updating snap CLI version | snap update-rules (refreshes these instructions) |
`

	const snapHeader = "# Snap — Local Checkpoint"

	// CLAUDE.md — update or create
	claudeMdPath := filepath.Join(root, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMdPath); err == nil {
		content := string(data)
		if idx := strings.Index(content, snapHeader); idx != -1 {
			// Replace existing snap section (from header to end of file or next top-level heading)
			before := content[:idx]
			os.WriteFile(claudeMdPath, []byte(strings.TrimRight(before, "\n")+"\n\n"+snapRules), 0644)
			fmt.Println("  Updated CLAUDE.md with latest snap rules")
		} else {
			os.WriteFile(claudeMdPath, append(data, []byte("\n\n"+snapRules)...), 0644)
			fmt.Println("  Appended snap rules to CLAUDE.md")
		}
	} else {
		os.WriteFile(claudeMdPath, []byte(snapRules), 0644)
		fmt.Println("  Created CLAUDE.md with snap rules")
	}

	// .cursorrules — update or create
	cursorPath := filepath.Join(root, ".cursorrules")
	if data, err := os.ReadFile(cursorPath); err == nil {
		content := string(data)
		if idx := strings.Index(content, snapHeader); idx != -1 {
			before := content[:idx]
			os.WriteFile(cursorPath, []byte(strings.TrimRight(before, "\n")+"\n\n"+snapRules), 0644)
			fmt.Println("  Updated .cursorrules with latest snap rules")
		} else {
			os.WriteFile(cursorPath, append(data, []byte("\n\n"+snapRules)...), 0644)
			fmt.Println("  Appended snap rules to .cursorrules")
		}
	} else {
		os.WriteFile(cursorPath, []byte(snapRules), 0644)
		fmt.Println("  Created .cursorrules with snap rules")
	}

	// .github/copilot-instructions.md — update or create
	copilotDir := filepath.Join(root, ".github")
	copilotPath := filepath.Join(copilotDir, "copilot-instructions.md")
	if data, err := os.ReadFile(copilotPath); err == nil {
		content := string(data)
		if idx := strings.Index(content, snapHeader); idx != -1 {
			before := content[:idx]
			os.WriteFile(copilotPath, []byte(strings.TrimRight(before, "\n")+"\n\n"+snapRules), 0644)
			fmt.Println("  Updated .github/copilot-instructions.md with latest snap rules")
		} else {
			os.WriteFile(copilotPath, append(data, []byte("\n\n"+snapRules)...), 0644)
			fmt.Println("  Appended snap rules to .github/copilot-instructions.md")
		}
	} else {
		os.MkdirAll(copilotDir, 0755)
		os.WriteFile(copilotPath, []byte(snapRules), 0644)
		fmt.Println("  Created .github/copilot-instructions.md with snap rules")
	}
}

func cmdUpdateRules() {
	root, err := os.Getwd()
	if err != nil {
		fatal("get working directory: %v", err)
	}

	writeAgentInstructions(root)
	fmt.Println("Agent instruction files updated to latest rules.")
}

func setupClaudeHook() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("  ⚠ Hook setup encountered an error — skipping (snap init succeeded)\n")
		}
	}()

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("  ⚠ Could not determine home directory — skipping hook setup")
		return
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	hookCommand := `if [ -d .snap ]; then FILE=$(echo "$TOOL_INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('file_path',''))" 2>/dev/null); if [ -n "$FILE" ] && [ -f "$FILE" ]; then snap save-file "$FILE" -m "before agent edit" 2>/dev/null; fi; fi`

	var settings map[string]interface{}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		// No settings file — create fresh
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			fmt.Println("  ⚠ Could not parse ~/.claude/settings.json — skipping hook setup")
			return
		}
	}

	// Check if hook already exists
	if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
		if preToolUse, ok := hooks["PreToolUse"].([]interface{}); ok {
			for _, entry := range preToolUse {
				if m, ok := entry.(map[string]interface{}); ok {
					if hooksArr, ok := m["hooks"].([]interface{}); ok {
						for _, h := range hooksArr {
							if hm, ok := h.(map[string]interface{}); ok {
								if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "snap save-file") {
									fmt.Println("  Claude Code hook already configured ✓")
									return
								}
							}
						}
					}
				}
			}
		}
	}

	// Add hook
	hook := map[string]interface{}{
		"matcher": "Edit|Write",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": hookCommand,
				"timeout": 5,
				"async":   true,
			},
		},
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
	}

	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		preToolUse = []interface{}{}
	}

	preToolUse = append(preToolUse, hook)
	hooks["PreToolUse"] = preToolUse
	settings["hooks"] = hooks

	// Ensure .claude directory exists
	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Println("  ⚠ Could not serialize settings — skipping hook setup")
		return
	}

	if err := os.WriteFile(settingsPath, output, 0644); err != nil {
		fmt.Println("  ⚠ Could not write ~/.claude/settings.json — skipping hook setup")
		return
	}

	fmt.Println("  Claude Code hook installed — auto-saves files before every agent edit ✓")
}

func cmdSave() {
	engine := requireInit()

	message := "snapshot"
	description := ""
	var msgParts []string

	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-d" || os.Args[i] == "--desc") && i+1 < len(os.Args) {
			description = os.Args[i+1]
			i++
		} else {
			msgParts = append(msgParts, os.Args[i])
		}
	}

	if len(msgParts) > 0 {
		message = strings.Join(msgParts, " ")
	}

	start := time.Now()
	snap, err := engine.SaveWithDescription(message, description, false)
	if err != nil {
		fatal("save: %v", err)
	}
	elapsed := time.Since(start)

	fmt.Printf("Saved snapshot #%d\n", snap.ID)
	fmt.Printf("  Message:  %s\n", snap.Message)
	if snap.Description != "" {
		fmt.Printf("  Desc:     %s\n", snap.Description)
	}
	fmt.Printf("  Files:    %d\n", snap.FileCount)
	fmt.Printf("  Time:     %s\n", elapsed.Round(time.Millisecond))
}

func cmdList() {
	engine := requireInit()

	snapshots, err := engine.List()
	if err != nil {
		fatal("list: %v", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("No snapshots yet. Run: snap save \"message\"")
		return
	}

	var userSnaps, autoSnaps []*snapshot.Snapshot
	for _, s := range snapshots {
		if s.AutoSave {
			autoSnaps = append(autoSnaps, s)
		} else {
			userSnaps = append(userSnaps, s)
		}
	}

	if len(userSnaps) > 0 {
		fmt.Printf("\033[1m📌 Checkpoints (%d)\033[0m\n\n", len(userSnaps))
		for i, snap := range userSnaps {
			marker := "●"
			if i == len(userSnaps)-1 {
				marker = "◉"
			}
			pinTag := ""
			if snap.Pinned {
				pinTag = " \033[33m⭐ pinned\033[0m"
			}
			fmt.Printf("  %s #%-4d  %s  %s  (%d files)%s\n",
				marker,
				snap.ID,
				snap.Timestamp.Format("Jan 02 15:04"),
				snap.Message,
				snap.FileCount,
				pinTag,
			)
			if snap.Description != "" {
				fmt.Printf("  │         \033[2m%s\033[0m\n", snap.Description)
			}
			if i < len(userSnaps)-1 {
				fmt.Println("  │")
			}
		}
		fmt.Println()
	}

	if len(autoSnaps) > 0 {
		fmt.Printf("\033[2m🔄 Auto-saves (%d)\033[0m\n\n", len(autoSnaps))
		for i, snap := range autoSnaps {
			marker := "○"
			if i == len(autoSnaps)-1 {
				marker = "◎"
			}
			fmt.Printf("  %s #%-4d  %s  %s  (%d files)\n",
				marker,
				snap.ID,
				snap.Timestamp.Format("Jan 02 15:04"),
				snap.Message,
				snap.FileCount,
			)
			if i < len(autoSnaps)-1 {
				fmt.Println("  │")
			}
		}
		fmt.Println()
	}

	fmt.Printf("  Total: %d checkpoints, %d auto-saves\n", len(userSnaps), len(autoSnaps))
}

func cmdRestore() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap restore <id>")
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot id: %s", os.Args[2])
	}

	target, err := engine.Load(id)
	if err != nil {
		fatal("load snapshot: %v", err)
	}

	fmt.Printf("Auto-saving current state before restore...\n")
	autoSnap, err := engine.Save(fmt.Sprintf("auto-save before restore to #%d \"%s\"", id, target.Message), true)
	if err != nil {
		fatal("auto-save failed: %v", err)
	}
	fmt.Printf("  Saved as #%d\n\n", autoSnap.ID)

	fmt.Printf("Restoring to #%d \"%s\"...\n", target.ID, target.Message)

	if err := engine.Restore(id); err != nil {
		fatal("restore: %v", err)
	}

	fmt.Printf("Restored successfully. (%d files)\n", target.FileCount)
	fmt.Printf("\nYour previous state is saved as #%d if you need it back.\n", autoSnap.ID)
}

func cmdDiff() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage:\n  snap diff <id>                  (snapshot vs current)\n  snap diff <id1> <id2>           (snapshot vs snapshot)\n  snap diff <id> <file>           (single file vs current)\n  snap diff <id1> <id2> <file>    (single file between snapshots)")
	}

	id1, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot id: %s", os.Args[2])
	}

	snap1, err := engine.Load(id1)
	if err != nil {
		fatal("load snapshot %d: %v", id1, err)
	}

	var treeB map[string]string
	var labelB string
	var filterFile string
	showDetailed := false
	argIdx := 3

	if argIdx < len(os.Args) {
		nextArg := os.Args[argIdx]
		if nextArg == "-f" || nextArg == "--full" {
			showDetailed = true
			argIdx++
		} else if id2, err2 := strconv.Atoi(nextArg); err2 == nil {
			snap2, err := engine.Load(id2)
			if err != nil {
				fatal("load snapshot %d: %v", id2, err)
			}
			treeB = snap2.Tree
			labelB = fmt.Sprintf("Snapshot #%d \"%s\"", snap2.ID, snap2.Message)
			argIdx++
		} else {
			filterFile = nextArg
			argIdx++
		}
	}

	for i := argIdx; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "-f" || arg == "--full" {
			showDetailed = true
		} else if filterFile == "" {
			filterFile = arg
		}
	}

	if treeB == nil {
		treeB, err = engine.GetCurrentTree()
		if err != nil {
			fatal("get current tree: %v", err)
		}
		labelB = "Current working directory"
	}

	labelA := fmt.Sprintf("Snapshot #%d \"%s\"", snap1.ID, snap1.Message)

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)
	diffEngine := diff.NewEngine(objStore)

	if filterFile != "" {
		fmt.Printf("── %s ──\n", filterFile)
		fmt.Printf("%s  →  %s\n\n", labelA, labelB)

		hashA, existsA := snap1.Tree[filterFile]
		hashB, existsB := treeB[filterFile]

		if !existsA && !existsB {
			fatal("file '%s' not found in either snapshot", filterFile)
		} else if !existsA {
			fmt.Printf("  File was \033[32madded\033[0m (not in snapshot #%d)\n", snap1.ID)
			return
		} else if !existsB {
			fmt.Printf("  File was \033[31mdeleted\033[0m (not in target)\n", )
			return
		} else if hashA == hashB {
			fmt.Println("  No changes in this file.")
			return
		}

		ld, err := diffEngine.DiffFile(hashA, hashB)
		if err != nil {
			fatal("diff file: %v", err)
		}

		output := diff.FormatLineDiff(ld)
		if output != "" {
			fmt.Print(output)
		}
		return
	}

	changes := diff.CompareTrees(snap1.Tree, treeB)

	if len(changes) == 0 {
		fmt.Printf("%s  →  %s\n\n", labelA, labelB)
		fmt.Println("  No differences.")
		return
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	fmt.Printf("%s  →  %s\n\n", labelA, labelB)

	var added, modified, deleted int
	for _, change := range changes {
		switch change.Status {
		case diff.Added:
			fmt.Printf("  \033[32m+ %s\033[0m  (added)\n", change.Path)
			added++
		case diff.Modified:
			fmt.Printf("  \033[33m~ %s\033[0m  (modified)\n", change.Path)
			modified++

			if showDetailed {
				hashA := snap1.Tree[change.Path]
				hashB := treeB[change.Path]
				ld, err := diffEngine.DiffFile(hashA, hashB)
				if err == nil && ld != nil {
					output := diff.FormatLineDiff(ld)
					if output != "" {
						lines := strings.Split(output, "\n")
						for _, line := range lines {
							fmt.Printf("    %s\n", line)
						}
					}
				}
			}
		case diff.Deleted:
			fmt.Printf("  \033[31m- %s\033[0m  (deleted)\n", change.Path)
			deleted++
		}
	}

	fmt.Printf("\n  %d modified, %d added, %d deleted\n", modified, added, deleted)
}

func cmdShow() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage:\n  snap show <id>              (list all files in snapshot)\n  snap show <id> <file>       (show file content at that snapshot)")
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot id: %s", os.Args[2])
	}

	snap, err := engine.Load(id)
	if err != nil {
		fatal("load snapshot %d: %v", id, err)
	}

	if len(os.Args) < 4 {
		fmt.Printf("Snapshot #%d \"%s\" (%s)\n", snap.ID, snap.Message, snap.Timestamp.Format("Jan 02 15:04"))
		fmt.Printf("Files (%d):\n\n", snap.FileCount)

		paths := make([]string, 0, len(snap.Tree))
		for p := range snap.Tree {
			paths = append(paths, p)
		}
		sort.Strings(paths)

		for _, p := range paths {
			fmt.Printf("  %s\n", p)
		}
		return
	}

	filePath := os.Args[3]
	hash, exists := snap.Tree[filePath]
	if !exists {
		fmt.Printf("File '%s' not found in snapshot #%d\n\n", filePath, id)
		fmt.Println("Available files:")
		paths := make([]string, 0, len(snap.Tree))
		for p := range snap.Tree {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		for _, p := range paths {
			fmt.Printf("  %s\n", p)
		}
		os.Exit(1)
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	data, err := objStore.Read(hash)
	if err != nil {
		fatal("read file: %v", err)
	}

	fmt.Printf("\033[36m── %s (snapshot #%d) ──\033[0m\n\n", filePath, id)
	fmt.Print(string(data))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
}

func cmdDelete() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap delete <id>")
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot id: %s", os.Args[2])
	}

	snap, err := engine.Load(id)
	if err != nil {
		fatal("snapshot %d not found", id)
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap", "snapshots", fmt.Sprintf("%04d.json", id))

	if err := os.Remove(snapPath); err != nil {
		fatal("delete snapshot: %v", err)
	}

	fmt.Printf("Deleted snapshot #%d \"%s\"\n", snap.ID, snap.Message)
	_ = engine
}

func cmdClean() {
	engine := requireInit()
	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")

	autoMode := len(os.Args) > 2 && (os.Args[2] == "--auto" || os.Args[2] == "-y")
	dryRun := len(os.Args) > 2 && (os.Args[2] == "--dry-run" || os.Args[2] == "-n")

	snapshots, err := engine.List()
	if err != nil {
		fatal("list snapshots: %v", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("Nothing to clean.")
		return
	}

	// Categorize what can be removed
	type removal struct {
		snap   *snapshot.Snapshot
		reason string
	}

	var toRemove []removal
	var toKeep []*snapshot.Snapshot

	// Track which trees we've seen (for duplicate detection)
	seenTrees := make(map[string]int) // tree hash -> first snapshot ID with this tree

	now := time.Now()
	retention := 7 * 24 * time.Hour // 7 days default

	for _, snap := range snapshots {
		// Never remove pinned
		if snap.Pinned {
			toKeep = append(toKeep, snap)
			continue
		}

		// Build a tree fingerprint for duplicate detection
		treeKey := buildTreeKey(snap.Tree)

		if firstID, exists := seenTrees[treeKey]; exists {
			toRemove = append(toRemove, removal{snap, fmt.Sprintf("duplicate of #%d (identical state)", firstID)})
			continue
		}
		seenTrees[treeKey] = snap.ID

		// Old auto-saves (> retention period)
		if snap.AutoSave && now.Sub(snap.Timestamp) > retention {
			toRemove = append(toRemove, removal{snap, fmt.Sprintf("auto-save older than %d days", int(retention.Hours()/24))})
			continue
		}

		// Superseded auto-saves: if there's a newer auto-save within 5 minutes, this one is redundant
		if snap.AutoSave {
			superseded := false
			for _, other := range snapshots {
				if other.ID > snap.ID && other.AutoSave && other.Timestamp.Sub(snap.Timestamp) < 5*time.Minute {
					superseded = true
					break
				}
			}
			if superseded {
				toRemove = append(toRemove, removal{snap, "superseded by newer auto-save within 5 min"})
				continue
			}
		}

		toKeep = append(toKeep, snap)
	}

	// Calculate sizes
	snapshotsDir := filepath.Join(snapPath, "snapshots")
	var removeSize int64
	for _, r := range toRemove {
		filename := fmt.Sprintf("%04d.json", r.snap.ID)
		if info, err := os.Stat(filepath.Join(snapshotsDir, filename)); err == nil {
			removeSize += info.Size()
		}
	}

	// Count orphaned objects (objects not referenced by any kept snapshot)
	referencedHashes := make(map[string]bool)
	for _, snap := range toKeep {
		for _, hash := range snap.Tree {
			referencedHashes[hash] = true
		}
	}

	var orphanCount int
	var orphanSize int64
	objectsDir := filepath.Join(snapPath, "objects")
	filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// Reconstruct hash from path: objects/ab/cdef... -> abcdef...
		rel, _ := filepath.Rel(objectsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 2 {
			hash := parts[0] + parts[1]
			if !referencedHashes[hash] {
				orphanCount++
				orphanSize += info.Size()
			}
		}
		return nil
	})

	// Display analysis
	totalSize := removeSize + orphanSize
	fmt.Printf("Snap Clean Analysis:\n\n")
	fmt.Printf("  Total snapshots: %d\n", len(snapshots))
	fmt.Printf("  Safe to remove:  %d snapshots\n", len(toRemove))
	fmt.Printf("  Keeping:         %d snapshots\n", len(toKeep))
	fmt.Printf("  Orphaned objects: %d\n", orphanCount)
	fmt.Printf("  Space to free:   %s\n\n", formatSize(totalSize))

	if len(toRemove) > 0 {
		fmt.Println("  Removals:")
		for _, r := range toRemove {
			age := now.Sub(r.snap.Timestamp)
			ageStr := formatAge(age)
			label := "auto"
			if !r.snap.AutoSave {
				label = "user"
			}
			fmt.Printf("    #%-4d [%s] %s (%s) — %s\n", r.snap.ID, label, r.snap.Message, ageStr, r.reason)
		}
		fmt.Println()
	}

	if len(toRemove) == 0 && orphanCount == 0 {
		fmt.Println("  Already clean. Nothing to do.")
		return
	}

	if dryRun {
		fmt.Println("  [dry-run] No changes made.")
		return
	}

	// Confirm or auto-proceed
	if !autoMode {
		fmt.Printf("Proceed? [y/n] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	// Remove snapshots
	removed := 0
	for _, r := range toRemove {
		filename := fmt.Sprintf("%04d.json", r.snap.ID)
		if err := os.Remove(filepath.Join(snapshotsDir, filename)); err == nil {
			removed++
		}
	}

	// Remove orphaned objects
	orphansRemoved := 0
	filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(objectsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 2 {
			hash := parts[0] + parts[1]
			if !referencedHashes[hash] {
				os.Remove(path)
				orphansRemoved++
			}
		}
		return nil
	})

	// Clean empty object directories
	entries, _ := os.ReadDir(objectsDir)
	for _, e := range entries {
		if e.IsDir() {
			dirPath := filepath.Join(objectsDir, e.Name())
			sub, _ := os.ReadDir(dirPath)
			if len(sub) == 0 {
				os.Remove(dirPath)
			}
		}
	}

	fmt.Printf("\nCleaned: %d snapshots removed, %d orphaned objects removed\n", removed, orphansRemoved)
	fmt.Printf("Freed: %s\n", formatSize(totalSize))
}

func buildTreeKey(tree map[string]string) string {
	keys := make([]string, 0, len(tree))
	for k := range tree {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString(":")
		b.WriteString(tree[k])
		b.WriteString(";")
	}
	return b.String()
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	} else if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

func formatAge(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	}
	return fmt.Sprintf("%d days ago", int(d.Hours()/24))
}

func cmdStatus() {
	engine := requireInit()

	snapshots, err := engine.List()
	if err != nil {
		fatal("list snapshots: %v", err)
	}

	if len(snapshots) == 0 {
		fmt.Println("No snapshots yet. Current state is untracked.")
		fmt.Println("Run: snap save \"initial state\"")
		return
	}

	// Find last full-project snapshot (skip single-file saves with <= 5 files)
	var lastSnap *snapshot.Snapshot
	for i := len(snapshots) - 1; i >= 0; i-- {
		if snapshots[i].FileCount > 5 {
			lastSnap = snapshots[i]
			break
		}
	}
	if lastSnap == nil {
		lastSnap = snapshots[len(snapshots)-1]
	}

	currentTree, err := engine.GetCurrentTree()
	if err != nil {
		fatal("get current tree: %v", err)
	}

	changes := diff.CompareTrees(lastSnap.Tree, currentTree)

	fmt.Printf("Last snapshot: #%d \"%s\" (%s)\n\n",
		lastSnap.ID,
		lastSnap.Message,
		lastSnap.Timestamp.Format("Jan 02 15:04"),
	)

	if len(changes) == 0 {
		fmt.Println("  No changes since last snapshot.")
		return
	}

	// Filter out files that are in .snapignore for current tree comparison
	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Path < changes[j].Path
	})

	fmt.Println("  Changes since last snapshot:")
	fmt.Println()

	for _, change := range changes {
		fmt.Printf("    %s %s\n", change.Status.Symbol(), change.Path)
	}

	added, modified, deleted := diff.CountChanges(lastSnap.Tree, currentTree)
	fmt.Printf("\n  %d modified, %d added, %d deleted\n", modified, added, deleted)
}

func cmdSaveFile() {
	_ = requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap save-file <file1> [file2...] [-m message]")
	}

	// Parse args: files and optional -m message
	var files []string
	message := "file checkpoint"

	for i := 2; i < len(os.Args); i++ {
		if (os.Args[i] == "-m" || os.Args[i] == "--message") && i+1 < len(os.Args) {
			message = strings.Join(os.Args[i+1:], " ")
			break
		}
		files = append(files, os.Args[i])
	}

	if len(files) == 0 {
		fatal("usage: snap save-file <file1> [file2...] [-m message]")
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	tree := make(map[string]string)
	for _, filePath := range files {
		fullPath := filepath.Join(root, filePath)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			fatal("read file %s: %v", filePath, err)
		}

		hash, err := objStore.Write(data)
		if err != nil {
			fatal("store file %s: %v", filePath, err)
		}
		tree[filePath] = hash
	}

	engine := snapshot.NewEngine(root)

	// Use first file name for single, or "N files" for multi
	label := files[0]
	if len(files) > 1 {
		label = fmt.Sprintf("%d files", len(files))
	}

	snap, err := engine.SaveSingleFileWithAutoSave(label, message, tree, false)
	if err != nil {
		fatal("save: %v", err)
	}

	fmt.Printf("Saved file checkpoint #%d\n", snap.ID)
	if len(files) == 1 {
		fmt.Printf("  File:     %s\n", files[0])
	} else {
		fmt.Printf("  Files:    %d\n", len(files))
		for _, f := range files {
			fmt.Printf("            %s\n", f)
		}
	}
	fmt.Printf("  Message:  %s\n", message)
}

func cmdRestoreFile() {
	_ = requireInit()

	if len(os.Args) < 4 {
		fatal("usage: snap restore-file <snapshot-id> <file>")
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot id: %s", os.Args[2])
	}

	filePath := os.Args[3]

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	engine := snapshot.NewEngine(root)
	snap, err := engine.Load(id)
	if err != nil {
		fatal("load snapshot: %v", err)
	}

	hash, exists := snap.Tree[filePath]
	if !exists {
		fatal("file '%s' not found in snapshot #%d", filePath, id)
	}

	// Auto-save current file state before restoring
	fullPath := filepath.Join(root, filePath)
	if currentData, err := os.ReadFile(fullPath); err == nil {
		currentHash, _ := objStore.Write(currentData)
		tree := map[string]string{filePath: currentHash}
		autoSnap, _ := engine.SaveSingleFileWithAutoSave(filePath, fmt.Sprintf("auto-save %s before restore from #%d", filePath, id), tree, true)
		if autoSnap != nil {
			fmt.Printf("Auto-saved current %s as #%d\n", filePath, autoSnap.ID)
		}
	}

	data, err := objStore.Read(hash)
	if err != nil {
		fatal("read object: %v", err)
	}

	os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		fatal("write file: %v", err)
	}

	fmt.Printf("Restored %s from snapshot #%d\n", filePath, id)
}

func cmdWatch() {
	_ = requireInit()

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	watchFile := filepath.Join(snapPath, "watchlist.json")

	if len(os.Args) < 3 {
		// Show current watchlist
		files := loadWatchlist(watchFile)
		if len(files) == 0 {
			fmt.Println("No files being watched.")
			fmt.Println("Usage: snap watch <file>     — add file to watchlist")
			fmt.Println("       snap watch rm <file>  — remove from watchlist")
			return
		}
		fmt.Printf("👁  Watched files (%d):\n\n", len(files))
		for _, f := range files {
			fmt.Printf("  • %s\n", f)
		}
		fmt.Println("\nThese files get auto-checkpointed on every change.")
		return
	}

	if os.Args[2] == "rm" || os.Args[2] == "remove" {
		if len(os.Args) < 4 {
			fatal("usage: snap watch rm <file>")
		}
		filePath := os.Args[3]
		files := loadWatchlist(watchFile)
		var updated []string
		for _, f := range files {
			if f != filePath {
				updated = append(updated, f)
			}
		}
		saveWatchlist(watchFile, updated)
		fmt.Printf("Removed %s from watchlist\n", filePath)
		return
	}

	filePath := os.Args[2]

	// Verify file exists
	fullPath := filepath.Join(root, filePath)
	if _, err := os.Stat(fullPath); err != nil {
		fatal("file not found: %s", filePath)
	}

	files := loadWatchlist(watchFile)
	for _, f := range files {
		if f == filePath {
			fmt.Printf("%s is already being watched\n", filePath)
			return
		}
	}

	files = append(files, filePath)
	saveWatchlist(watchFile, files)
	fmt.Printf("👁  Now watching: %s\n", filePath)
	fmt.Println("   Auto-checkpoint on every change detected.")
}

func loadWatchlist(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var files []string
	json.Unmarshal(data, &files)
	return files
}

func saveWatchlist(path string, files []string) {
	data, _ := json.MarshalIndent(files, "", "  ")
	os.WriteFile(path, data, 0644)
}

func cmdPin(pin bool) {
	engine := requireInit()

	if len(os.Args) < 3 {
		if pin {
			fatal("usage: snap pin <id>")
		} else {
			fatal("usage: snap unpin <id>")
		}
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot id: %s", os.Args[2])
	}

	if err := engine.SetPinned(id, pin); err != nil {
		fatal("pin: %v", err)
	}

	if pin {
		fmt.Printf("📌 Pinned snapshot #%d — will never be auto-deleted\n", id)
	} else {
		fmt.Printf("Unpinned snapshot #%d\n", id)
	}
}

func cmdRecord() {
	_ = requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap record <start|stop|status>")
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")

	switch os.Args[2] {
	case "start":
		pidPath := filepath.Join(snapPath, "recorder.pid")
		if _, err := os.Stat(pidPath); err == nil {
			pidData, _ := os.ReadFile(pidPath)
			pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
			if pid > 0 {
				proc, err := os.FindProcess(pid)
				if err == nil && isProcessRunning(proc) {
					fmt.Println("⏺  Recording already running.")
					return
				}
			}
			os.Remove(pidPath)
		}

		// Daemonize: fork self with --daemon flag
		if len(os.Args) > 3 && os.Args[3] == "--daemon" {
			// Save initial checkpoint before recording
			engine := snapshot.NewEngine(root)
			engine.Save("recording started", true)

			rec := recorder.New(root, recorder.DefaultConfig())
			if err := rec.Start(); err != nil {
				fatal("start recording: %v", err)
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt)
			<-sigCh

			rec.Stop()
			return
		}

		// Launch daemon process
		exe, _ := os.Executable()
		cmd := execCommand(exe, "record", "start", "--daemon")
		cmd.Dir = root
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			fatal("start daemon: %v", err)
		}
		cmd.Process.Release()

		time.Sleep(200 * time.Millisecond)

		fmt.Println("⏺  Recording started — watching all file changes")
		fmt.Println("   Every change is tracked with timestamp.")
		fmt.Println("   Use 'snap timeline' to see changes.")
		fmt.Println("   Use 'snap rewind' to jump to any moment.")
		fmt.Println("   Use 'snap record stop' to stop recording.")

	case "stop":
		pidPath := filepath.Join(snapPath, "recorder.pid")
		pidData, err := os.ReadFile(pidPath)
		if err != nil {
			fmt.Println("No recording session running.")
			return
		}

		pid, _ := strconv.Atoi(strings.TrimSpace(string(pidData)))
		if pid > 0 {
			proc, err := os.FindProcess(pid)
			if err == nil {
				proc.Signal(os.Interrupt)
			}
		}

		os.Remove(pidPath)
		fmt.Println("⏹  Recording stopped.")

	case "status":
		pidPath := filepath.Join(snapPath, "recorder.pid")
		if _, err := os.Stat(pidPath); err == nil {
			pidData, _ := os.ReadFile(pidPath)
			fmt.Printf("⏺  Recording active (PID %s)\n", strings.TrimSpace(string(pidData)))

			tl := loadTimeline(snapPath)
			changes, _ := tl.LoadAll()
			if len(changes) > 0 {
				duration := changes[len(changes)-1].Timestamp.Sub(changes[0].Timestamp)
				fmt.Printf("   %d changes recorded over %s\n", len(changes), duration.Round(time.Second))

				usage, _ := tl.DiskUsage()
				fmt.Printf("   Storage: %.1f KB\n", float64(usage)/1024)
			}
		} else {
			fmt.Println("⏹  Not recording.")
		}

	default:
		fatal("usage: snap record <start|stop|status>")
	}
}

func cmdRewind() {
	_ = requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap rewind <time>\n\nExamples:\n  snap rewind \"5 minutes ago\"\n  snap rewind \"2:47 PM\"\n  snap rewind \"14:30\"\n  snap rewind \"2024-08-16 14:30:05\"\n  snap rewind \"Aug 16 14:30\"")
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")

	timeStr := strings.Join(os.Args[2:], " ")
	target := parseTimeSpec(timeStr)

	if target.IsZero() {
		fatal("couldn't parse time: %s\n\nExamples: \"5 minutes ago\", \"2:47 PM\", \"14:30\", \"2024-08-16 14:30:05\"", timeStr)
	}

	tl := loadTimeline(snapPath)
	state := tl.GetStateAt(target)

	if len(state) == 0 {
		fatal("no recorded state at %s", target.Format("15:04:05"))
	}

	objStore := store.New(snapPath)

	// Auto-save before rewind
	engine := snapshot.NewEngine(root)
	autoSnap, err := engine.Save(fmt.Sprintf("auto-save before rewind to %s", target.Format("15:04:05")), true)
	if err != nil {
		fatal("auto-save: %v", err)
	}
	fmt.Printf("Auto-saved as #%d\n", autoSnap.ID)

	restored := 0
	for path, hash := range state {
		data, err := objStore.Read(hash)
		if err != nil {
			continue
		}

		fullPath := filepath.Join(root, path)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, data, 0644)
		restored++
	}

	fmt.Printf("⏪ Rewound to %s (%d files restored)\n", target.Format("15:04:05"), restored)
}

func cmdTimeline() {
	_ = requireInit()

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	tl := loadTimeline(snapPath)

	changes, err := tl.LoadAll()
	if err != nil || len(changes) == 0 {
		fmt.Println("No timeline data. Start recording with: snap record start")
		return
	}

	limit := 20
	if len(os.Args) > 2 {
		if n, err := strconv.Atoi(os.Args[2]); err == nil {
			limit = n
		}
	}

	start := 0
	if len(changes) > limit {
		start = len(changes) - limit
	}

	tl.DetectAgentBurst(changes)

	fmt.Printf("Timeline — last %d changes (total: %d)\n\n", min(limit, len(changes)), len(changes))

	timeFmt := "Jan 02 15:04:05"

	for i := start; i < len(changes); i++ {
		c := changes[i]
		icon := "  "
		switch c.Action {
		case "create":
			icon = "\033[32m+\033[0m"
		case "modify":
			icon = "\033[33m~\033[0m"
		case "delete":
			icon = "\033[31m-\033[0m"
		}

		agent := ""
		if c.IsAgent {
			agent = " \033[35m[agent]\033[0m"
		}

		fmt.Printf("  %s %s  %s %s%s\n",
			c.Timestamp.Format(timeFmt),
			icon,
			c.Path,
			c.Action,
			agent,
		)
	}

	fmt.Printf("\n  Span: %s → %s\n",
		changes[0].Timestamp.Format(timeFmt),
		changes[len(changes)-1].Timestamp.Format(timeFmt),
	)
	fmt.Println("  Tip: copy a timestamp above and use: snap rewind \"<timestamp>\"")
}

func loadTimeline(snapPath string) *delta.Timeline {
	tl := delta.NewTimeline(snapPath)
	tl.Init()
	return tl
}

func parseTimeSpec(spec string) time.Time {
	now := time.Now()

	if strings.Contains(spec, "ago") {
		parts := strings.Fields(spec)
		if len(parts) >= 2 {
			n, err := strconv.Atoi(parts[0])
			if err != nil {
				return time.Time{}
			}
			unit := parts[1]
			switch {
			case strings.HasPrefix(unit, "second"):
				return now.Add(-time.Duration(n) * time.Second)
			case strings.HasPrefix(unit, "minute"):
				return now.Add(-time.Duration(n) * time.Minute)
			case strings.HasPrefix(unit, "hour"):
				return now.Add(-time.Duration(n) * time.Hour)
			}
		}
		return time.Time{}
	}

	// Full date-time formats (exact timestamps)
	fullFormats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02T15:04",
		"Jan 2 15:04:05",
		"Jan 2 15:04",
		"Jan 02 15:04:05",
		"Jan 02 15:04",
		"2 Jan 15:04:05",
		"2 Jan 15:04",
		"02 Jan 15:04",
	}

	for _, format := range fullFormats {
		t, err := time.ParseInLocation(format, spec, now.Location())
		if err == nil {
			// If year is zero (formats without year), use current year
			if t.Year() == 0 {
				t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			}
			return t
		}
	}

	// Time-only formats (assumes today)
	timeFormats := []string{
		"3:04 PM",
		"3:04PM",
		"3:04:05 PM",
		"3:04:05PM",
		"15:04",
		"15:04:05",
	}

	for _, format := range timeFormats {
		t, err := time.Parse(format, spec)
		if err == nil {
			return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
		}
	}

	return time.Time{}
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

func isProcessRunning(proc *os.Process) bool {
	err := proc.Signal(nil)
	return err == nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func requireInit() *snapshot.Engine {
	root, err := os.Getwd()
	if err != nil {
		fatal("get working directory: %v", err)
	}

	engine := snapshot.NewEngine(root)
	if !engine.IsInitialized() {
		fmt.Fprintf(os.Stderr, "Not a snap project. Run 'snap init' first.\n")
		os.Exit(1)
	}

	return engine
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

// Snappack binary format:
// [MAGIC: "SNAPPACK\x01" 9 bytes]
// [FLAGS: 1 byte — bit 0 = encrypted]
// [METADATA_LEN: 4 bytes big-endian]
// [METADATA: JSON bytes]
// [BLOB_COUNT: 4 bytes big-endian]
// [BLOB: hash_hex (64 bytes) + data_len (4 bytes) + compressed_data (data_len bytes)] × N
// [CHECKSUM: SHA-256 of everything above (32 bytes)]
//
// If encrypted: after MAGIC+FLAGS, everything is AES-256-GCM encrypted as one block.

var snappackMagic = []byte("SNAPPACK\x01")

func cmdExport() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap export <id> [-o output] [-p password]")
	}

	id, err := strconv.Atoi(os.Args[2])
	if err != nil {
		fatal("invalid snapshot ID: %s", os.Args[2])
	}

	var outputFile string
	var password string

	for i := 3; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "-o", "--output":
			if i+1 < len(os.Args) {
				outputFile = os.Args[i+1]
				i++
			}
		case "-p", "--password":
			if i+1 < len(os.Args) {
				password = os.Args[i+1]
				i++
			}
		}
	}

	snapshots, err := engine.List()
	if err != nil {
		fatal("list: %v", err)
	}

	var snap *snapshot.Snapshot
	for _, s := range snapshots {
		if s.ID == id {
			snap = s
			break
		}
	}
	if snap == nil {
		fatal("snapshot #%d not found", id)
	}

	if outputFile == "" {
		safeName := strings.ReplaceAll(snap.Message, " ", "-")
		safeName = strings.ReplaceAll(safeName, "/", "_")
		if len(safeName) > 40 {
			safeName = safeName[:40]
		}
		outputFile = fmt.Sprintf("checkpoint-%d-%s.snap", id, safeName)
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	// Build metadata
	meta := map[string]interface{}{
		"id":          snap.ID,
		"message":     snap.Message,
		"description": snap.Description,
		"timestamp":   snap.Timestamp.Format(time.RFC3339),
		"file_count":  snap.FileCount,
		"tree":        snap.Tree,
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		fatal("marshal metadata: %v", err)
	}

	// Collect unique blobs
	uniqueHashes := make(map[string]bool)
	for _, hash := range snap.Tree {
		uniqueHashes[hash] = true
	}

	// Build payload
	var payload []byte

	// Metadata length + data
	metaLen := make([]byte, 4)
	binary.BigEndian.PutUint32(metaLen, uint32(len(metaJSON)))
	payload = append(payload, metaLen...)
	payload = append(payload, metaJSON...)

	// Blob count
	blobCount := make([]byte, 4)
	binary.BigEndian.PutUint32(blobCount, uint32(len(uniqueHashes)))
	payload = append(payload, blobCount...)

	// Blobs
	for hash := range uniqueHashes {
		data, err := objStore.ReadRaw(hash)
		if err != nil {
			fatal("read blob %s: %v", hash[:8], err)
		}

		// Hash (64 bytes hex string)
		payload = append(payload, []byte(hash)...)

		// Data length + data
		dataLen := make([]byte, 4)
		binary.BigEndian.PutUint32(dataLen, uint32(len(data)))
		payload = append(payload, dataLen...)
		payload = append(payload, data...)
	}

	// Checksum of payload
	checksum := sha256.Sum256(payload)
	payload = append(payload, checksum[:]...)

	// Build final file
	var output []byte
	flags := byte(0)
	if password != "" {
		flags = 1
	}
	output = append(output, snappackMagic...)
	output = append(output, flags)

	if password != "" {
		// Encrypt payload with AES-256-GCM
		key := sha256.Sum256([]byte(password))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			fatal("create cipher: %v", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			fatal("create GCM: %v", err)
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
			fatal("generate nonce: %v", err)
		}
		encrypted := gcm.Seal(nonce, nonce, payload, nil)
		output = append(output, encrypted...)
	} else {
		output = append(output, payload...)
	}

	if err := os.WriteFile(outputFile, output, 0644); err != nil {
		fatal("write file: %v", err)
	}

	fmt.Printf("Exported snapshot #%d → %s\n", id, outputFile)
	fmt.Printf("  Message:  %s\n", snap.Message)
	fmt.Printf("  Files:    %d\n", snap.FileCount)
	fmt.Printf("  Size:     %s\n", formatSize(int64(len(output))))
	if password != "" {
		fmt.Printf("  Password: protected ✓\n")
	}
	fmt.Printf("\nShare this file. Import with: snap import %s\n", outputFile)
}

func cmdImport() {
	engine := requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap import <file.snap> [-p password]")
	}

	inputFile := os.Args[2]
	var password string

	for i := 3; i < len(os.Args); i++ {
		if (os.Args[i] == "-p" || os.Args[i] == "--password") && i+1 < len(os.Args) {
			password = os.Args[i+1]
			i++
		}
	}

	data, err := os.ReadFile(inputFile)
	if err != nil {
		fatal("read file: %v", err)
	}

	// Verify magic
	if len(data) < 10 || string(data[:9]) != string(snappackMagic) {
		fatal("not a valid .snap file (bad magic)")
	}

	flags := data[9]
	payload := data[10:]

	// Decrypt if needed
	if flags&1 != 0 {
		if password == "" {
			fatal("this file is password-protected. Use: snap import %s -p \"password\"", inputFile)
		}
		key := sha256.Sum256([]byte(password))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			fatal("create cipher: %v", err)
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			fatal("create GCM: %v", err)
		}
		nonceSize := gcm.NonceSize()
		if len(payload) < nonceSize {
			fatal("corrupted file (too short)")
		}
		nonce, ciphertext := payload[:nonceSize], payload[nonceSize:]
		payload, err = gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			fatal("wrong password or corrupted file")
		}
	}

	// Verify checksum
	if len(payload) < 32 {
		fatal("corrupted file (no checksum)")
	}
	content := payload[:len(payload)-32]
	expectedChecksum := payload[len(payload)-32:]
	actualChecksum := sha256.Sum256(content)
	if string(actualChecksum[:]) != string(expectedChecksum) {
		fatal("corrupted file (checksum mismatch)")
	}

	offset := 0

	// Read metadata
	if offset+4 > len(content) {
		fatal("corrupted file (no metadata length)")
	}
	metaLen := binary.BigEndian.Uint32(content[offset : offset+4])
	offset += 4

	if offset+int(metaLen) > len(content) {
		fatal("corrupted file (metadata truncated)")
	}
	metaJSON := content[offset : offset+int(metaLen)]
	offset += int(metaLen)

	var meta map[string]interface{}
	if err := json.Unmarshal(metaJSON, &meta); err != nil {
		fatal("parse metadata: %v", err)
	}

	// Read blob count
	if offset+4 > len(content) {
		fatal("corrupted file (no blob count)")
	}
	blobCount := binary.BigEndian.Uint32(content[offset : offset+4])
	offset += 4

	// Read and store blobs
	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	for i := 0; i < int(blobCount); i++ {
		if offset+64 > len(content) {
			fatal("corrupted file (blob %d hash truncated)", i)
		}
		hash := string(content[offset : offset+64])
		offset += 64

		if offset+4 > len(content) {
			fatal("corrupted file (blob %d length truncated)", i)
		}
		dataLen := binary.BigEndian.Uint32(content[offset : offset+4])
		offset += 4

		if offset+int(dataLen) > len(content) {
			fatal("corrupted file (blob %d data truncated)", i)
		}
		blobData := content[offset : offset+int(dataLen)]
		offset += int(dataLen)

		// Write blob (skips if already exists via dedup)
		if !objStore.Has(hash) {
			if err := objStore.WriteRaw(hash, blobData); err != nil {
				fatal("write blob %s: %v", hash[:8], err)
			}
		}
	}

	// Recreate snapshot
	tree := make(map[string]string)
	if treeData, ok := meta["tree"].(map[string]interface{}); ok {
		for path, hash := range treeData {
			tree[path] = hash.(string)
		}
	}

	message := "imported"
	if msg, ok := meta["message"].(string); ok {
		message = fmt.Sprintf("[imported] %s", msg)
	}
	description := ""
	if desc, ok := meta["description"].(string); ok {
		description = desc
	}

	snap, err := engine.SaveFromTree(tree, message, description)
	if err != nil {
		fatal("create snapshot: %v", err)
	}

	fmt.Printf("Imported → Snapshot #%d\n", snap.ID)
	fmt.Printf("  Message:  %s\n", message)
	fmt.Printf("  Files:    %d\n", len(tree))
	fmt.Printf("  Blobs:    %d imported\n", blobCount)
	if desc, ok := meta["description"].(string); ok && desc != "" {
		fmt.Printf("  Desc:     %s\n", desc)
	}
}

func printUsage() {
	fmt.Printf(`snap v%s — Local development checkpoint tool

Usage:
  snap <command> [arguments]

Checkpoints:
  init                       Initialize snap in current directory
  save [message] [-d "desc"] Save a snapshot (description optional)
  list, ls                   List all snapshots
  show <id>                  List all files in a snapshot
  show <id> <file>           View file content at that snapshot
  restore <id>               Restore to a snapshot (auto-saves current state first)
  diff <id>                  Diff snapshot vs current working directory
  diff <id1> <id2>           Diff between two snapshots
  diff <id> <file>           Diff a specific file vs current
  diff <id1> <id2> <file>    Diff a specific file between two snapshots
  diff ... -f                Show full line-level diff for all modified files
  delete <id>, rm <id>       Delete a snapshot
  pin <id>                   Pin a snapshot (never auto-deleted)
  unpin <id>                 Unpin a snapshot
  save-file <file> [msg]     Save checkpoint of a single file
  restore-file <id> <file>   Restore a single file from any snapshot
  status                     Show changes since last snapshot

Continuous Recording:
  record start               Start recording all file changes
  record stop                Stop recording
  record status              Show recording status and stats
  rewind <time>              Rewind project to any recorded moment
  timeline [n]               Show last n changes (default 20)

Sharing:
  export <id> [-o file]      Export checkpoint as .snap file
  export <id> -p "password"  Export with password protection
  import <file>              Import a .snap file into timeline
  import <file> -p "pass"   Import a password-protected file

Maintenance:
  clean                      Analyze and remove safe-to-delete snapshots
  clean --dry-run            Show what would be removed (no changes)
  clean --auto               Remove without confirmation (for automation)
  update-rules               Update AI agent instruction files to latest rules
  setup-hooks                Install Claude Code auto-save hook (~/.claude/settings.json)

Time Formats (for rewind):
  "5 minutes ago"            Relative time
  "2:47 PM"                  12-hour format
  "14:30"                    24-hour format
  "14:30:05"                 With seconds (same as timeline output)
  "2024-08-16 14:30:05"      Full date-time
  "Aug 16 14:30"             Month day time

Other:
  version                    Show version
  help                       Show this help

Examples:
  snap init
  snap save "before refactoring auth"
  snap save "auth done" -d "JWT tokens working, refresh pending"
  snap list
  snap pin 3
  snap record start
  snap rewind "5 minutes ago"
  snap timeline
  snap restore 3
  snap status

`, version)
}
