# Contributing to Snap

## Branch Naming

| Type | Pattern | Example |
|------|---------|---------|
| New feature | `feat/name` | `feat/parallel-workspaces` |
| Bug fix | `fix/name` | `fix/scope-detection` |
| Refactor | `refactor/name` | `refactor/object-store` |
| Docs | `docs/name` | `docs/api-reference` |
| Performance | `perf/name` | `perf/dedup-hashing` |
| Chore (CI, deps, config) | `chore/name` | `chore/ci-pipeline` |
| Experiment | `exp/name` | `exp/mcp-server` |

Names: lowercase, hyphen-separated, short.

## Commit Messages

Follow conventional commits:

```
feat: add parallel workspaces
fix: scope detection from subdirectories
refactor: simplify object store hashing
docs: update roadmap with future vision
perf: reduce dedup overhead on large files
chore: update CI workflow
```

One-line summary. Add a body only if the "why" isn't obvious from the title.

## Development Setup

```bash
# Clone
git clone https://github.com/NiHaLOO7/snap.git
cd snap

# Build
go build -o snap ./cmd/snap/

# Run tests
go test ./...

# Install locally
cp snap /usr/local/bin/snap
```

## VS Code Extension

```bash
cd vscode-extension
npm install
npm run compile
# Press F5 in VS Code to launch Extension Development Host
```

## Pull Requests

- One PR per logical change
- Branch off `main`, PR back to `main`
- Keep PRs small and focused
- Include a brief description of what and why

## Project Structure

```
snap/
  cmd/snap/main.go    — CLI entry point (all commands)
  internal/snap/      — Core library (object store, snapshots, diff)
  vscode-extension/   — VS Code extension (TypeScript)
```
