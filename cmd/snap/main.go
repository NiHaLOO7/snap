package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nihalkumar/snap/internal/diff"
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

	engine := snapshot.NewEngine(root)
	if engine.IsInitialized() {
		fmt.Println("Already initialized in this directory.")
		return
	}

	if err := engine.Init(); err != nil {
		fatal("initialize: %v", err)
	}

	fmt.Printf("Initialized snap in %s/.snap/\n", root)
	fmt.Println("\nReady to save snapshots. Run:")
	fmt.Println("  snap save \"initial state\"")
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
			fmt.Printf("  %s #%-4d  %s  %s  (%d files)\n",
				marker,
				snap.ID,
				snap.Timestamp.Format("Jan 02 15:04"),
				snap.Message,
				snap.FileCount,
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

	lastSnap := snapshots[len(snapshots)-1]

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

func printUsage() {
	fmt.Printf(`snap v%s — Local development checkpoint tool

Usage:
  snap <command> [arguments]

Commands:
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
  status                     Show changes since last snapshot
  version                    Show version
  help                       Show this help

Examples:
  snap init
  snap save "before refactoring auth"
  snap save "auth done" -d "JWT tokens working, refresh pending"
  snap list
  snap show 3
  snap show 3 src/main.go
  snap diff 3
  snap diff 3 7
  snap diff 1 2 src/main.go
  snap diff 3 -f
  snap restore 3
  snap delete 2
  snap status

`, version)
}
