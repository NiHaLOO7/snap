package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

var defaultIgnores = []string{
	".snap",
	".git",
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
}

type Matcher struct {
	patterns []string
}

func NewMatcher(projectRoot string) *Matcher {
	m := &Matcher{
		patterns: append([]string{}, defaultIgnores...),
	}

	ignoreFile := filepath.Join(projectRoot, ".snapignore")
	if f, err := os.Open(ignoreFile); err == nil {
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

	return m
}

func (m *Matcher) ShouldIgnore(relPath string) bool {
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
