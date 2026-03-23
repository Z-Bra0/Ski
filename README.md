# ski

Managing AI agent skills across tools is still manual, duplicated, and hard to reproduce.

Teams copy skill folders into `.claude/skills`, `.codex/skills`, and other agent directories by hand, then lose track of which repo, tag, or commit each project is actually using.

`ski` turns that into a package-manager workflow for agent skills. Install skills from Git into Claude, Codex, Cursor, and OpenClaw with a manifest, lockfile, and shared store-backed copied installs.

---

## Status

- git repositories as skill sources
- local and global scope
- `init`, `add`, `install`, `remove`, `update`, `list`, `info`, and `doctor`

---

## Limitations

- Git-only sources
- Trust is manual
- No Windows support

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

```bash
ski init --target claude
ski add https://github.com/org/repo-map.git
```

`ski add` is the first-time workflow: it updates `ski.toml`, resolves and writes `ski.lock.json`, fetches the skill into the store, and copies it into the configured targets.

Use `ski install` later to restore skills from `ski.toml` and `ski.lock.json`, for example in a fresh clone.

---

## Examples

Share one `repo-map` skill across Claude and Codex:

```bash
ski init --target claude --target codex
ski add https://github.com/org/repo-map.git
```

This keeps one stored snapshot of the skill and materializes matching copies into both `.claude/skills` and `.codex/skills`.

Pin a team audit skill to a specific commit:

```bash
ski init --target claude
ski add git:https://github.com/acme/team-audit-skill.git@9f3c2ab
```

This makes the project reproducible because the manifest and lockfile keep the selected source and resolved revision explicit.

Restore skills from the manifest and lockfile in a fresh clone:

```bash
ski install
```

---

## Build

```bash
make build                     # local dev build; `ski version` prints `dev`
make release VERSION=0.1.1
```

---

## Test

```bash
make test
```

---

## Commands

```bash
ski init [-g]
ski add [-g] [--target target]... <source>
ski install [-g]
ski list [-g]
ski info [-g] <skill>
ski doctor [-g]
ski update [-g] [skill]
ski remove [-g] [--target target]... <skill>
ski version
```

---

## Usage Notes

- Use `ski` only with skill repositories you have verified and trust. Review the upstream repo and `SKILL.md` before `add`, `install`, or `update`.
- `ski add` prompts when a repo contains multiple skills. In non-interactive mode, use `--skill` or `--all`.
- `ski add --target ...` writes a per-skill target override for new skills. If the skill already exists with the same identity, `add --target ...` extends it into those additional targets.
- `ski remove --target ...` removes a skill only from those targets. Without `--target`, `remove` deletes the skill entry entirely.
- `ski list` shows 1-based skill references like `@1`. Those references are scope-local and can be used with `info`, `update`, `remove`, and `add @N --target ...`.
- Supported sources are remote Git endpoints. You can use `git:https://...` or omit the `git:` prefix for URL-form sources such as `https://...`, `ssh://...`, and `git://...`.
- `ski version` reports the CLI build version. Dev builds print `dev`; release builds use the version passed to `make release VERSION=...`.
- `make release VERSION=...` also writes `dist/ski_<version>_checksums.txt` for installer verification.
- Local targets write into the project. `-g` uses `~/.ski/global.toml` and global agent directories instead.
- Built-in targets include `cursor`, `claude`, `copilot`, `codex`, `windsurf`, `gemini`, `antigravity`, `opencode`, `openclaw`, `goose`, and `agents`.
- Custom target folders use `dir:`. For example: `dir:./agent-skills/claude`.

---

## Docs

- [SPEC.md](SPEC.md) — file formats, schemas, adapter interfaces
- [ARCHITECTURE.md](ARCHITECTURE.md) — internal design and Go layout
- [DECISIONS.md](DECISIONS.md) — design decisions and rationale

---

## Author

[Z-Bra](https://x.com/Z_Bra0)

---

## License

GPL-3.0. See [LICENSE](LICENSE).
