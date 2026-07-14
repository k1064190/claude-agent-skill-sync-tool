# Changelog

All notable changes to this project will be documented in this file.

## [0.8.0] - 2026-07-14

### Added

- `settings/` item type — version-controlled JSON fragments injected into a
  platform's live settings file. Motivated by Claude plugins, whose only
  portable form is the `enabledPlugins` / `extraKnownMarketplaces` declaration
  in `settings.json` (plugin *artifacts* are not shareable: Antigravity uses
  directory bundles, Opencode `.js` files, Codex TOML).
- Ownership semantics: a fragment owns whole top-level keys — present keys
  replace the live value wholesale (so removals propagate), absent keys are
  preserved untouched (so machine-local settings never churn).
- `--status` reports `applied` / `pending` / `missing`; `--refresh` injects,
  backing the live file up to `<file>.bak` first and writing nothing when
  already in sync. Only Claude has a managed settings file today.
- `--capture` / `-c` — the reverse direction: pull the live value of every
  fragment-owned key back into the fragment. Needed because the fragment owns
  `enabledPlugins` wholesale, so a plugin added through Claude Code's `/plugin`
  would otherwise be removed again by the next `--refresh`. Capture only touches
  keys the fragment already declares, so machine-local settings never leak into
  version control.

### Security

- Settings writes are atomic (temp file + rename) with a compare-and-swap
  re-read, so a concurrent write by the agent is folded in rather than clobbered
  and a crash cannot leave a truncated settings file. Symlinked settings files
  are written through to their target, the live file's permission bits are
  preserved, and stale permissive `.bak` files are replaced rather than
  truncated.

## [0.7.0] - 2026-07-11

### Fixed

- `agents` / `rules` discovery now follows symlinked directories, including the
  item-type directory itself. `filepath.Walk` does not follow symlinks, so a
  source root whose `agents/` was a link into an external repo previously
  discovered zero items. Skills already worked; this brings `.md` items to
  parity so every item type can be curated as symlinks into cloned repos.
  Symlink cycles are guarded the same way as in skill discovery.

## [0.6.0] - 2026-07-10

### Added

- Skill discovery follows **symlinked directories**, so the source root can be a
  pure curation layer of links into cloned skill repositories
- `claude-sync --update` / `-u` — `git pull --ff-only` every repository
  referenced by symlinks under the source root; composes with
  `--status`/`--refresh` in one invocation
- README: recommended repos → symlinks layout and a Maintenance Commands table
  (documents `--status`/`--refresh` introduced in 0.5.0)

## [0.5.0] - 2026-07-07

### Added

- First-class **Antigravity** (`agy`) platform, routing to agy's real discovery
  locations: user scope `~/.gemini/config/<type>` and project scope `./.agents/<type>`
- `claude-sync --status` — non-interactive per-platform drift report (linked/broken
  counts for items; `in-sync`/`stale`/`missing` for templates); read-only
- `claude-sync --refresh` — idempotent, non-interactive re-link of existing items
  and rebuild of existing instruction files; preserves prior selections
- `claude-sync --project` / `-p` — select project scope for `--status`/`--refresh`
- `BuildTemplateContent` / `TemplateTargetName` helpers for pure template building

### Changed

- Instruction file build now shares a pure content builder between write and compare paths

### Removed

- `Gemini` platform (superseded by Antigravity; Gemini CLI's free tier was retired)

## [0.1.1.0] - 2026-03-28

### Added

- `tui_tree_select()` — hierarchical tree TUI for slash-delimited paths; dir nodes show
  `[x]`/`[ ]`/`[~]` computed state; Space on a dir cascades to all descendants
- `_tui_tree_build()` at module level — parses input items into parallel display arrays
  (`_TTB_LABEL`, `_TTB_TYPE`, `_TTB_INDENT`, `_TTB_ORIG`, `_TTB_SEL`, `_TTB_CACHE`);
  directly testable without a TTY
- EXIT/INT/TERM trap in `tui_tree_select` restores cursor and terminal state on signal exit
- Bash 4.3+ version guard in both sync scripts with a clear error message
- `tests/_tui_tree_select.bats` — 12 bats tests covering build logic, cascade toggle,
  output format, and edge cases against the real source functions
- `claude/agents/` and `claude/skills/` directory structure tracked in git via `.gitkeep`
  files and `.gitignore` rules; actual agent/skill content excluded

### Changed

- `sync_agents.sh` and `sync_skills.sh`: replaced `tui_checkbox_select` call with
  `tui_tree_select` for hierarchical selection
- Sync behavior: replaced `rm -rf` + `cp`/`rsync` with surgical symlink management —
  selected items become symlinks into the repo; deselected items have their symlinks
  removed only if they point to this repo; foreign symlinks and plain files are untouched
- Symlink removal uses plain `readlink` (not `readlink -f`) — correctly handles dangling
  symlinks and works on macOS
- Selection result captured via temp file to surface `tui_tree_select` exit code;
  crashes are reported as errors rather than producing a silent partial sync

## [0.1.0.0] - 2026-03-28

### Added

- Initial release: `tui_checkbox_select` flat TUI, `sync_agents.sh`, `sync_skills.sh`
