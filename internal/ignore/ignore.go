package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var defaultIgnores = []string{
	".snap",
	".git",
	".claude",
	".github",
	"node_modules",
	"vendor",
	"__pycache__",
	".DS_Store",
	"*.exe",
	"*.dll",
	"*.so",
	"*.dylib",
	"dist",
	"build",
	".env",
	"tmp",
	".cursorrules",
	".cursor",
	"CLAUDE.md",
}

type Matcher struct {
	mu          sync.RWMutex
	patterns    []string
	projectRoot string
	lastLoad    time.Time
	lastMod     time.Time
}

func NewMatcher(projectRoot string) *Matcher {
	m := &Matcher{
		projectRoot: projectRoot,
	}
	m.reload()
	return m
}

func (m *Matcher) reload() {
	m.patterns = append([]string{}, defaultIgnores...)

	ignoreFile := filepath.Join(m.projectRoot, ".snapignore")
	info, err := os.Stat(ignoreFile)
	if err != nil {
		m.lastLoad = time.Now()
		return
	}

	m.lastMod = info.ModTime()
	m.lastLoad = time.Now()

	f, err := os.Open(ignoreFile)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m.patterns = append(m.patterns, line)
	}
}

func (m *Matcher) checkReload() {
	if time.Since(m.lastLoad) < 2*time.Second {
		return
	}

	ignoreFile := filepath.Join(m.projectRoot, ".snapignore")
	info, err := os.Stat(ignoreFile)
	if err != nil {
		return
	}

	if info.ModTime().After(m.lastMod) {
		m.mu.Lock()
		m.reload()
		m.mu.Unlock()
	} else {
		m.lastLoad = time.Now()
	}
}

func (m *Matcher) ShouldIgnore(relPath string) bool {
	m.checkReload()

	m.mu.RLock()
	defer m.mu.RUnlock()

	parts := strings.Split(relPath, string(os.PathSeparator))

	for _, pattern := range m.patterns {
		for _, part := range parts {
			if matched, _ := filepath.Match(pattern, part); matched {
				return true
			}
		}

		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
	}

	return false
}
