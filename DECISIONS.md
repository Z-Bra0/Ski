# Decision Log

Key design decisions and their rationale.

- TOML for manifest, JSON for lockfile — TOML is readable for hand-editing (ski.toml); JSON is easy to generate and diff (ski.lock.json)
- Project-scoped only, no global install — keeps behavior predictable; global state causes team sync issues
- Version is informational — avoids multi-version store complexity; one copy per skill, version only used for update checks
- `@ref` syntax for pinning — `git:<url>@v1.0.0` or `@commitSHA`; keeps refs explicit while staying close to familiar package-manager conventions
- `ski add` = write + fetch + link — same as `npm install <pkg>`; no separate install step needed after add
- `git` adapter name = last URL path segment minus `.git` — simple, predictable; collision errors ask user to set explicit name
- SHA-256 for integrity — full directory hash, sorted walk; MD5 too weak for integrity checks
- `github` deferred — it is mostly UX sugar over Git-hosted repositories; implement `git` first for the MVP
- `skillhub`/`clawhub` adapters deferred — API formats unknown; they are outside the MVP
- Windows/symlink support deferred — needs research; macOS/Linux only for v1
- Multi-skill repos via `#subpath` — extend source syntax to `git:<url>[@ref]#path/to/skill`; `EnsureGit` moves only the subpath into the store; name derived from last path segment; deferred post-MVP
