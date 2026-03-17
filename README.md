# ski

A package manager for AI agent skills.

Install skills from Git repositories into agent platforms such as Claude, Codex, Cursor, and OpenClaw with a manifest, lockfile, and shared store.

---

## Status

```bash
Implemented in v1:
- git: sources
- local and global scope
- init, add, install, remove, update, list, doctor
```

Deferred:
- github: shorthand
- info, search, scan, prune
- registry-style sources

---

## Build

```bash
go build ./cmd/ski
go run ./cmd/ski -- help
```

---

## Quick Start

```bash
ski init                         # create ski.toml
# edit ski.toml and set targets = ["claude"]
ski add https://github.com/org/repo-map.git
ski list                         # show installed skills
ski doctor                       # verify links and lock state

# another machine or fresh clone:
ski install                      # restore from ski.toml + ski.lock.json
```

Use `ski` only with skill repositories you have verified and trust. Before `ski add`, `ski install`, or `ski update`, review the upstream repo and `SKILL.md`, and re-check the diff before upgrading to a newer ref or commit.

If a repo contains multiple skills, `ski add` prompts in a terminal. In non-interactive mode, use `ski add <source> --skill skill-a --skill skill-b` or `ski add <source> --all`. Legacy `##skill-a,skill-b` source selectors are still accepted during migration.

If the repo URL or local path contains a literal `@`, `#`, or `\`, escape it in the source string as `\@`, `\#`, or `\\`. Example: `git:/tmp/skill\#\#pack`.

URL-form git sources may omit the `git:` prefix, including `https://...`, `ssh://...`, `git://...`, and `file://...`. Plain local filesystem paths still require it, for example `git:/tmp/repo-map`.

`targets = ["claude"]` in `ski.toml` means project-local installation into `./.claude/skills/`. Use `-g` to operate on `~/.ski/global.toml` / `~/.ski/global.lock.json` and link built-in targets into user-global agent directories such as `~/.claude/skills/`.

Custom target folders use a `dir:` prefix. In local scope, `dir:./agent-skills/claude` resolves relative to the repo root. In global scope, `dir:agent-skills/claude` resolves relative to the user home directory, and `~` expansion is allowed.

---

## Commands

```bash
ski init [-g]                    # create the local or global manifest
ski add [-g] <source>            # add + fetch + link
ski add [-g] <source> --skill x  # add selected upstream skill(s) from one repo
ski add [-g] <source> --all      # add all discovered skills from one repo
ski add [-g] <source> --name x   # alias one selected skill locally
ski install [-g]                 # restore from manifest + lockfile
ski remove [-g] <skill>          # remove one skill from the active scope
ski update [-g] [skill]          # update all skills or one skill
ski update [-g] --check [skill]  # report available updates only
ski list [-g]                    # list declared skills
ski doctor [-g]                  # check links and lock/store consistency
```

---

## Docs

- [SPEC.md](SPEC.md) — file formats, schemas, adapter interfaces
- [ARCHITECTURE.md](ARCHITECTURE.md) — internal design and Go layout
- [DECISIONS.md](DECISIONS.md) — design decisions and rationale

MVP source support is git repositories via canonical `git:` sources or bare URL-form sources such as `https://...` and `file://...`. `github:` may be added later as a convenience alias over Git-hosted repositories.
