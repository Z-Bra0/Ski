# ski

A package manager for AI agent skills.

Install skills from Git repositories into agent platforms such as Claude, Codex, Cursor, and OpenClaw with a manifest, lockfile, and shared store.

---

## Status

- git repositories as skill sources
- local and global scope
- `init`, `add`, `install`, `remove`, `update`, `list`, and `doctor`

---

## Limitations

- Git-only sources
- Trust is manual
- No Windows support

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/Z-Bra0/Ski/master/scripts/install.sh | sh
```

Install a specific release:

```bash
curl -fsSL https://raw.githubusercontent.com/Z-Bra0/Ski/master/scripts/install.sh | sh -s -- --version v0.1.1
```

---

## Quick Start

```bash
ski init
ski add https://github.com/org/repo-map.git
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
ski add [-g] <source>
ski install [-g]
ski list [-g]
ski doctor [-g]
ski update [-g] [skill]
ski remove [-g] <skill>
ski version
```

---

## Usage Notes

- Use `ski` only with skill repositories you have verified and trust. Review the upstream repo and `SKILL.md` before `add`, `install`, or `update`.
- `ski add` prompts when a repo contains multiple skills. In non-interactive mode, use `--skill` or `--all`.
- Supported sources are remote Git endpoints. You can use `git:https://...` or omit the `git:` prefix for URL-form sources such as `https://...`, `ssh://...`, and `git://...`.
- `ski version` reports the CLI build version. Dev builds print `dev`; release builds use the version passed to `make release VERSION=...`.
- `make release VERSION=...` also writes `dist/ski_<version>_checksums.txt` for installer verification.
- Local targets write into the project. `-g` uses `~/.ski/global.toml` and global agent directories instead.
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
