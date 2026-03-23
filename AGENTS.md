## Repo map

- `SPEC.md` — file formats, schemas, adapter interfaces (source of truth for behavior)
- `ARCHITECTURE.md` — internal design and Go project layout
- `DECISIONS.md` — design decisions and rationale (append new decisions here)
- `cmd/ski/` — CLI entry point
- `internal/cli/` — one file per command
- `internal/manifest/` — ski.toml parse/write
- `internal/lockfile/` — ski.lock.json read/write
- `internal/store/` — central store (~/.ski/store)
- `internal/source/` — source adapters (`git/` in the MVP; `github/` can be added later)
- `internal/target/` — target adapters (claude/, codex/, cursor/, openclaw/)
- `internal/scanner/` — security scanners

## Conventions

- Go with cobra for CLI
- TOML for config (ski.toml), JSON for lockfile (ski.lock.json)
- Adapter pattern: new sources/targets = new adapter, no core changes
- Keep one copy per skill in store, identified by commit SHA
- Copied skill directories in agent dirs, sourced from store; never raw symlinks

## Boundaries

### Ask First
- Large cross-top-level-folder refactors
- New dependencies with broad impact
- Destructive data or migration changes
- Changes to ski.toml or ski.lock.json schema

### Never
- Commit secrets, credentials, or tokens
- Break the source/target adapter interfaces without updating SPEC.md

## Git Commit Conventions
- Format: feat|fix|docs|refactor: short summary

## Agent Workflow

1. Research Phase: Analyze the entire codebase
2. Planning Phase: Include file paths, code snippets, trade-offs
3. Execution Phase
