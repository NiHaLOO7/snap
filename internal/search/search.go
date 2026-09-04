package search

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nihalkumar/snap/internal/store"
)

type FileMatch struct {
	SnapshotID int    `json:"snapshot_id"`
	Path       string `json:"path"`
	Score      int    `json:"score"`
}

type ContentMatch struct {
	SnapshotID int    `json:"snapshot_id"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Content    string `json:"content"`
}

type GrepOptions struct {
	UseRegex       bool
	CaseInsensitive bool
}

type snapshotMeta struct {
	ID   int               `json:"id"`
	Tree map[string]string `json:"tree"`
}

func loadSnapshots(snapPath string) ([]snapshotMeta, error) {
	dir := filepath.Join(snapPath, "snapshots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var snapshots []snapshotMeta
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s snapshotMeta
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		snapshots = append(snapshots, s)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ID < snapshots[j].ID
	})

	return snapshots, nil
}

func SearchFiles(snapPath, query string) ([]FileMatch, error) {
	snapshots, err := loadSnapshots(snapPath)
	if err != nil {
		return nil, fmt.Errorf("load snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, nil
	}

	var results []FileMatch
	queryLower := strings.ToLower(query)
	for _, s := range snapshots {
		for p := range s.Tree {
			score := substringScore(queryLower, strings.ToLower(p))
			if score > 0 {
				results = append(results, FileMatch{
					SnapshotID: s.ID,
					Path:       p,
					Score:      score,
				})
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	return results, nil
}

func GrepContent(snapPath string, pattern string, opts GrepOptions) ([]ContentMatch, error) {
	snapshots, err := loadSnapshots(snapPath)
	if err != nil {
		return nil, fmt.Errorf("load snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return nil, nil
	}

	objStore := store.New(snapPath)

	// Dedupe by (path, hash) — each unique file version appears once with its latest checkpoint
	type fileEntry struct {
		snapID int
		hash   string
		path   string
	}
	type versionKey struct{ path, hash string }

	versionMap := make(map[versionKey]fileEntry)
	for _, s := range snapshots {
		for p, h := range s.Tree {
			versionMap[versionKey{p, h}] = fileEntry{snapID: s.ID, hash: h, path: p}
		}
	}

	// Group by hash — same content only scanned once
	hashToFiles := make(map[string][]fileEntry)
	for _, fe := range versionMap {
		hashToFiles[fe.hash] = append(hashToFiles[fe.hash], fe)
	}

	var matcher func(line string) bool
	if opts.UseRegex {
		flags := ""
		if opts.CaseInsensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex: %w", err)
		}
		matcher = func(line string) bool {
			return re.MatchString(line)
		}
	} else {
		if opts.CaseInsensitive {
			patLower := strings.ToLower(pattern)
			matcher = func(line string) bool {
				return strings.Contains(strings.ToLower(line), patLower)
			}
		} else {
			matcher = func(line string) bool {
				return strings.Contains(line, pattern)
			}
		}
	}

	var results []ContentMatch

	binaryExts := map[string]bool{
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".ico": true, ".webp": true, ".svg": true,
		".mp3": true, ".mp4": true, ".wav": true, ".ogg": true, ".webm": true, ".avi": true, ".mov": true, ".flac": true,
		".zip": true, ".gz": true, ".tar": true, ".bz2": true, ".7z": true, ".rar": true, ".xz": true,
		".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true, ".ppt": true, ".pptx": true,
		".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true, ".dat": true,
		".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
		".pyc": true, ".pyo": true, ".class": true, ".o": true, ".obj": true, ".a": true, ".lib": true,
		".vsix": true, ".snap": true, ".db": true, ".sqlite": true, ".sqlite3": true,
	}

	for hash, files := range hashToFiles {
		ext := strings.ToLower(filepath.Ext(files[0].path))
		if binaryExts[ext] {
			continue
		}
		if isBinaryHash(hash, objStore) {
			continue
		}

		data, err := objStore.Read(hash)
		if err != nil {
			continue
		}

		var matchingLines []struct {
			lineNum int
			content string
		}

		scanner := bufio.NewScanner(bytes.NewReader(data))
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if matcher(line) {
				trimmed := line
				if len(trimmed) > 200 {
					trimmed = trimmed[:200] + "..."
				}
				matchingLines = append(matchingLines, struct {
					lineNum int
					content string
				}{lineNum, trimmed})
			}
		}

		if len(matchingLines) > 0 {
			for _, fe := range files {
				for _, ml := range matchingLines {
					results = append(results, ContentMatch{
						SnapshotID: fe.snapID,
						Path:       fe.path,
						Line:       ml.lineNum,
						Content:    ml.content,
					})
				}
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		return results[i].Line < results[j].Line
	})

	return results, nil
}

func substringScore(queryLower, targetLower string) int {
	idx := strings.Index(targetLower, queryLower)
	if idx == -1 {
		return 0
	}

	score := 100
	lastSlash := strings.LastIndex(targetLower, "/")
	filename := targetLower
	if lastSlash >= 0 {
		filename = targetLower[lastSlash+1:]
	}

	if filename == queryLower {
		score += 50
	} else if strings.HasPrefix(filename, queryLower) {
		score += 30
	} else if idx > lastSlash {
		score += 20
	}

	if idx == 0 || (idx > 0 && strings.ContainsRune("/\\._- ", rune(targetLower[idx-1]))) {
		score += 10
	}

	score -= idx
	return score
}

func isBinaryHash(hash string, objStore *store.ObjectStore) bool {
	data, err := objStore.Read(hash)
	if err != nil {
		return true
	}

	checkSize := 512
	if len(data) < checkSize {
		checkSize = len(data)
	}

	nullCount := 0
	for i := 0; i < checkSize; i++ {
		if data[i] == 0 {
			nullCount++
		}
	}

	return nullCount > checkSize/8
}
