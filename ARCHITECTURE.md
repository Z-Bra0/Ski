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
├── Lockfile Manager     (read/write ski.lock.json)
├── Target Adapters      (link skills into .claude/skills, .codex/skills, etc.)
├── Security Scanner     (rule-based + external scanners)
└── Doctor / Maintenance (symlink checks, prune, consistency validation)
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
    cli/                # one file per command
    manifest/           # ski.toml parse/write
    lockfile/           # ski.lock.json read/write
    store/              # central store: fetch, cache, gc
    source/             # source adapters (git/ in the MVP)
    target/             # target adapters (claude/, codex/, cursor/, openclaw/)
    scanner/            # security scanners (rules/, external/)
```

---

## Future Extensions

- dependency resolution between skills
- content-addressed store (hash-based dedup)
- skill signing and verification
- capability sandboxing
- auto-detect agent platforms (v2)
