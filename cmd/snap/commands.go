package main

import (
	"encoding/json"
	"fmt"
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

func cmdInit() {
	root, err := os.Getwd()
	if err != nil {
		fatal("get working directory: %v", err)
	}

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

	snap, err := engine.Save("initial state", false)
	if err == nil {
		fmt.Printf("  Saved initial checkpoint #%d (%d files)\n", snap.ID, snap.FileCount)
	}

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
			fmt.Printf("  File was \033[31mdeleted\033[0m (not in target)\n")
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

	type removal struct {
		snap   *snapshot.Snapshot
		reason string
	}

	var toRemove []removal
	var toKeep []*snapshot.Snapshot

	seenTrees := make(map[string]int)

	now := time.Now()
	retention := 7 * 24 * time.Hour

	for _, snap := range snapshots {
		if snap.Pinned {
			toKeep = append(toKeep, snap)
			continue
		}

		treeKey := buildTreeKey(snap.Tree)

		if firstID, exists := seenTrees[treeKey]; exists {
			toRemove = append(toRemove, removal{snap, fmt.Sprintf("duplicate of #%d (identical state)", firstID)})
			continue
		}
		seenTrees[treeKey] = snap.ID

		if snap.AutoSave && now.Sub(snap.Timestamp) > retention {
			toRemove = append(toRemove, removal{snap, fmt.Sprintf("auto-save older than %d days", int(retention.Hours()/24))})
			continue
		}

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

	snapshotsDir := filepath.Join(snapPath, "snapshots")
	var removeSize int64
	for _, r := range toRemove {
		filename := fmt.Sprintf("%04d.json", r.snap.ID)
		if info, err := os.Stat(filepath.Join(snapshotsDir, filename)); err == nil {
			removeSize += info.Size()
		}
	}

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

	if !autoMode {
		fmt.Printf("Proceed? [y/n] ")
		var answer string
		fmt.Scanln(&answer)
		if answer != "y" && answer != "Y" {
			fmt.Println("Cancelled.")
			return
		}
	}

	removed := 0
	for _, r := range toRemove {
		filename := fmt.Sprintf("%04d.json", r.snap.ID)
		if err := os.Remove(filepath.Join(snapshotsDir, filename)); err == nil {
			removed++
		}
	}

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

		if len(os.Args) > 3 && os.Args[3] == "--daemon" {
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

		exe, _ := os.Executable()
		cmd := exec.Command(exe, "record", "start", "--daemon")
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

func cmdUpdateRules() {
	root, err := os.Getwd()
	if err != nil {
		fatal("get working directory: %v", err)
	}

	writeAgentInstructions(root)
	fmt.Println("Agent instruction files updated to latest rules.")
}

func loadTimeline(snapPath string) *delta.Timeline {
	tl := delta.NewTimeline(snapPath)
	tl.Init()
	return tl
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
