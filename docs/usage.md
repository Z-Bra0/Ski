# Usage

`ski` is optimized for shared skills that are sourced from Git, copied into repo-local or global target directories, and restored from a manifest plus lockfile.

## Adding and Extending Skills

Use `ski add <source>` for the first install. It updates `ski.toml`, resolves and writes `ski.lock.json`, fetches the skill snapshot into the store, and copies it into the configured targets.

If the source repo contains multiple skills:

- on a TTY, `ski add` prompts for selection
- in non-interactive use, pass `--skill <name>` or `--all`

Use `--target` to override targets for the added skill entry:

```bash
ski add git:https://github.com/anthropics/skills.git --skill skill-creator --target claude --target codex
```

If a skill with the same repository URL and `upstream_skill` already exists, `ski add` updates that existing skill in place. If `--target` is also provided, the new targets are unioned into the existing skill.

## Restoring and Updating

Use `ski install` to restore skills from `ski.toml` and `ski.lock.json`, for example in a fresh clone or CI job.

If `ski.lock.json` does not exist yet, `ski install` resolves the manifest entries, creates the lockfile, and installs the resulting skills.

Use `ski update` to refresh tracked skills:

- `ski update` updates all skills
- `ski update <skill>` updates one skill
- `ski update --check` reports available updates without changing anything

`ski update --check` prints:

- `NAME` — local skill name
- `TRACKING` — explicit manifest ref, or the remote default branch when no `@ref` is set
- `CURRENT` — current locked commit short SHA, or `(none)` when no lock entry exists yet
- `LATEST` — latest resolved commit short SHA
- `LATEST_AT` — latest resolved commit date in `YYYY-MM-DD`

## References and Target Overrides

`ski list` shows 1-based skill references like `@1`. Those references are scope-local shortcuts and can be used with:

- `ski info @1`
- `ski update @1`
- `ski remove @1`
- `ski add @1 --target codex`

Use `ski remove --target ...` to remove only selected targets from a skill. Without `--target`, `remove` deletes the skill entry entirely.

## Source Formats and Targets

Supported source formats are remote Git endpoints:

- `git:https://...`
- `https://...`
- `ssh://...`
- `git://...`

`@ref` is optional. When omitted, `ski` tracks the remote default branch via `HEAD`. When provided, the ref may be a simple branch name, tag name, or commit SHA.

Local installs write into the project. `-g` uses `~/.ski/global.toml` and global target directories instead.

Built-in targets currently include:

- `claude`
- `codex`
- `cursor`
- `copilot`
- `windsurf`
- `gemini`
- `antigravity`
- `openclaw`
- `opencode`
- `goose`
- `agents`

Custom target folders use `dir:`. Example:

```bash
ski init --target dir:./agent-skills/claude
```

## Troubleshooting and Doctor

Use `ski` only with skill repositories you have verified and trust. Review the upstream repo and `SKILL.md` before `add`, `install`, or `update`.

Use `ski doctor` to check the active scope for drifted, missing, or otherwise inconsistent target installs, lockfile state, and store state.

Use `ski doctor --fix` to repair safe issues in place, including:

- missing lockfile entries recreated from the manifest
- orphaned lockfile entries removed
- stale lockfile source, upstream skill, target, or integrity fields refreshed
- missing or invalid store snapshots refetched
- missing target installs materialized
- drifted managed target directories replaced with the locked store snapshot
- unexpected managed target directories removed

`ski doctor --fix` exits non-zero if any issues still require manual intervention after the repair pass.

## Version and Release Notes

`ski version` reports the CLI build version. Development builds print `dev`.

`make release VERSION=...` writes release archives and `dist/ski_<version>_checksums.txt` for installer verification.
