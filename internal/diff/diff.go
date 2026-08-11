package diff

import (
	"fmt"
	"strings"

	"github.com/nihalkumar/snap/internal/store"
)

type FileChange struct {
	Path    string
	Status  ChangeStatus
	Added   int
	Removed int
}

type ChangeStatus int

const (
	Added    ChangeStatus = iota
	Modified
	Deleted
)

func (s ChangeStatus) String() string {
	switch s {
	case Added:
		return "added"
	case Modified:
		return "modified"
	case Deleted:
		return "deleted"
	default:
		return "unknown"
	}
}

func (s ChangeStatus) Symbol() string {
	switch s {
	case Added:
		return "+"
	case Modified:
		return "~"
	case Deleted:
		return "-"
	default:
		return "?"
	}
}

type LineDiff struct {
	Hunks []Hunk
}

type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []DiffLine
}

type DiffLine struct {
	Type    LineType
	Content string
}

type LineType int

const (
	Context LineType = iota
	Addition
	Deletion
)

type Engine struct {
	objStore *store.ObjectStore
}

func NewEngine(objStore *store.ObjectStore) *Engine {
	return &Engine{objStore: objStore}
}

func CompareTrees(treeA, treeB map[string]string) []FileChange {
	var changes []FileChange

	for path, hashB := range treeB {
		hashA, exists := treeA[path]
		if !exists {
			changes = append(changes, FileChange{Path: path, Status: Added})
		} else if hashA != hashB {
			changes = append(changes, FileChange{Path: path, Status: Modified})
		}
	}

	for path := range treeA {
		if _, exists := treeB[path]; !exists {
			changes = append(changes, FileChange{Path: path, Status: Deleted})
		}
	}

	return changes
}

func (e *Engine) DiffFile(hashA, hashB string) (*LineDiff, error) {
	dataA, err := e.objStore.Read(hashA)
	if err != nil {
		return nil, fmt.Errorf("read old version: %w", err)
	}

	dataB, err := e.objStore.Read(hashB)
	if err != nil {
		return nil, fmt.Errorf("read new version: %w", err)
	}

	linesA := strings.Split(string(dataA), "\n")
	linesB := strings.Split(string(dataB), "\n")

	return computeDiff(linesA, linesB), nil
}

func DiffLines(contentA, contentB []byte) *LineDiff {
	linesA := strings.Split(string(contentA), "\n")
	linesB := strings.Split(string(contentB), "\n")
	return computeDiff(linesA, linesB)
}

func computeDiff(a, b []string) *LineDiff {
	lcs := longestCommonSubsequence(a, b)

	var edits []DiffLine
	i, j, k := 0, 0, 0

	for k < len(lcs) {
		for i < len(a) && a[i] != lcs[k] {
			edits = append(edits, DiffLine{Type: Deletion, Content: a[i]})
			i++
		}
		for j < len(b) && b[j] != lcs[k] {
			edits = append(edits, DiffLine{Type: Addition, Content: b[j]})
			j++
		}
		edits = append(edits, DiffLine{Type: Context, Content: lcs[k]})
		i++
		j++
		k++
	}

	for i < len(a) {
		edits = append(edits, DiffLine{Type: Deletion, Content: a[i]})
		i++
	}
	for j < len(b) {
		edits = append(edits, DiffLine{Type: Addition, Content: b[j]})
		j++
	}

	hunks := buildHunks(edits)
	return &LineDiff{Hunks: hunks}
}

func longestCommonSubsequence(a, b []string) []string {
	n := len(a)
	m := len(b)

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if a[i-1] == b[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				if dp[i-1][j] > dp[i][j-1] {
					dp[i][j] = dp[i-1][j]
				} else {
					dp[i][j] = dp[i][j-1]
				}
			}
		}
	}

	lcs := make([]string, 0, dp[n][m])
	i, j := n, m
	for i > 0 && j > 0 {
		if a[i-1] == b[j-1] {
			lcs = append([]string{a[i-1]}, lcs...)
			i--
			j--
		} else if dp[i-1][j] > dp[i][j-1] {
			i--
		} else {
			j--
		}
	}

	return lcs
}

func buildHunks(edits []DiffLine) []Hunk {
	if len(edits) == 0 {
		return nil
	}

	contextLines := 3
	var hunks []Hunk
	var currentHunk *Hunk
	oldLine := 1
	newLine := 1

	for i, edit := range edits {
		isChange := edit.Type != Context

		if isChange && currentHunk == nil {
			start := i - contextLines
			if start < 0 {
				start = 0
			}
			currentHunk = &Hunk{
				OldStart: oldLine - (i - start),
				NewStart: newLine - (i - start),
			}
			for j := start; j < i; j++ {
				currentHunk.Lines = append(currentHunk.Lines, edits[j])
				currentHunk.OldCount++
				currentHunk.NewCount++
			}
		}

		if currentHunk != nil {
			currentHunk.Lines = append(currentHunk.Lines, edit)
			switch edit.Type {
			case Context:
				currentHunk.OldCount++
				currentHunk.NewCount++
			case Addition:
				currentHunk.NewCount++
			case Deletion:
				currentHunk.OldCount++
			}

			if edit.Type == Context {
				nextChangeWithin := false
				end := i + contextLines + 1
				if end > len(edits) {
					end = len(edits)
				}
				for j := i + 1; j < end; j++ {
					if edits[j].Type != Context {
						nextChangeWithin = true
						break
					}
				}
				if !nextChangeWithin && i+contextLines < len(edits)-1 {
					hunks = append(hunks, *currentHunk)
					currentHunk = nil
				}
			}
		}

		switch edit.Type {
		case Context:
			oldLine++
			newLine++
		case Addition:
			newLine++
		case Deletion:
			oldLine++
		}
	}

	if currentHunk != nil {
		hunks = append(hunks, *currentHunk)
	}

	return hunks
}

func FormatLineDiff(ld *LineDiff) string {
	if ld == nil || len(ld.Hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, hunk := range ld.Hunks {
		sb.WriteString(fmt.Sprintf("\033[36m@@ -%d,%d +%d,%d @@\033[0m\n",
			hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount))

		for _, line := range hunk.Lines {
			switch line.Type {
			case Context:
				sb.WriteString("  " + line.Content + "\n")
			case Addition:
				sb.WriteString("\033[32m+ " + line.Content + "\033[0m\n")
			case Deletion:
				sb.WriteString("\033[31m- " + line.Content + "\033[0m\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func FormatLineDiffPlain(ld *LineDiff) string {
	if ld == nil || len(ld.Hunks) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, hunk := range ld.Hunks {
		sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n",
			hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount))

		for _, line := range hunk.Lines {
			switch line.Type {
			case Context:
				sb.WriteString("  " + line.Content + "\n")
			case Addition:
				sb.WriteString("+ " + line.Content + "\n")
			case Deletion:
				sb.WriteString("- " + line.Content + "\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func CountChanges(treeA, treeB map[string]string) (added, modified, deleted int) {
	for path, hashB := range treeB {
		hashA, exists := treeA[path]
		if !exists {
			added++
		} else if hashA != hashB {
			modified++
		}
	}

	for path := range treeA {
		if _, exists := treeB[path]; !exists {
			deleted++
		}
	}
	return
}
