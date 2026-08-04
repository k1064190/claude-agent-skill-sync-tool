# Modular policy assembly implementation

## Why

Fixed templates and Codex-only notifier/settings paths could not support lightweight per-project policies or shared Claude/Codex completion behavior.

## What

- Added validated policy-module discovery, deterministic assembly, and atomic user/project manifests.
- Added interactive module selection, read-only unconfigured status, safe refresh, canonical project `AGENTS.md`, and regular `CLAUDE.md` references.
- Added shared Claude/Codex notifier routing and exact legacy-link migration.
- Added additive Claude hook merging and explicit Codex TOML key removal.

## How

The CLI keeps the legacy template builder as a fallback. Modular sources require a manifest before noninteractive writes. Settings apply the owned base fragment first, then additive/removal companions, and the notifier migration only removes the exact old symlink target.

## Code locations

- `go/internal/sync/policy_module.go`
- `go/internal/sync/policy_manifest.go`
- `go/internal/sync/settings_aux.go`
- `go/internal/sync/settings_toml.go`
- `go/cmd/claude-sync/main.go`
- `go/internal/config/`

## Review outcome

Doctor Cho explicitly waived an independent code review for this implementation. The authored diff was self-checked, the complete Go suite passed with isolated `TMPDIR`, and two isolated refreshes verified migration plus idempotence against the real my-skills sources.

## Retrospective

Separate manifests make selection reproducible without guessing during refresh, and auxiliary settings sources add narrow behavior without widening ownership of user configuration.
