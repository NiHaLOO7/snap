package recorder

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nihalkumar/snap/internal/delta"
	"github.com/nihalkumar/snap/internal/ignore"
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
	running   bool
	stopCh    chan struct{}
}

func New(rootPath string, config Config) *Recorder {
	snapPath := filepath.Join(rootPath, ".snap")
	return &Recorder{
		rootPath:  rootPath,
		snapPath:  snapPath,
		store:     store.New(snapPath),
		timeline:  delta.NewTimeline(snapPath),
		ignore:    ignore.NewMatcher(rootPath),
		config:    config,
		lastEvent: make(map[string]time.Time),
		fileState: make(map[string]string),
		stopCh:    make(chan struct{}),
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

		hash := store.Hash(data)
		r.fileState[relPath] = hash
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
				// Aggressively compact if over limit
				r.timeline.Compact(r.config.Retention / 2)
			}
		}
	}
}

func (r *Recorder) GetTimeline() *delta.Timeline {
	return r.timeline
}

func (r *Recorder) GetStore() *store.ObjectStore {
	return r.store
}
