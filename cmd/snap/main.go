package main

import (
	"fmt"
	"os"
)

const version = "1.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
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
	case "search":
		cmdSearch()
	case "grep":
		cmdGrep()
	case "update-rules":
		cmdUpdateRules()
	case "setup-hooks":
		setupClaudeHook()
	case "version":
		fmt.Printf("snap v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
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

Search:
  search <query>             Fuzzy search files by name across snapshots
  search <query> --json      Output results as JSON
  grep <pattern>             Search file contents across snapshots
  grep <pattern> --regex     Use regex pattern matching
  grep <pattern> -i          Case-insensitive search
  grep <pattern> --json      Output results as JSON

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
