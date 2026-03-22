# ski Architecture

For file formats, adapter interfaces, and behavioral contracts see [SPEC.md](SPEC.md).

---

## System Overview

```
CLI
│
Application Core
│
├── Source Adapters      (fetch skills from git in the MVP)
├── Skill Store          (~/.ski/store — central on-disk cache)
├── Manifest/Lockfile    (read/write local and global state)
├── Target Adapters      (link skills into .claude/skills, .codex/skills, .opencode/skills, etc.)
└── Doctor / Maintenance (symlink checks, consistency validation)
```

---

## Key Design Decisions

**Central store** — all skills land in `~/.ski/store/<adapter>/<name>/<commit>/`, shared across projects. Deduplication and caching come for free.

**Symlinks, not copies** — agent directories hold symlinks into the store. One skill file, many targets.

**Two-sided adapters** — source adapters normalize fetching; target adapters normalize linking. New registries and agent platforms are just new adapters.

---

## Go Project Layout

```
ski/
  cmd/ski/              # entry point

  internal/
    app/                # orchestration for local/global operations
    cli/                # one file per command
    manifest/           # ski.toml parse/write
    lockfile/           # ski.lock.json read/write
    store/              # central store: fetch, cache, gc
    source/             # source adapters (git/ in the MVP)
    target/             # target adapters and built-in target registry
```

---

## Future Extensions

- packaged release/install story
- dependency resolution between skills
- content-addressed store (hash-based dedup)
- prune command for unused store entries
- scanner module and security scanning workflow
- skill signing and verification
- capability sandboxing
- auto-detect agent platforms (v2)
