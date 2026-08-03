# Modular agent policy design

## Why

The fixed `common.md + <platform>.md` template forces testing, staged documentation, and review rules into every environment. User and project scope need reproducible, independently selectable policy features instead.

## What

- Discover policy modules from `templates/modules/<id>/module.md`; YAML frontmatter supplies `id`, `description`, `default`, and `order`, while the Markdown body is the shared prompt.
- Apply optional `claude.md` and `codex.md` overlays only in user scope.
- Persist one shared module list in `~/.config/claude-sync/policy.toml` for user scope or `.claude-sync/policy.toml` for project scope.
- Build project policy into canonical `AGENTS.md`; write project `CLAUDE.md` as the regular file `@AGENTS.md`.
- Keep legacy template sources supported, but do not overwrite modular-template destinations until a manifest exists.
- Generalize the notifier for Claude and Codex root `Stop` hooks and remove managed Fast-mode keys.

## How

The module loader validates unique IDs and deterministic ordering. A small manifest codec owns only `version` and the ordered module ID list. Interactive template selection writes the manifest after confirmation; status and refresh read it without inventing selections. Claude hooks use a separate additive hook fragment so machine-local hooks are preserved. Codex key removals use an explicit unset list rather than changing the existing fragment ownership contract.

## Code locations

- `go/internal/sync/` — module, manifest, build, settings, and hook primitives.
- `go/cmd/claude-sync/main.go` — interactive selection, status, refresh, and migration wiring.
- `go/internal/config/` — shared notifier routing and manifest paths.
- `README.md` — module layout and scope behavior.

## Retrospective

Keeping selection in a manifest makes refresh deterministic; keeping project output common-only preserves one policy source for Claude and Codex.
