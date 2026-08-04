# Modular Agent Policy Implementation Plan

> **For agentic workers:** Execute inline in this session. Do not dispatch review or implementation subagents.

**Goal:** Replace fixed policy concatenation with selectable, persistent modules and finish the previously approved agent, notifier, and settings parity work.

**Architecture:** The sync tool owns module parsing, manifest persistence, deterministic rendering, and safe settings/hook merges. `my-skills` owns the actual prompt modules, native agent definitions, notifier program, and portable settings fragments. Legacy template sources remain readable, while modular sources require a manifest before non-interactive refresh can write output.

**Tech Stack:** Go 1.24, Bubble Tea, Python standard library, Markdown with YAML frontmatter, minimal TOML manifests.

## Global Constraints

- Project output is common-only `AGENTS.md`; `CLAUDE.md` is a regular file containing only `@AGENTS.md`.
- User output shares one module selection across platforms and appends only the current platform overlay.
- Default modules are `interaction`, `implementation-discipline`, and `git-workflow`.
- Automatic code-review agents and mandatory stage review are removed.
- No credential migration, external notification delivery, push, or PR creation occurs during implementation.

---

### Task 1: Policy module and manifest core

**Files:**
- Create: `go/internal/sync/policy_module.go`
- Create: `go/internal/sync/policy_module_test.go`
- Create: `go/internal/sync/policy_manifest.go`
- Create: `go/internal/sync/policy_manifest_test.go`
- Modify: `go/internal/sync/builder.go`
- Modify: `go/internal/sync/builder_test.go`

**Interfaces:**
- `LoadPolicyModules(srcDir string) ([]PolicyModule, error)` validates metadata and loads optional overlays.
- `BuildPolicyContent(modules []PolicyModule, selected []string, platform config.Platform, includeOverlay bool) (string, error)` renders deterministic Markdown.
- `ReadPolicyManifest(path string) (PolicyManifest, bool, error)` and `WritePolicyManifest(path string, manifest PolicyManifest) error` persist version 1 manifests atomically.

- [x] Write parsing, ordering, selection, overlay, malformed-input, and manifest round-trip tests.
- [x] Run `cd go && go test ./internal/sync` and confirm the new tests fail before implementation.
- [x] Implement the minimal module/manifest core and retain legacy builder fallback.
- [x] Re-run `cd go && go test ./internal/sync` and confirm it passes.

### Task 2: Interactive, status, refresh, and project output

**Files:**
- Modify: `go/cmd/claude-sync/main.go`
- Modify: `go/cmd/claude-sync/main_test.go`
- Modify: `go/internal/config/config.go`
- Modify: `go/internal/config/config_test.go`

**Interfaces:**
- `policyManifestPath(scope config.Scope) (string, error)` resolves user or project state.
- `writeProjectPolicyOutputs(...)` writes canonical `AGENTS.md` and platform compatibility files safely.
- Status reports `unconfigured` when modular sources have no manifest; refresh skips them without writing.

- [x] Add failing tests for manifest paths, default selection, canonical project output, and unconfigured refresh behavior.
- [x] Implement module TUI selection with description preview and manifest persistence after confirmation.
- [x] Implement user/platform rendering and project canonical rendering.
- [x] Run `cd go && go test ./...`.

### Task 3: Shared notifier and safe settings extensions

**Files:**
- Modify: `go/internal/config/config.go`
- Modify: `go/internal/config/platform.go`
- Modify: `go/internal/config/platform_test.go`
- Modify: `go/cmd/claude-sync/main.go`
- Modify: `go/cmd/claude-sync/main_test.go`
- Modify: `go/internal/sync/settings.go`
- Modify: `go/internal/sync/settings_test.go`
- Modify: `go/internal/sync/settings_toml.go`
- Modify: `go/internal/sync/settings_toml_test.go`

**Interfaces:**
- Shared `notifiers` items target `~/.local/bin` for Claude and Codex.
- `ApplySettings` removes exact keys declared by `settings/<platform>.unset`.
- `ApplySettings` additively merges exact handlers from `settings/claude-hooks.json` without replacing unrelated live hooks.

- [x] Add failing routing, legacy-notifier migration, TOML unset, and additive Claude hook tests.
- [x] Implement minimal safe merge and migration behavior.
- [x] Run `cd go && go test ./...`.

### Task 4: Portable module catalog and native agents

**Files:**
- Replace: `/home/cwh/projects/my-skills/templates/common.md`
- Replace: `/home/cwh/projects/my-skills/templates/{claude,codex}.md`
- Create: `/home/cwh/projects/my-skills/templates/modules/*/module.md`
- Create: `/home/cwh/projects/my-skills/templates/modules/*/{claude,codex}.md` where required
- Modify: `/home/cwh/projects/my-skills/agents/quality-testing/code-reviewer.md`
- Create: `/home/cwh/projects/my-skills/codex-agents/*.toml`
- Delete: `/home/cwh/projects/my-skills/codex-agents/adversarial-code-reviewer.toml`

- [x] Split the existing common rules into ten approved modules with only three defaults.
- [x] Remove mandatory review language and make `code-reviewer-pro` explicitly requested only.
- [x] Add the twelve approved role-equivalent Codex agents; document the three MCP-dependent exclusions.
- [x] Add contract tests for metadata, defaults, review removal, and agent parity.

### Task 5: Portable notifier and settings data

**Files:**
- Move: `/home/cwh/projects/my-skills/codex-notifiers/codex-notify` to `/home/cwh/projects/my-skills/notifiers/agent-notify`
- Modify: `/home/cwh/projects/my-skills/tests/test_codex_notify.py`
- Modify: `/home/cwh/projects/my-skills/settings/codex.toml`
- Create: `/home/cwh/projects/my-skills/settings/codex.unset`
- Create: `/home/cwh/projects/my-skills/settings/claude-hooks.json`
- Modify: both repositories' `README.md` and stage summaries.

- [x] Write failing tests for platform-labelled root Stop messages and the neutral credential directory.
- [x] Implement the standard-library notifier and settings fragments.
- [x] Run the my-skills test suite in its project environment.

### Task 6: End-to-end verification and local commits

- [x] Run `gofmt` on changed Go files and `cd go && go test ./...`.
- [x] Run every my-skills test and validate every TOML/JSON/Markdown contract.
- [x] Build `claude-sync`, exercise status plus temporary user/project module builds without touching real credentials, and run `claude-sync --refresh` only after source and binary checks pass.
- [x] Record concise stage outcomes in both repositories.
- [x] Commit each repository locally on `feat/modular-policy-assembly`; do not push or create a PR.
