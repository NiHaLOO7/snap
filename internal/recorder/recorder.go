package recorder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nihalkumar/snap/internal/delta"
	"github.com/nihalkumar/snap/internal/ignore"
	"github.com/nihalkumar/snap/internal/snapshot"
	"github.com/nihalkumar/snap/internal/store"
)

type Config struct {
	MaxStorage    int64         `json:"max_storage_mb"`
	Retention     time.Duration `json:"retention"`
	MinInterval   time.Duration `json:"min_interval"`
}

func DefaultConfig() Config {
	return Config{
		MaxStorage:  100,
		Retention:   7 * 24 * time.Hour,
		MinInterval: 2 * time.Second,
	}
}

type Recorder struct {
	rootPath  string
	snapPath  string
	store     *store.ObjectStore
	timeline  *delta.Timeline
	ignore    *ignore.Matcher
	config    Config
	watcher   *fsnotify.Watcher
	mu        sync.Mutex
	lastEvent map[string]time.Time
	fileState map[string]string // path -> last known hash
	watchlist map[string]bool
	running   bool
	stopCh    chan struct{}
}

func New(rootPath string, config Config) *Recorder {
	snapPath := filepath.Join(rootPath, ".snap")
	r := &Recorder{
		rootPath:  rootPath,
		snapPath:  snapPath,
		store:     store.New(snapPath),
		timeline:  delta.NewTimeline(snapPath),
		ignore:    ignore.NewMatcher(rootPath),
		config:    config,
		lastEvent: make(map[string]time.Time),
		fileState: make(map[string]string),
		watchlist: make(map[string]bool),
		stopCh:    make(chan struct{}),
	}
	r.loadWatchlist()
	return r
}

func (r *Recorder) loadWatchlist() {
	watchFile := filepath.Join(r.snapPath, "watchlist.json")
	data, err := os.ReadFile(watchFile)
	if err != nil {
		return
	}
	var files []string
	if err := json.Unmarshal(data, &files); err != nil {
		return
	}
	for _, f := range files {
		r.watchlist[f] = true
	}
}

func (r *Recorder) Start() error {
	if err := r.timeline.Init(); err != nil {
		return fmt.Errorf("init timeline: %w", err)
	}

	if err := r.buildInitialState(); err != nil {
		return fmt.Errorf("build initial state: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	r.watcher = watcher

	if err := r.addWatchDirs(); err != nil {
		watcher.Close()
		return fmt.Errorf("add watch dirs: %w", err)
	}

	r.running = true

	go r.eventLoop()
	go r.flushLoop()
	go r.compactionLoop()

	// Write PID file
	pidPath := filepath.Join(r.snapPath, "recorder.pid")
	os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

	return nil
}

func (r *Recorder) Stop() error {
	if !r.running {
		return nil
	}

	close(r.stopCh)
	r.running = false

	if r.watcher != nil {
		r.watcher.Close()
	}

	r.timeline.Flush()

	pidPath := filepath.Join(r.snapPath, "recorder.pid")
	os.Remove(pidPath)

	return nil
}

func (r *Recorder) IsRunning() bool {
	pidPath := filepath.Join(r.snapPath, "recorder.pid")
	_, err := os.Stat(pidPath)
	return err == nil
}

func (r *Recorder) buildInitialState() error {
	now := time.Now()
	hasExistingTimeline := false

	entries, _ := os.ReadDir(filepath.Join(r.snapPath, "timeline"))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".seg" {
			hasExistingTimeline = true
			break
		}
	}

	return filepath.Walk(r.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(r.rootPath, path)
		if relPath == "." {
			return nil
		}

		if r.ignore.ShouldIgnore(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		hash, _ := r.store.Write(data)
		r.fileState[relPath] = hash

		// Record initial state if no existing timeline
		if !hasExistingTimeline {
			r.timeline.Record(&delta.Change{
				Timestamp: now,
				Path:      relPath,
				Action:    "create",
				NewHash:   hash,
				FullSize:  len(data),
			})
		}

		return nil
	})
}

func (r *Recorder) addWatchDirs() error {
	return filepath.Walk(r.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		relPath, _ := filepath.Rel(r.rootPath, path)
		if relPath == "." {
			r.watcher.Add(path)
			return nil
		}

		if r.ignore.ShouldIgnore(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			r.watcher.Add(path)
		}

		return nil
	})
}

func (r *Recorder) eventLoop() {
	for {
		select {
		case <-r.stopCh:
			return
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			r.handleEvent(event)
		case _, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (r *Recorder) handleEvent(event fsnotify.Event) {
	relPath, err := filepath.Rel(r.rootPath, event.Name)
	if err != nil {
		return
	}

	if r.ignore.ShouldIgnore(relPath) {
		return
	}

	if strings.HasPrefix(relPath, ".snap") {
		return
	}

	// Skip temp files created by editors (VS Code, vim, etc.)
	if strings.Contains(relPath, ".tmp.") || strings.HasSuffix(relPath, "~") || strings.HasSuffix(relPath, ".swp") {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()

	// For delete/rename events, wait briefly — editors do delete+create for atomic saves
	if event.Op&fsnotify.Remove != 0 || event.Op&fsnotify.Rename != 0 {
		r.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		r.mu.Lock()

		// Check if file came back (atomic save pattern)
		if _, err := os.Stat(event.Name); err == nil {
			// File is back — treat as modify
			data, err := os.ReadFile(event.Name)
			if err != nil {
				return
			}
			newHash, err := r.store.Write(data)
			if err != nil {
				return
			}
			oldHash := r.fileState[relPath]
			if oldHash == newHash {
				return
			}
			r.lastEvent[relPath] = now
			r.fileState[relPath] = newHash
			r.timeline.Record(&delta.Change{
				Timestamp: now,
				Path:      relPath,
				Action:    "modify",
				OldHash:   oldHash,
				NewHash:   newHash,
				FullSize:  len(data),
			})
			return
		}

		// File truly deleted
		oldHash := r.fileState[relPath]
		if oldHash == "" {
			return
		}
		r.lastEvent[relPath] = now
		delete(r.fileState, relPath)
		r.timeline.Record(&delta.Change{
			Timestamp: now,
			Path:      relPath,
			Action:    "delete",
			OldHash:   oldHash,
		})
		return
	}

	if event.Op&fsnotify.Write != 0 || event.Op&fsnotify.Create != 0 {
		if last, ok := r.lastEvent[relPath]; ok {
			if now.Sub(last) < r.config.MinInterval {
				return
			}
		}

		info, err := os.Stat(event.Name)
		if err != nil {
			return
		}

		if info.IsDir() {
			r.watcher.Add(event.Name)
			return
		}

		data, err := os.ReadFile(event.Name)
		if err != nil {
			return
		}

		newHash, err := r.store.Write(data)
		if err != nil {
			return
		}

		oldHash, existed := r.fileState[relPath]
		if existed && oldHash == newHash {
			return
		}

		action := "modify"
		if !existed {
			action = "create"
		}

		r.lastEvent[relPath] = now
		r.fileState[relPath] = newHash
		r.timeline.Record(&delta.Change{
			Timestamp: now,
			Path:      relPath,
			Action:    action,
			OldHash:   oldHash,
			NewHash:   newHash,
			FullSize:  len(data),
		})

		// Auto-checkpoint watched files
		if r.watchlist[relPath] {
			engine := snapshot.NewEngine(r.rootPath)
			tree := map[string]string{relPath: newHash}
			engine.SaveSingleFileWithAutoSave(relPath, fmt.Sprintf("watch: %s changed", relPath), tree, true)
		}
	}
}

func (r *Recorder) flushLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.timeline.Flush()
		}
	}
}

func (r *Recorder) compactionLoop() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.timeline.Compact(r.config.Retention)

			usage, _ := r.timeline.DiskUsage()
			maxBytes := r.config.MaxStorage * 1024 * 1024
			if usage > maxBytes {
				r.timeline.Compact(r.config.Retention / 2)
			}

			// Auto-GC snapshots
			r.autoGC()
		}
	}
}

func (r *Recorder) autoGC() {
	engine := snapshot.NewEngine(r.rootPath)
	snapshots, err := engine.List()
	if err != nil || len(snapshots) < 20 {
		return
	}

	now := time.Now()
	retention := r.config.Retention
	snapshotsDir := filepath.Join(r.snapPath, "snapshots")

	// Build tree fingerprints for duplicate detection
	seenTrees := make(map[string]bool)
	var removed int

	for _, snap := range snapshots {
		if snap.Pinned {
			continue
		}

		// Remove duplicate trees
		treeKey := buildSnapshotTreeKey(snap.Tree)
		if seenTrees[treeKey] {
			filename := fmt.Sprintf("%04d.json", snap.ID)
			os.Remove(filepath.Join(snapshotsDir, filename))
			removed++
			continue
		}
		seenTrees[treeKey] = true

		// Remove old auto-saves beyond retention
		if snap.AutoSave && now.Sub(snap.Timestamp) > retention {
			filename := fmt.Sprintf("%04d.json", snap.ID)
			os.Remove(filepath.Join(snapshotsDir, filename))
			removed++
			continue
		}

		// Remove superseded auto-saves (newer auto-save within 5 min)
		if snap.AutoSave {
			for _, other := range snapshots {
				if other.ID > snap.ID && other.AutoSave && other.Timestamp.Sub(snap.Timestamp) < 5*time.Minute {
					filename := fmt.Sprintf("%04d.json", snap.ID)
					os.Remove(filepath.Join(snapshotsDir, filename))
					removed++
					break
				}
			}
		}
	}

	// Clean orphaned objects if we removed snapshots
	if removed > 0 {
		r.cleanOrphanedObjects()
	}
}

func (r *Recorder) cleanOrphanedObjects() {
	engine := snapshot.NewEngine(r.rootPath)
	snapshots, _ := engine.List()

	referenced := make(map[string]bool)
	for _, snap := range snapshots {
		for _, hash := range snap.Tree {
			referenced[hash] = true
		}
	}

	objectsDir := filepath.Join(r.snapPath, "objects")
	filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(objectsDir, path)
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) == 2 {
			hash := parts[0] + parts[1]
			if !referenced[hash] {
				os.Remove(path)
			}
		}
		return nil
	})
}

func buildSnapshotTreeKey(tree map[string]string) string {
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

func (r *Recorder) GetTimeline() *delta.Timeline {
	return r.timeline
}

func (r *Recorder) GetStore() *store.ObjectStore {
	return r.store
}
