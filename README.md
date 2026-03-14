# ski

A package manager for AI agent skills.

Install skills from Git repositories into any agent platform — Claude, Codex, Cursor, and more — with a single manifest and lockfile.

---

## Install

```bash
brew install ski
```

```bash
curl -fsSL https://ski.sh/install | sh
```

---

## Quick Start

```bash
ski init                         # create ski.toml
ski add git:https://github.com/org/repo-map.git
ski install                      # restore from ski.toml + ski.lock.json
ski list                         # show installed skills
```

`targets = ["claude"]` in `ski.toml` means project-local installation into `./.claude/skills/`. v1 does not write to `~/.claude/skills/` or other global agent directories.

---

## Commands

```bash
ski init                   # create ski.toml
ski add <source>           # add + fetch + link (like npm install <pkg>)
ski install                # restore from manifest + lockfile
ski remove <skill>         # remove skill
ski update [skill]         # update all or one skill to latest
ski update --check         # dry run — report outdated without changing
ski list                   # list installed skills
ski info <skill>           # show skill details
ski search <query>         # search across sources
ski scan [--all]           # security scan
ski doctor                 # check for broken links / inconsistencies
ski prune                  # remove unused skills from store
```

---

## Development

```bash
# Run all tests
go test ./...

# Run the CLI locally
go run ./cmd/ski -- help

# Build the binary
go build ./cmd/ski
```

---

## Docs

- [SPEC.md](SPEC.md) — file formats, schemas, adapter interfaces
- [ARCHITECTURE.md](ARCHITECTURE.md) — internal design and Go layout

MVP source support is `git:` only. `github:` may be added later as a convenience alias over Git-hosted repositories.
