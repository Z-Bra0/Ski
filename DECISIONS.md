# Decision Log

Current design decisions and their rationale.

## State And Data Model

- TOML for manifest, JSON for lockfile — `ski.toml` is readable for hand-editing; `ski.lock.json` is easy to generate and diff.
- Separate local and global scope — repo state stays reproducible in `ski.toml` / `ski.lock.json`, while personal installs live in `~/.ski/global.toml` / `~/.ski/global.lock.json`.
- Manifest and lockfile schema versions are explicit — schema evolution is tracked by top-level `version` fields rather than inferred from contents.
- Skill entry `version` is informational — store identity is still commit-based; the skill `version` field is used only for display and update reporting.
- `upstream_skill` is stored separately from `source` — the canonical manifest and lockfile keep the repo location and selected skill as separate fields, instead of encoding skill selection into the source string.

## Sources

- `git` is the only MVP source adapter — it covers the actual fetch and resolution model with the smallest surface area.
- Remote-only git sources — manifests accept remote Git endpoints only; local filesystem repositories were removed to keep manifests portable and reduce scope-specific path handling.
- `@ref` syntax for pinning — `git:<url>@v1.0.0` or `@commitSHA` keeps refs explicit while staying close to familiar package-manager conventions.
- `github` is deferred — it is mostly UX sugar over Git-hosted repositories and does not change the core fetch model.
- `skillhub` / `clawhub` are deferred — API formats and trust models are unknown, so they are outside the first release.

## Skill Discovery And UX

- `ski add` performs declare + fetch + lock + link — the first add should leave the project ready to use without a separate install step.
- Manifest skill names default from discovered `SKILL.md` metadata — local names stay aligned with the upstream skill contract; `--name` remains available as a local alias for a single selected skill.
- Multi-skill repositories are selected by discovered skill name — `ski add` scans for `SKILL.md` files up to depth 3, supports explicit `--skill` selection, `--all`, and interactive selection on TTYs.
- Legacy `##skill` selectors are read-only migration input — the CLI may still accept them on input, but canonical manifests and lockfiles write `source` plus `upstream_skill` instead.
- Skill validation is compatibility-first — `ski` hard-fails only on metadata that would break its own install model, while strict Agent Skills spec mismatches remain non-fatal; `ski add` currently surfaces them as warnings so broader ecosystem repos still work.

## Integrity And Targets

- SHA-256 for integrity — the lockfile stores a SHA-256 hash of the full stored repository snapshot, not just the selected subdirectory.
- `dir:` prefixes custom target folders — built-in target names stay unambiguous while custom destinations remain explicit and scope-aware.
- Windows support is deferred — the first release targets macOS and Linux and relies on symlink behavior that still needs Windows-specific design work.
