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

## Build

```bash
make build
```

---

## Test

```bash
make test
```

---

## Quick Start

```bash
ski init
ski add https://github.com/org/repo-map.git
ski install
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
```

---

## Usage Notes

- Use `ski` only with skill repositories you have verified and trust. Review the upstream repo and `SKILL.md` before `add`, `install`, or `update`.
- `ski add` prompts when a repo contains multiple skills. In non-interactive mode, use `--skill` or `--all`.
- URL-form git sources may omit the `git:` prefix, but plain local filesystem paths still require it, for example `git:/tmp/repo-map`.
- Local targets write into the project. `-g` uses `~/.ski/global.toml` and global agent directories instead.
- Custom target folders use `dir:`. For example: `dir:./agent-skills/claude`.

---

## Docs

- [SPEC.md](SPEC.md) — file formats, schemas, adapter interfaces
- [ARCHITECTURE.md](ARCHITECTURE.md) — internal design and Go layout
- [DECISIONS.md](DECISIONS.md) — design decisions and rationale

---

## License

GPL-3.0. See [LICENSE](LICENSE).
