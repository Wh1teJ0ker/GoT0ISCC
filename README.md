# GoT0ISCC

`GoT0ISCC` is a desktop ISCC workspace built with `Go + Wails + React`. It consolidates account management, track snapshots, theory automation, writeup sync, desktop tooling, and managed Python execution into a single desktop application.

## Features

- Account management with local credential, proxy, and retry settings
- Practice and arena track snapshots backed by local synchronized data
- Theory workflow with bank search, manual submit, AI settings, and automation status
- Combat workspace integration for challenge snapshots and submissions
- Writeup snapshot and remote sync tracking
- Managed Python environment bootstrap and sandbox execution
- Migration bundle export for local runtime state

## Stack

- Go `1.24.x`
- Wails `v2.10.2`
- React `18`
- Vite `3`
- SQLite

## Repository layout

```text
GoT0ISCC/
  .github/                  # GitHub Actions, templates, metadata
  cmd/                      # helper CLI tools
  extensions/               # browser extension assets and helpers
  frontend/                 # React app and Wails-generated bindings
  internal/                 # application, domain, platform, desktop API
  scripts/                  # local packaging and export scripts
  tools/                    # maintenance and normalization tools
  main.go                   # Wails application entrypoint
  wails.json                # Wails configuration
```

## Prerequisites

- Go `1.24.x`
- Node.js `20+`
- Wails CLI `v2.10.2`

Install Wails CLI:

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
```

## Local development

Install frontend dependencies:

```bash
cd frontend
npm ci
```

Generate Wails bindings after Go API changes:

```bash
wails generate module
```

Start development mode:

```bash
wails dev
```

Validate the repository:

```bash
go test ./...
cd frontend && npm run build
```

## Data policy

Runtime data lives under `data/` and is intentionally excluded from Git:

- `data/got0iscc.db`
- `data/runtime/`
- `data/challenges/`
- `data/python/`

Build outputs and local runtime state are also excluded:

- `build/`
- `runtime/`
- `frontend/node_modules/`
- local virtual environments

## GitHub automation

The repository includes a cross-platform workflow:

- Ubuntu: frontend build + `go test ./...`
- macOS: Wails desktop build + uploaded artifact
- Windows: Wails desktop build + uploaded artifact

Relevant files:

- [`.github/workflows/build.yml`](.github/workflows/build.yml)
- [`CONTRIBUTING.md`](CONTRIBUTING.md)
- [`.github/PULL_REQUEST_TEMPLATE.md`](.github/PULL_REQUEST_TEMPLATE.md)
- [`.github/dependabot.yml`](.github/dependabot.yml)

## Release notes

Local packaging remains available in:

- [`scripts/package_release.sh`](scripts/package_release.sh)

That script is tuned for the local macOS environment. CI packaging is handled separately through GitHub Actions.

## Maintenance notes

- `frontend/wailsjs/` is tracked because the frontend imports generated Wails bindings directly.
- `frontend/dist/index.html` is kept as a minimal placeholder so Go embed and `go test` succeed on a clean clone.
- After desktop API signature changes, regenerate bindings before commit.
