package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/nihalkumar/snap/internal/ignore"
	"github.com/nihalkumar/snap/internal/store"
)

type Snapshot struct {
	ID          int               `json:"id"`
	Message     string            `json:"message"`
	Description string            `json:"description,omitempty"`
	Timestamp   time.Time         `json:"timestamp"`
	Tree        map[string]string `json:"tree"`
	FileCount   int               `json:"file_count"`
	AutoSave    bool              `json:"auto_save,omitempty"`
}

type Engine struct {
	rootPath     string
	snapPath     string
	store        *store.ObjectStore
	ignoreMatcher *ignore.Matcher
}

func NewEngine(rootPath string) *Engine {
	snapPath := filepath.Join(rootPath, ".snap")
	return &Engine{
		rootPath:     rootPath,
		snapPath:     snapPath,
		store:        store.New(snapPath),
		ignoreMatcher: ignore.NewMatcher(rootPath),
	}
}

func (e *Engine) Init() error {
	dirs := []string{
		e.snapPath,
		filepath.Join(e.snapPath, "snapshots"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	if err := e.store.Init(); err != nil {
		return fmt.Errorf("init object store: %w", err)
	}

	configPath := filepath.Join(e.snapPath, "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		config := map[string]string{"version": "1", "project": filepath.Base(rootPath(e.rootPath))}
		data, _ := json.MarshalIndent(config, "", "  ")
		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
	}

	return nil
}

func (e *Engine) IsInitialized() bool {
	_, err := os.Stat(e.snapPath)
	return err == nil
}

func (e *Engine) Save(message string, autoSave bool) (*Snapshot, error) {
	return e.SaveWithDescription(message, "", autoSave)
}

func (e *Engine) SaveWithDescription(message, description string, autoSave bool) (*Snapshot, error) {
	tree, err := e.buildTree()
	if err != nil {
		return nil, fmt.Errorf("build tree: %w", err)
	}

	nextID, err := e.nextID()
	if err != nil {
		return nil, fmt.Errorf("get next id: %w", err)
	}

	snap := &Snapshot{
		ID:          nextID,
		Message:     message,
		Description: description,
		Timestamp:   time.Now(),
		Tree:        tree,
		FileCount:   len(tree),
		AutoSave:    autoSave,
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal snapshot: %w", err)
	}

	filename := fmt.Sprintf("%04d.json", snap.ID)
	path := filepath.Join(e.snapPath, "snapshots", filename)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write snapshot: %w", err)
	}

	return snap, nil
}

func (e *Engine) Restore(id int) error {
	snap, err := e.Load(id)
	if err != nil {
		return fmt.Errorf("load snapshot %d: %w", id, err)
	}

	currentFiles, err := e.collectFiles()
	if err != nil {
		return fmt.Errorf("collect current files: %w", err)
	}

	for path, hash := range snap.Tree {
		data, err := e.store.Read(hash)
		if err != nil {
			return fmt.Errorf("read object for %s: %w", path, err)
		}

		fullPath := filepath.Join(e.rootPath, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create dir for %s: %w", path, err)
		}

		if err := os.WriteFile(fullPath, data, 0644); err != nil {
			return fmt.Errorf("write file %s: %w", path, err)
		}
	}

	for _, path := range currentFiles {
		if _, exists := snap.Tree[path]; !exists {
			fullPath := filepath.Join(e.rootPath, path)
			os.Remove(fullPath)
			e.cleanEmptyDirs(filepath.Dir(fullPath))
		}
	}

	return nil
}

func (e *Engine) Load(id int) (*Snapshot, error) {
	filename := fmt.Sprintf("%04d.json", id)
	path := filepath.Join(e.snapPath, "snapshots", filename)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("snapshot %d not found", id)
	}

	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parse snapshot %d: %w", id, err)
	}

	return &snap, nil
}

func (e *Engine) List() ([]*Snapshot, error) {
	dir := filepath.Join(e.snapPath, "snapshots")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var snapshots []*Snapshot
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var snap Snapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			continue
		}
		snapshots = append(snapshots, &snap)
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].ID < snapshots[j].ID
	})

	return snapshots, nil
}

func (e *Engine) GetCurrentTree() (map[string]string, error) {
	return e.buildTree()
}

func (e *Engine) buildTree() (map[string]string, error) {
	tree := make(map[string]string)

	err := filepath.Walk(e.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(e.rootPath, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		if e.ignoreMatcher.ShouldIgnore(relPath) {
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
			return fmt.Errorf("read file %s: %w", relPath, err)
		}

		hash, err := e.store.Write(data)
		if err != nil {
			return fmt.Errorf("store file %s: %w", relPath, err)
		}

		tree[relPath] = hash
		return nil
	})

	return tree, err
}

func (e *Engine) collectFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(e.rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(e.rootPath, path)
		if err != nil {
			return err
		}

		if relPath == "." {
			return nil
		}

		if e.ignoreMatcher.ShouldIgnore(relPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			files = append(files, relPath)
		}
		return nil
	})

	return files, err
}

func (e *Engine) nextID() (int, error) {
	snapshots, err := e.List()
	if err != nil {
		return 0, err
	}
	if len(snapshots) == 0 {
		return 1, nil
	}
	return snapshots[len(snapshots)-1].ID + 1, nil
}

func (e *Engine) cleanEmptyDirs(dir string) {
	for dir != e.rootPath {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			break
		}
		os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

func rootPath(path string) string {
	return filepath.Base(path)
}
