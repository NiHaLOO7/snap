package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const hookCommand = `if [ -d .snap ]; then FILE=$(echo "$TOOL_INPUT" | python3 -c "import sys,json; print(json.load(sys.stdin).get('file_path',''))" 2>/dev/null); if [ -n "$FILE" ] && [ -f "$FILE" ]; then snap save-file "$FILE" -m "before agent edit" 2>/dev/null; fi; fi`

func setupClaudeHook() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("  ⚠ Hook setup encountered an error — skipping (snap init succeeded)\n")
		}
	}()

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("  ⚠ Could not determine home directory — skipping hook setup")
		return
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")

	var settings map[string]interface{}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		settings = make(map[string]interface{})
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			fmt.Println("  ⚠ Could not parse ~/.claude/settings.json — skipping hook setup")
			return
		}
	}

	if hooks, ok := settings["hooks"].(map[string]interface{}); ok {
		if preToolUse, ok := hooks["PreToolUse"].([]interface{}); ok {
			for _, entry := range preToolUse {
				if m, ok := entry.(map[string]interface{}); ok {
					if hooksArr, ok := m["hooks"].([]interface{}); ok {
						for _, h := range hooksArr {
							if hm, ok := h.(map[string]interface{}); ok {
								if cmd, ok := hm["command"].(string); ok && strings.Contains(cmd, "snap save-file") {
									fmt.Println("  Claude Code hook already configured ✓")
									return
								}
							}
						}
					}
				}
			}
		}
	}

	hook := map[string]interface{}{
		"matcher": "Edit|Write",
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": hookCommand,
				"timeout": 5,
				"async":   true,
			},
		},
	}

	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		hooks = make(map[string]interface{})
	}

	preToolUse, ok := hooks["PreToolUse"].([]interface{})
	if !ok {
		preToolUse = []interface{}{}
	}

	preToolUse = append(preToolUse, hook)
	hooks["PreToolUse"] = preToolUse
	settings["hooks"] = hooks

	os.MkdirAll(filepath.Join(home, ".claude"), 0755)

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		fmt.Println("  ⚠ Could not serialize settings — skipping hook setup")
		return
	}

	if err := os.WriteFile(settingsPath, output, 0644); err != nil {
		fmt.Println("  ⚠ Could not write ~/.claude/settings.json — skipping hook setup")
		return
	}

	fmt.Println("  Claude Code hook installed — auto-saves files before every agent edit ✓")
}
