# Decision Log

Key design decisions and their rationale.

- TOML for manifest, JSON for lockfile — TOML is readable for hand-editing (ski.toml); JSON is easy to generate and diff (ski.lock.json)
- Project-scoped only, no global install — keeps behavior predictable; global state causes team sync issues
- Version is informational — avoids multi-version store complexity; one copy per skill, version only used for update checks
- `@ref` syntax for pinning — `git:<url>@v1.0.0` or `@commitSHA`; keeps refs explicit while staying close to familiar package-manager conventions
- `ski add` = write + fetch + link — same as `npm install <pkg>`; no separate install step needed after add
- Manifest skill names default from discovered `SKILL.md` metadata — keeps local names aligned with the upstream skill contract; `--name` remains available as a project-local alias for single-skill adds
- SHA-256 for integrity — full directory hash, sorted walk; MD5 too weak for integrity checks
- `github` deferred — it is mostly UX sugar over Git-hosted repositories; implement `git` first for the MVP
- `skillhub`/`clawhub` adapters deferred — API formats unknown; they are outside the MVP
- Windows/symlink support deferred — needs research; macOS/Linux only for v1
- Multi-skill repos via `#subpath` — original idea, now superseded by skill-name selectors because selecting by `SKILL.md` metadata keeps the source syntax simpler for users
- Multi-skill repos via skill-name selectors — `ski add` recursively discovers `SKILL.md` files up to depth 3, prompts on TTY when multiple skills are found, and uses `git:<url>[@ref]##skill-a,skill-b` plus `--all` for non-interactive selection. Literal `@`, `#`, and `\` in the URL/ref are escaped as `\@`, `\#`, and `\\`. Canonical manifest entries are written one selected skill at a time, for example `git:<url>##skill-a`.
