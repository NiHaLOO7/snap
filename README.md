# Snap — Local Development Checkpoints

A fast, local checkpoint tool for developers. Save your project state instantly, diff between any two points, and restore safely — without needing Git commits.

Built for the AI-assisted development workflow where you make experimental changes, hand control to agents, and need a safety net that's faster than `git commit`.

```
┌─────────────────────────────────────────────────┐
│              Your Development Flow               │
│                                                  │
│   Working State                                  │
│        │                                         │
│        ▼                                         │
│   snap save "before refactor"   ← 2ms           │
│        │                                         │
│        ▼                                         │
│   Agent makes changes...                         │
│        │                                         │
│        ▼                                         │
│   snap save "after agent"                        │
│        │                                         │
│        ▼                                         │
│   Something broke?                               │
│        │                                         │
│        ▼                                         │
│   snap restore 3                ← instant        │
│   (auto-saves current state)                     │
│        │                                         │
│        ▼                                         │
│   Back to safety. Zero work lost.                │
└─────────────────────────────────────────────────┘
```

---

## Table of Contents

- [Why Snap?](#why-snap)
- [Installation](#installation)
  - [CLI Installation](#cli-installation)
  - [VS Code Extension](#vs-code-extension-installation)
- [Quick Start](#quick-start)
- [CLI Commands](#cli-commands)
- [VS Code Extension](#vs-code-extension)
- [Storage Design](#storage-design)
- [.snapignore](#snapignore)
- [Build from Source](#build-from-source)
- [Architecture](#architecture)

---

## Why Snap?

Git is great for publishing code. But between commits, your code is unprotected.

| Problem | Snap Solution |
|---------|---------------|
| Agent changes 20 files, something breaks | `snap restore` — back to safety in 1 second |
| Agent "reverts" but misses 3 files | Every save is a complete project snapshot |
| Experimenting and want to compare states | `snap diff 3 7` — any two points |
| Don't want to pollute Git history | Snap is completely separate from Git |
| Lost track of what changed | `snap status` — instant overview |
| Don't know which checkpoint to restore | `snap show 3 src/main.go` — view file at any point |

---

## Installation

### CLI Installation

Download the binary for your platform from the [Releases](https://github.com/NiHaLOO7/snap/releases) page.

**macOS (Apple Silicon — M1/M2/M3/M4):**

```bash
# Download
curl -L https://github.com/NiHaLOO7/snap/releases/download/v1.0.0/snap-darwin-arm64 -o snap

# Make executable
chmod +x snap

# Move to PATH
sudo mv snap /usr/local/bin/snap

# Verify
snap version
```

**macOS (Intel):**

```bash
curl -L https://github.com/NiHaLOO7/snap/releases/download/v1.0.0/snap-darwin-amd64 -o snap
chmod +x snap
sudo mv snap /usr/local/bin/snap
```

**Linux:**

```bash
curl -L https://github.com/NiHaLOO7/snap/releases/download/v1.0.0/snap-linux-amd64 -o snap
chmod +x snap
sudo mv snap /usr/local/bin/snap
```

**Windows:**

1. Download `snap-windows-amd64.exe` from the release page
2. Rename to `snap.exe`
3. Move to a folder in your PATH (e.g., `C:\Users\YourName\bin\`)
4. Add that folder to your system PATH if not already there

### VS Code Extension Installation

Download `snap-checkpoints-1.1.0.vsix` from the [Releases](https://github.com/NiHaLOO7/snap/releases) page.

**Method 1: Command Line**

```bash
code --install-extension snap-checkpoints-1.1.0.vsix
```

**Method 2: VS Code UI**

1. Open VS Code
2. Go to Extensions panel (Ctrl+Shift+X / Cmd+Shift+X)
3. Click the `...` menu (top-right of Extensions panel)
4. Select **"Install from VSIX..."**
5. Browse to the downloaded `.vsix` file
6. Click Install
7. Reload VS Code

> **Important:** The VS Code extension requires the `snap` CLI to be installed and available in your system PATH. Install the CLI first.

---

## Quick Start

```bash
# 1. Go to your project
cd your-project

# 2. Initialize snap
snap init

# 3. Save your first checkpoint
snap save "initial working state"

# 4. Work, make changes, let agents run...

# 5. Save another checkpoint with optional description
snap save "auth complete" -d "JWT working, refresh token pending"

# 6. Check what changed since last save
snap status

# 7. Something broke? See the diff
snap diff 1 2 -f

# 8. Restore to any checkpoint (current state auto-saved)
snap restore 1

# 9. View a file at any checkpoint
snap show 2 src/main.go
```

---

## CLI Commands

### `snap init`

Initialize snap in the current directory. Creates a `.snap/` folder.

```
$ snap init
Initialized snap in /Users/you/project/.snap/

Ready to save snapshots. Run:
  snap save "initial state"
```

---

### `snap save [message] [-d "description"]`

Take a snapshot of the entire project state. Description is optional.

```
$ snap save "before oauth implementation"
Saved snapshot #4
  Message:  before oauth implementation
  Files:    47
  Time:     12ms

$ snap save "auth done" -d "JWT tokens working, refresh token still pending"
Saved snapshot #5
  Message:  auth done
  Desc:     JWT tokens working, refresh token still pending
  Files:    48
  Time:     8ms
```

**How it works internally:**
- Walks all project files (respecting `.snapignore`)
- Hashes each file with SHA-256
- Only stores files that changed (content-addressed deduplication)
- Compresses with zlib
- Saves metadata (timestamp, message, description, file tree)

---

### `snap list` / `snap ls`

Show all checkpoints organized in two categories.

```
$ snap list

📌 Checkpoints (3)

  ● #1     Aug 11 10:02  initial state  (42 files)
  │
  ● #3     Aug 11 11:15  after auth refactor  (47 files)
  │
  ◉ #5     Aug 11 11:45  working oauth  (48 files)
  │         JWT tokens working, refresh token still pending

🔄 Auto-saves (2)

  ○ #4     Aug 11 11:20  auto-save before restore to #2 "before auth"  (47 files)
  │
  ◎ #6     Aug 11 12:01  auto-save before restore to #1 "initial state"  (48 files)

  Total: 3 checkpoints, 2 auto-saves
```

User checkpoints and auto-saves are separated so you can easily identify your intentional save points.

---

### `snap show <id> [file]`

View what's inside a checkpoint — list files or view specific file content.

```
# List all files in a snapshot
$ snap show 3
Snapshot #3 "after auth refactor" (Aug 11 11:15)
Files (47):

  README.md
  src/auth/jwt.go
  src/auth/middleware.go
  src/main.go
  ...

# View a specific file's content at that snapshot
$ snap show 3 src/auth/jwt.go
── src/auth/jwt.go (snapshot #3) ──

package auth

import (
    "crypto/sha256"
    ...
)
```

This helps you decide which checkpoint to restore — peek at the code before jumping back.

---

### `snap restore <id>`

Jump back to any previous state. **Always auto-saves current state first** — you can never lose work.

```
$ snap restore 2
Auto-saving current state before restore...
  Saved as #6

Restoring to #2 "before auth refactor"...
Restored successfully. (42 files)

Your previous state is saved as #6 if you need it back.
```

**Safety guarantee:** Before restoring, your current state is automatically saved. You can always get it back.

---

### `snap diff`

Compare any two states. Multiple modes available.

```
# Snapshot vs current working directory
$ snap diff 3
Snapshot #3 "after auth"  →  Current working directory

  + src/new_file.go  (added)
  ~ src/auth.go  (modified)
  - src/old.go  (deleted)

  1 modified, 1 added, 1 deleted

# Between two snapshots
$ snap diff 1 5

# Full line-level diff (colored output)
$ snap diff 1 5 -f
Snapshot #1 "initial"  →  Snapshot #5 "oauth working"

  ~ src/main.go  (modified)
    @@ -3,6 +3,8 @@
      import "fmt"

      func main() {
    -     fmt.Println("Hello")
    +     fmt.Println("Hello World")
    +     startAuth()
      }

# Diff a specific file only
$ snap diff 1 2 src/main.go
── src/main.go ──
Snapshot #1 "initial"  →  Snapshot #2 "after changes"

@@ -3,6 +3,6 @@
  import "fmt"

  func main() {
-     fmt.Println("Hello")
+     fmt.Println("Hello World")
  }
```

---

### `snap status`

Show what changed since the last snapshot.

```
$ snap status
Last snapshot: #5 "working oauth" (Aug 11 11:45)

  Changes since last snapshot:

    ~ src/auth.go
    + src/middleware.go
    - src/old_handler.go

  1 modified, 1 added, 1 deleted
```

---

### `snap delete <id>` / `snap rm <id>`

Delete a checkpoint you no longer need.

```
$ snap delete 4
Deleted snapshot #4 "auto-save before restore to #2"
```

---

## VS Code Extension

The extension provides a visual interface for all snap operations directly in VS Code.

### How it Works

Once installed, a **bookmark icon** appears in the activity bar (left sidebar). Click it to open the Snap panel with two sections:

**1. Timeline Panel**

```
SNAP CHECKPOINTS
│
├── 📌 Checkpoints
│   ├── ◉ #5 — working oauth          Aug 11 11:45 • 48 files
│   │   ├── src/main.go               ← click to view content
│   │   ├── src/auth/jwt.go               click diff icon for comparison
│   │   └── src/auth/middleware.go
│   │
│   ├── ● #3 — after auth refactor    Aug 11 11:15 • 47 files
│   │   └── ...
│   │
│   └── ● #1 — initial state          Aug 11 10:02 • 42 files
│       └── ...
│
└── 🔄 Auto-saves
    ├── ◎ #6 — auto-save before restore to #1 "initial state"
    └── ○ #4 — auto-save before restore to #2 "before auth"
```

**2. Changes Panel**

Shows files changed since last snapshot (auto-refreshes as you edit):

```
CHANGES SINCE LAST SNAPSHOT
  ~ src/auth.go         modified
  + src/new_file.go     added
  - src/removed.go      deleted
```

### Extension Features

| Action | How |
|--------|-----|
| **Save checkpoint** | Click save icon (top of Timeline panel) → type message → optional description |
| **View file at checkpoint** | Expand checkpoint → click any file |
| **Diff file vs current** | Expand checkpoint → click diff icon on a file |
| **Restore checkpoint** | Right-click checkpoint → Restore |
| **Delete checkpoint** | Click trash icon on checkpoint |
| **Refresh** | Click refresh icon (top of Timeline panel) |

### Diff View

When you click the diff icon on a file, VS Code opens its **native diff editor**:

- Side-by-side comparison (snapshot version ↔ current version)
- **Word-level highlighting** — exact characters that changed are highlighted (just like Git in VS Code)
- Full syntax highlighting for both sides
- Inline navigation between changes

This gives you the same experience as VS Code's built-in Git diff.

### Extension Requirements

- `snap` CLI must be installed and available in system PATH
- Project must have `.snap/` directory (run `snap init` first)
- Extension auto-activates when it detects `.snap` folder in workspace

---

## Storage Design

Snap uses a **content-addressed object store** — the same fundamental approach as Git:

```
.snap/
├── objects/              # Content-addressed blob store
│   ├── a3/
│   │   └── 4f8b2c...    # zlib-compressed file content
│   ├── 7c/
│   │   └── 2e91f0...
│   └── ...
├── snapshots/            # Snapshot metadata (JSON)
│   ├── 0001.json
│   ├── 0002.json
│   └── ...
└── config.json
```

**How deduplication works:**

```
Snapshot #1: { "src/main.go": "abc123", "src/utils.go": "def456" }
Snapshot #2: { "src/main.go": "xyz789", "src/utils.go": "def456" }
                                ↑ new blob              ↑ same hash — NOT stored again
```

If 200 files exist and only 5 changed since last save, only 5 new blobs are stored. The other 195 paths point to existing objects.

**Key properties:**
- **SHA-256 hashing** — content integrity guaranteed
- **Zlib compression** — all blobs compressed on disk
- **Deduplication** — unchanged files cost zero additional storage
- **Independence** — every snapshot is self-contained, can be deleted without breaking others

---

## .snapignore

Create a `.snapignore` file in your project root to exclude files/directories:

```
# Dependencies
node_modules
vendor
.venv

# Build output
dist
build
*.exe

# IDE
.idea

# Secrets
.env
*.pem
```

**Default ignores** (always excluded even without `.snapignore`):
- `.snap`, `.git`
- `node_modules`, `vendor`, `__pycache__`
- `.DS_Store`
- `*.exe`, `*.dll`, `*.so`, `*.dylib`
- `dist`, `build`, `.env`, `tmp`

---

## Build from Source

Requires Go 1.21+.

```bash
# Clone
git clone https://github.com/NiHaLOO7/snap.git
cd snap

# Build for your platform
go build -o snap ./cmd/snap/
sudo mv snap /usr/local/bin/

# Or build all platforms
make build-all
# Binaries will be in dist/
```

**Build the VS Code extension:**

```bash
cd vscode-extension
npm install
npx tsc -p ./
npx @vscode/vsce package --allow-missing-repository
# Produces snap-checkpoints-x.x.x.vsix
```

---

## Architecture

```
snap/
├── cmd/snap/main.go              # CLI entry point, all commands
├── internal/
│   ├── store/store.go            # Content-addressed object store (SHA-256 + zlib)
│   ├── snapshot/snapshot.go      # Save/restore/list engine
│   ├── diff/diff.go              # Tree comparison + LCS line diff
│   └── ignore/ignore.go          # .snapignore pattern matching
├── vscode-extension/
│   ├── src/extension.ts          # Extension activation, commands
│   ├── src/snapProvider.ts       # Timeline tree view (categories + files)
│   ├── src/changesProvider.ts    # Changed files panel
│   └── src/snapCli.ts            # CLI wrapper for extension
├── Makefile                      # Cross-platform build targets
└── README.md
```

**Tech Stack:**
- **CLI:** Go (zero external dependencies — stdlib only)
- **Hashing:** SHA-256 (`crypto/sha256`)
- **Compression:** zlib (`compress/zlib`)
- **Diff:** LCS-based algorithm
- **Extension:** TypeScript, VS Code API
- **Storage:** File-based, content-addressed

---

## Roadmap

- [x] Content-addressed object store
- [x] Snapshot save with deduplication
- [x] Snapshot restore with auto-save safety
- [x] Categorized listing (Checkpoints vs Auto-saves)
- [x] Optional descriptions on checkpoints
- [x] Tree-level diff (file changes)
- [x] Line-level diff with color output
- [x] File-specific diff mode
- [x] File viewer at any checkpoint
- [x] Checkpoint deletion
- [x] Status (changes since last save)
- [x] `.snapignore` support
- [x] VS Code extension with native diff
- [x] Cross-platform binaries (macOS/Windows/Linux)
- [ ] Garbage collector (time-based auto-save cleanup)
- [ ] Named states / tags (⭐ "last known good")
- [ ] Snapshot export to Git commit
- [ ] Auto-save hooks (before/after agent runs)

---

## License

MIT
