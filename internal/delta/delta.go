package delta

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type Change struct {
	Timestamp time.Time `json:"ts"`
	Path      string    `json:"path"`
	Action    string    `json:"action"` // "modify", "create", "delete"
	OldHash   string    `json:"old_hash,omitempty"`
	NewHash   string    `json:"new_hash,omitempty"`
	Delta     []byte    `json:"delta,omitempty"`
	FullSize  int       `json:"full_size"`
	IsAgent   bool      `json:"is_agent,omitempty"`
}

type Timeline struct {
	mu       sync.RWMutex
	basePath string
	changes  []*Change
	current  string // current segment file
}

func NewTimeline(snapPath string) *Timeline {
	return &Timeline{
		basePath: filepath.Join(snapPath, "timeline"),
	}
}

func (t *Timeline) Init() error {
	return os.MkdirAll(t.basePath, 0755)
}

func (t *Timeline) Record(change *Change) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.changes = append(t.changes, change)

	if len(t.changes) >= 100 {
		return t.flush()
	}
	return nil
}

func (t *Timeline) Flush() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.flush()
}

func (t *Timeline) flush() error {
	if len(t.changes) == 0 {
		return nil
	}

	segmentName := fmt.Sprintf("%d.seg", time.Now().UnixMilli())
	segPath := filepath.Join(t.basePath, segmentName)

	data, err := json.Marshal(t.changes)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	w.Write(data)
	w.Close()

	if err := os.WriteFile(segPath, buf.Bytes(), 0644); err != nil {
		return err
	}

	t.changes = t.changes[:0]
	return nil
}

func (t *Timeline) LoadAll() ([]*Change, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	entries, err := os.ReadDir(t.basePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var allChanges []*Change

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".seg" {
			continue
		}

		changes, err := t.loadSegment(filepath.Join(t.basePath, entry.Name()))
		if err != nil {
			continue
		}
		allChanges = append(allChanges, changes...)
	}

	// Add in-memory changes
	allChanges = append(allChanges, t.changes...)

	sort.Slice(allChanges, func(i, j int) bool {
		return allChanges[i].Timestamp.Before(allChanges[j].Timestamp)
	})

	return allChanges, nil
}

func (t *Timeline) loadSegment(path string) ([]*Change, error) {
	compressed, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var changes []*Change
	if err := json.Unmarshal(data, &changes); err != nil {
		return nil, err
	}

	return changes, nil
}

func (t *Timeline) GetStateAt(target time.Time) map[string]string {
	changes, _ := t.LoadAll()

	state := make(map[string]string)
	for _, c := range changes {
		if c.Timestamp.After(target) {
			break
		}
		switch c.Action {
		case "create", "modify":
			state[c.Path] = c.NewHash
		case "delete":
			delete(state, c.Path)
		}
	}

	return state
}

func (t *Timeline) FindWhen(testCmd string, changes []*Change) *time.Time {
	// Find last timestamp where state changed
	// Caller runs testCmd against states to find break point
	// Returns timestamp of breaking change
	for i := len(changes) - 1; i >= 0; i-- {
		return &changes[i].Timestamp
	}
	return nil
}

func (t *Timeline) GetChangesBetween(from, to time.Time) []*Change {
	all, _ := t.LoadAll()
	var result []*Change
	for _, c := range all {
		if c.Timestamp.After(from) && !c.Timestamp.After(to) {
			result = append(result, c)
		}
	}
	return result
}

func (t *Timeline) DetectAgentBurst(changes []*Change) {
	window := 3 * time.Second
	for i := 0; i < len(changes); i++ {
		count := 1
		for j := i + 1; j < len(changes); j++ {
			if changes[j].Timestamp.Sub(changes[i].Timestamp) <= window {
				count++
			} else {
				break
			}
		}
		if count >= 5 {
			for j := i; j < i+count && j < len(changes); j++ {
				changes[j].IsAgent = true
			}
		}
	}
}

func (t *Timeline) DiskUsage() (int64, error) {
	var total int64
	entries, err := os.ReadDir(t.basePath)
	if err != nil {
		return 0, err
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}
	return total, nil
}

func (t *Timeline) Compact(retention time.Duration) error {
	entries, err := os.ReadDir(t.basePath)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-retention)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".seg" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(t.basePath, entry.Name()))
		}
	}

	return nil
}
