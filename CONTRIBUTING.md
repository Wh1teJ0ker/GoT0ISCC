# Contributing

## Scope

This repository contains the Wails desktop application for `GoT0ISCC`. Runtime data, challenge attachments, local databases, and release artifacts must stay out of Git.

## Development baseline

- Go `1.24.x`
- Node.js `20+`
- Wails CLI `v2.10.2`

## Before opening a pull request

1. Install frontend dependencies with `npm ci` in `frontend/`.
2. Regenerate Wails bindings with `wails generate module` after API signature changes.
3. Run `go test ./...` from the repository root.
4. Run `npm run build` in `frontend/`.
5. Verify that no runtime data or build artifacts were staged.

## Commit scope

- Keep changes focused.
- Separate refactors from behavior changes where practical.
- Do not commit `data/`, `build/`, `runtime/`, `frontend/node_modules/`, or local virtual environments.

## Pull request notes

Include:

- the problem being solved
- the user-visible effect
- validation steps you ran
- any follow-up work that remains
