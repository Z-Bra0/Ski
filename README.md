# ski

Lightweight, Git-based toolkit for sharing agent skills across repos without copy-paste drift.

`ski` helps teams reuse the same skills across multiple codebases while keeping installs reproducible and repo-aware.

## Best Fit

- teams sharing skills across multiple repos
- per-repo version pinning and restore
- project-scoped or global installs
- automation-friendly

## Not For

- skill registries
- marketplaces
- public skill discovery

---

## Install

Install with Homebrew:

```bash
brew tap Z-Bra0/skicli
brew install skicli
```

Homebrew installs the formula as `skicli`, but the command is still `ski`.

Or install with the release script:

```bash
curl -fsSL https://raw.githubusercontent.com/Z-Bra0/Ski/master/scripts/install.sh | sh
```

---

## Quick Start

Adopt a shared skill in one repo:

```bash
ski init --target claude
ski add git:https://github.com/anthropics/skills.git --skill skill-creator
```

`ski add` is the first-time workflow: it updates `ski.toml`, resolves and writes `ski.lock.json`, fetches the skill into the store, and copies it into the configured targets.

Use `ski install` later to restore skills from `ski.toml` and `ski.lock.json`, for example in a fresh clone.

---

## Notes

- Use `ski` only with skill repositories you have verified and trust.
- `ski add` is for first-time add + lock + install. `ski install` restores from `ski.toml` and `ski.lock.json`.
- Local installs write into the project. Use `-g` for global manifest and global target directories.
- Use `ski disable <skill>` to keep tracking a skill without installing it into targets. Use `ski enable <skill>` to restore it later.

---

## Docs

- [docs/usage.md](docs/usage.md) — usage patterns, targets, refs, and troubleshooting
- [SPEC.md](SPEC.md) — file formats, schemas, and adapter interfaces
- [ARCHITECTURE.md](ARCHITECTURE.md) — internal design and Go layout
- [DECISIONS.md](DECISIONS.md) — design decisions and rationale

---

## Status

- git repositories as skill sources
- local and global scope
- `init`, `add`, `install`, `remove`, `update`, `list`, `info`, `enable`, `disable`, and `doctor`

---

## Limitations

- Git-only sources
- Trust is manual
- No Windows support

---

## Commands

```bash
ski init [-g]
ski add [-g] [--target target]... <source>
ski enable [-g] <skill>
ski disable [-g] <skill>
ski install [-g]
ski list [-g]
ski info [-g] <skill>
ski doctor [-g] [--fix]
ski update [-g] [skill]
ski remove [-g] [--target target]... <skill>
ski version
```

---

## Build

```bash
make build                     # local dev build; `ski version` prints `dev`
make release VERSION=0.2.1
```

---

## Test

```bash
make test
```

---

## Author

[Z-Bra](https://x.com/Z_Bra0)

---

## License

GPL-3.0. See [LICENSE](LICENSE).
