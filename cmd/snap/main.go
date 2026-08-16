package main

import (
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
	case "record":
		cmdRecord()
	case "rewind":
		cmdRewind()
	case "timeline":
		cmdTimeline()
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
		fixed, err := engine.Repair()
		if err != nil {
			fatal("repair: %v", err)
		}
		if fixed > 0 {
			fmt.Printf("Repaired .snap/ structure (%d broken references removed)\n", fixed)
		} else {
			fmt.Println("Already initialized. Structure verified ✓")
		}
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

func cmdSaveFile() {
	_ = requireInit()

	if len(os.Args) < 3 {
		fatal("usage: snap save-file <file> [message]")
	}

	filePath := os.Args[2]
	message := "file checkpoint"
	if len(os.Args) > 3 {
		message = strings.Join(os.Args[3:], " ")
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")
	objStore := store.New(snapPath)

	fullPath := filepath.Join(root, filePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		fatal("read file: %v", err)
	}

	hash, err := objStore.Write(data)
	if err != nil {
		fatal("store file: %v", err)
	}

	engine := snapshot.NewEngine(root)
	tree := map[string]string{filePath: hash}

	snap, err := engine.SaveSingleFile(filePath, message, tree)
	if err != nil {
		fatal("save: %v", err)
	}

	fmt.Printf("Saved file checkpoint #%d\n", snap.ID)
	fmt.Printf("  File:     %s\n", filePath)
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

	data, err := objStore.Read(hash)
	if err != nil {
		fatal("read object: %v", err)
	}

	fullPath := filepath.Join(root, filePath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		fatal("write file: %v", err)
	}

	fmt.Printf("Restored %s from snapshot #%d\n", filePath, id)
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
		fatal("usage: snap rewind <time>\n\nExamples:\n  snap rewind \"5 minutes ago\"\n  snap rewind \"2:47 PM\"\n  snap rewind \"14:30\"")
	}

	root, _ := os.Getwd()
	snapPath := filepath.Join(root, ".snap")

	timeStr := strings.Join(os.Args[2:], " ")
	target := parseTimeSpec(timeStr)

	if target.IsZero() {
		fatal("couldn't parse time: %s\n\nExamples: \"5 minutes ago\", \"2:47 PM\", \"14:30\"", timeStr)
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
			c.Timestamp.Format("15:04:05"),
			icon,
			c.Path,
			c.Action,
			agent,
		)
	}

	fmt.Printf("\n  Span: %s → %s\n",
		changes[0].Timestamp.Format("15:04:05"),
		changes[len(changes)-1].Timestamp.Format("15:04:05"),
	)
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

	formats := []string{
		"3:04 PM",
		"3:04PM",
		"15:04",
		"15:04:05",
		"3:04:05 PM",
	}

	for _, format := range formats {
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

Time Formats:
  "5 minutes ago"            Relative time
  "2:47 PM"                  12-hour format
  "14:30"                    24-hour format

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
