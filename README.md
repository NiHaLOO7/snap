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

## Why Snap?

Git is great for publishing code. But between commits, your code is unprotected.

| Problem | Snap Solution |
|---------|---------------|
| Agent changes 20 files, something breaks | `snap restore` — back to safety in 1 second |
| Agent "reverts" but misses 3 files | Every save is a complete project snapshot |
| Experimenting and want to compare states | `snap diff 3 7` — any two points |
| Don't want to pollute Git history | Snap is completely separate from Git |
| Lost track of what changed | `snap status` — instant overview |

## Installation

```bash
# Build from source
cd snap
go build -o snap ./cmd/snap/

# Move to PATH
mv snap /usr/local/bin/
```

## Quick Start

```bash
# Initialize in your project
cd your-project
snap init

# Save your first checkpoint
snap save "initial working state"

# Make changes, let agents run, experiment...
# Then save again
snap save "after auth refactor"

# See what changed
snap status

# Compare any two checkpoints
snap diff 1 2

# Something broke? Restore instantly
snap restore 1
# (current state is auto-saved, you never lose work)
```

## Commands

### `snap init`

Initialize snap in the current directory. Creates a `.snap/` folder.

```
$ snap init
Initialized snap in /Users/you/project/.snap/

Ready to save snapshots. Run:
  snap save "initial state"
```

### `snap save [message]`

Take a snapshot of the entire project state.

```
$ snap save "before oauth implementation"
Saved snapshot #4
  Message:  before oauth implementation
  Files:    47
  Time:     12ms
```

**How it works internally:**
- Walks all project files (respecting `.snapignore`)
- Hashes each file with SHA-256
- Only stores files that changed (content-addressed deduplication)
- Compresses with zlib
- Saves metadata (timestamp, message, file tree)

### `snap list` / `snap ls`

Show all checkpoints in timeline order.

```
$ snap list
Snapshots (5 total):

  ● #1     Aug 11 10:02  initial state  (42 files)
  │
  ● #2     Aug 11 10:34  before auth refactor  (42 files)
  │
  ● #3     Aug 11 11:15  after claude changes  (47 files)
  │
  ● #4     Aug 11 11:20  auto-save before restore to #2  (47 files) [auto]
  │
  ◉ #5     Aug 11 11:45  working oauth  (48 files)
```

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

**Safety guarantee:** Before restoring, your current state is automatically saved as a new checkpoint marked `[auto]`. You can always get it back.

### `snap diff <id> [id2]`

Compare any two states. If only one ID is given, compares against current working directory.

```
# Snapshot vs current working directory
$ snap diff 3
Snapshot #3 "after claude changes"  →  Current working directory

  + src/new_file.go  (added)
  ~ src/auth.go  (modified)
  - src/deprecated.go  (deleted)

  1 modified, 1 added, 1 deleted

# Snapshot vs snapshot
$ snap diff 1 5
Snapshot #1 "initial state"  →  Snapshot #5 "working oauth"

  + src/oauth.go  (added)
  + src/token.go  (added)
  ~ src/main.go  (modified)
  ~ src/config.go  (modified)

  2 modified, 2 added, 0 deleted

# Full line-level diff
$ snap diff 1 5 -f
Snapshot #1 "initial state"  →  Snapshot #5 "working oauth"

  + src/oauth.go  (added)
  ~ src/main.go  (modified)
    @@ -1,4 +1,7 @@
      package main
    +
    + import "github.com/you/oauth"
    +
      func main() {
    +     oauth.Init()
          // ...
```

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

## Storage Design

Snap uses a **content-addressed object store** (the same approach as Git internally):

```
.snap/
├── objects/              # Content-addressed blob store
│   ├── a3/
│   │   └── 4f8b2c...    # zlib-compressed file content
│   ├── 7c/
│   │   └── 2e91f0...
│   └── ...
├── snapshots/            # Snapshot metadata
│   ├── 0001.json
│   ├── 0002.json
│   └── ...
└── config.json
```

**Key properties:**
- **Deduplication**: If a file hasn't changed, it's not stored again. The snapshot just references the existing blob by hash.
- **Compression**: All blobs are zlib-compressed.
- **Integrity**: SHA-256 hashes guarantee data integrity.
- **Speed**: Only changed files are processed on each save.

A project with 1000 files where only 5 changed? Only 5 new blobs stored.

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
.vscode

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

## VS Code Extension

Snap comes with a VS Code extension for visual checkpoint management.

**Features:**
- **Sidebar timeline**: See all snapshots in activity bar
- **One-click save**: Save checkpoint with message prompt
- **Visual diff**: Click any snapshot to see full diff
- **Restore with confirmation**: Right-click → Restore (with safety warning)
- **Live changes panel**: See what's changed since last snapshot (auto-refreshes)

**Install:**
```bash
cd vscode-extension
npm install
npm run compile
# Then install the .vsix or run in development mode
```

The extension calls the `snap` CLI binary under the hood — install the CLI first.

## Capabilities Summary

| Feature | Description |
|---------|-------------|
| **Instant Save** | Full project snapshot in milliseconds |
| **Content Dedup** | Only stores what actually changed (SHA-256 addressed) |
| **Zlib Compression** | All blobs compressed, minimal disk usage |
| **Safe Restore** | Auto-saves before every restore — zero data loss |
| **Tree Diff** | Compare file trees between any two checkpoints |
| **Line Diff** | Myers algorithm line-by-line diff (same as Git) |
| **Status** | What changed since last snapshot |
| **Ignore Patterns** | `.snapignore` file + smart defaults |
| **VS Code UI** | Full sidebar integration, visual diff |
| **Git Compatible** | Lives alongside Git, doesn't interfere |
| **Offline** | 100% local, no network, no accounts |
| **Fast** | Sub-second saves even for large projects |

## Use Cases

**1. AI Agent Safety Net**
```bash
snap save "before claude refactors"
# Let agent run...
# Something broke?
snap restore 4
```

**2. Experimental Feature Development**
```bash
snap save "stable baseline"
# Try approach A
snap save "approach A"
snap restore 1
# Try approach B
snap save "approach B"
snap diff 2 3    # Compare approaches
```

**3. Mid-Feature Progress**
```bash
# Working on a feature, not ready to commit
snap save "auth halfway done"
# Continue working
snap save "auth 80% done"
# Keep going until ready for Git
git add . && git commit -m "feat: add authentication"
```

**4. Before Risky Operations**
```bash
snap save "before database migration changes"
# Make risky changes...
# If it goes wrong:
snap restore 7
```

## Architecture

```
cmd/snap/main.go           # CLI entry point, command routing
internal/
├── store/store.go         # Content-addressed object store (SHA-256 + zlib)
├── snapshot/snapshot.go   # Save/restore/list engine
├── diff/diff.go           # Tree comparison + Myers line diff
├── ignore/ignore.go       # .snapignore pattern matching
└── config/                # Configuration (reserved)
vscode-extension/          # VS Code sidebar + diff UI
```

## Tech Stack

- **Language**: Go
- **Hashing**: SHA-256 (crypto/sha256)
- **Compression**: zlib (compress/zlib)
- **Storage**: File-based, content-addressed
- **Diff Algorithm**: Myers (same as Git)
- **VS Code Extension**: TypeScript
- **Dependencies**: Zero external Go dependencies (stdlib only)

## Project State

- [x] Content-addressed object store
- [x] Snapshot save with deduplication
- [x] Snapshot restore with auto-save safety
- [x] Timeline listing
- [x] Tree-level diff (file changes)
- [x] Line-level diff (Myers algorithm)
- [x] Status (changes since last save)
- [x] `.snapignore` support
- [x] VS Code extension
- [ ] Garbage collector (delete old snapshots, clean unreferenced blobs)
- [ ] Named states / tags (⭐ "last known good")
- [ ] Snapshot export to Git commit
- [ ] Auto-save hooks (before/after agent runs)

## License

MIT
