package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nihalkumar/snap/internal/snapshot"
)

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
			if t.Year() == 0 {
				t = time.Date(now.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			}
			return t
		}
	}

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
