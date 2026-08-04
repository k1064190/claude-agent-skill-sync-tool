# claude-sync

Interactive CLI for selectively syncing AI agent skills, agents, rules, modular instruction policies, settings, and completion notifier programs across multiple platforms (Claude Code, Antigravity, Codex, Opencode).

## Why?

AI agents load skills, agents, and rules from specific directories (e.g., `~/.claude/`, `~/.gemini/`). As you collect many of these, issues arise:

- **Context bloat** — Unused skills consume context window space.
- **Interference** — Skills for different workflows can conflict.
- **Platform Fragmentation** — Managing different instruction formats (`CLAUDE.md`, `GEMINI.md`, `AGENTS.md`) across different tools is tedious.

`claude-sync` solves this by providing a unified source-of-truth. You pick exactly what to sync — globally or per-project — and it handles the platform-specific paths and file merging for you.

## Installation

Download the latest binary from the [releases page](https://github.com/k1064190/claude-agent-skill-sync-tool/releases) and place it on your `PATH`:

```bash
# Linux x86-64
curl -L https://github.com/k1064190/claude-agent-skill-sync-tool/releases/latest/download/claude-sync-linux-amd64 -o ~/.local/bin/claude-sync
chmod +x ~/.local/bin/claude-sync
```

```bash
# macOS Apple Silicon
curl -L https://github.com/k1064190/claude-agent-skill-sync-tool/releases/latest/download/claude-sync-darwin-arm64 -o ~/.local/bin/claude-sync
chmod +x ~/.local/bin/claude-sync
```

Make sure `~/.local/bin` is in your `PATH`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Then run from anywhere:

```bash
claude-sync
```

<details>
<summary>Build from source (requires Go 1.24+)</summary>

```bash
git clone https://github.com/k1064190/claude-agent-skill-sync-tool.git
cd claude-agent-skill-sync-tool/go
go build -o ~/.local/bin/claude-sync ./cmd/claude-sync
```

</details>

## Usage

### 1. First Run — Configuration

The tool asks for a **source root** containing your assets.

```
<source-root>/
├── skills/           # Skill directories (shared across platforms via .agents/skills)
├── agents/           # Agent .md files (platform-specific routing)
├── rules/            # Rule .md files
├── codex-agents/     # Native Codex custom-agent .toml files → ~/.codex/agents/
├── notifiers/        # Executable Claude/Codex notifier programs → ~/.local/bin/
├── templates/modules/<id>/
│   ├── module.md     # Metadata + shared system prompt
│   ├── claude.md     # Optional automatic Claude overlay
│   └── codex.md      # Optional automatic Codex overlay
├── settings/         # Settings fragments merged into each platform's settings file
└── codex-rules/      # Codex execpolicy .rules → ~/.codex/rules/ (deterministic command blocks)
```

Entries under `skills/` (and the other item directories) may be **symlinks into
cloned repositories** — see [Recommended layout](#recommended-layout-repos--symlinks)
below. Discovery follows them like regular directories.

### 2. Platform Selection

Choose one or more platforms to sync to:

```
  Which platforms would you like to sync to?

  [x] Claude
  [x] Antigravity
  [ ] Codex
  [ ] Opencode

  (Space to toggle, Enter to confirm, a to select all)
```

### 3. Scope & Item Selection

1.  **Scope**: Choose **User scope** (global) or **Project scope** (current directory).
2.  **Item Type**: Choose what to sync (`skills`, `agents`, `rules`, `codex-agents`, `notifiers`, `templates`, or `settings`).

### 4. Template Builder (Special Case)

If `templates/modules/` exists, selecting `templates` presents each module's
description and lets you assemble the desired policy. Each `module.md` contains
YAML metadata (`id`, `description`, `default`, `order`) followed by its shared
system prompt. Optional `claude.md`, `codex.md`, `gemini.md`, and `opencode.md`
files in the same module directory are automatic platform overlays.

Selections are saved to `~/.config/claude-sync/policy.toml` for user scope or
`./.claude-sync/policy.toml` for project scope. User scope builds each platform's
instruction file from the shared prompt plus its overlay. Project scope uses
only shared prompts: `AGENTS.md` is canonical, while `CLAUDE.md` is a regular
file containing only `@AGENTS.md` when Claude is selected.

Existing modular sources without a manifest are left untouched by `--status`
and `--refresh`; run the interactive template flow once to create the manifest.
Sources without `templates/modules/` continue to use the legacy
`templates/common.md` + `templates/<platform>.md` builder.

### 5. Tree Selection & Sync

For `skills`, `agents`, `rules`, `codex-agents`, and `notifiers`, use the interactive TUI to pick items. The tool will then:
- Create symlinks in the target directories.
- Use absolute paths in output for clear visibility.

## Recommended Layout: Repos → Symlinks

Keep every skill in a git repository and make the source root a pure curation
layer of symlinks. Updates then flow through the symlink chain with a single
`git pull` — no re-copying, no re-syncing:

```
~/projects/<repo>                    # clones: external collections + your own skills repo
        │  ln -s                     #   (curation: pick which skills are sync candidates)
<source-root>/skills/<name>          # symlink → repo skill
        │  claude-sync               #   (selection: pick which candidates are active)
~/.claude/skills, ~/.gemini/config/skills, ...
```

To update everything at once:

```bash
claude-sync --update
```

## Settings Fragments

Plugins and other settings cannot be symlinked across platforms — each agent
stores them differently (Claude declares plugins in `settings.json`, Antigravity
uses `plugins/<name>/plugin.json` bundles, Opencode loads `*.js`, Codex uses
TOML). What *is* portable is Claude's declaration, so `settings/` holds a
version-controlled **fragment** that claude-sync injects into the live file:

```
<source-root>/settings/claude.json   →   ~/.claude/settings.json
```

```json
{
  "enabledPlugins": {
    "superpowers@claude-plugins-official": true,
    "claude-dashboard@claude-dashboard": true
  },
  "extraKnownMarketplaces": {
    "claude-dashboard": { "source": { "source": "github", "repo": "uppinote20/claude-dashboard" } }
  }
}
```

**Ownership semantics — the fragment owns whole top-level keys:**

- Keys **present** in the fragment replace the live value wholesale. Removing a
  plugin from the fragment's `enabledPlugins` actually removes it.
- Keys **absent** from the fragment are preserved untouched — `theme`, `hooks`,
  `permissions`, and anything else Claude Code writes never churn.

The live file is backed up to `settings.json.bak` before any change, and a
run that would change nothing writes nothing.

**Codex** is also supported: `settings/codex.toml` is merged into
`~/.codex/config.toml` as a **TOML** fragment with the same ownership semantics.
The owned top-level keys (e.g. `sandbox_mode`, `approval_policy`) are set, while
every `[table]` section (per-project trust, hook-trust hashes, TUI counters) is
preserved byte-for-byte — only the file's pre-table preamble is rewritten.
Opencode and Antigravity are skipped.

Two optional companion files extend the base fragments without owning unrelated
settings: `settings/claude-hooks.json` additively appends hook groups by event,
and `settings/codex.unset` lists bare or dotted settings to remove from the live
Codex config (for example, `features.fast_mode`).

## Completion Notifiers

Executable files under `notifiers/` can be selected for Claude and Codex and are
linked into `~/.local/bin/`. The destination is user-global even when project
scope is selected. Existing commands not owned by claude-sync are never
overwritten. Point each platform at the installed command through its settings
source. A Codex fragment can use:

```toml
notify = []
hooks.Stop = [{ hooks = [{ type = "command", command = "$HOME/.local/bin/agent-notify codex", timeout = 10 }] }]
```

The root `Stop` hook runs after the main Codex turn, while omitting
`hooks.SubagentStop` prevents individual subagent completions from notifying.
Codex asks for one-time trust for a new hook definition; approve it through
`/hooks` after the first sync on each machine.

The example uses the user-global executable's absolute `$HOME` path. Subsequent
`claude-sync --refresh` runs repair an existing notifier link and reapply the
settings fragment without adding a notifier that was never selected. During the
rename migration, only the exact legacy `codex-notify` symlink previously owned
by claude-sync is removed; real commands and foreign symlinks are preserved.
Credential files are intentionally not moved between platform-specific and
shared configuration directories.

**Adding a plugin.** Install it the native way (Claude Code's `/plugin`), which
writes `enabledPlugins` in the live file. Because the fragment *owns* that key,
the next `--refresh` would otherwise remove your new plugin again — so capture it
back into the fragment first:

```bash
claude-sync --capture   # pull the live values of owned keys into the fragment
git -C <repo> commit -am "add <plugin>"
```

`--capture` only touches keys the fragment already declares, so machine-local
settings never leak into version control.

## Maintenance Commands

| Flag | Effect |
| --- | --- |
| `--status` / `-s` | Read-only drift report: linked/broken link counts per platform, `in-sync`/`stale`/`missing` per instruction file. |
| `--refresh` / `-r` | Idempotent repair: re-link existing items, remove dangling tool-owned links, rebuild existing instruction files (backing up to `<file>.bak` first). Never adds new items. |
| `--update` / `-u` | `git pull --ff-only` every repository referenced by symlinks under the source root. Combine with `--status`/`--refresh` in one invocation. |
| `--capture` / `-c` | Reverse of the settings injection: pull the live value of every fragment-owned key back into the fragment. Run after changing settings through the agent's own UI (e.g. `/plugin`). |
| `--project` / `-p` | Operate on project scope (`./`) instead of user scope (`~/`). |

## TUI Controls

| Key             | Action                                 |
| --------------- | -------------------------------------- |
| `↑` / `↓`       | Navigate up/down                       |
| `PgUp` / `PgDn` | Page up/down                           |
| `←` / `→`       | Toggle/Hide description preview        |
| `Space`         | Toggle item / Platform                 |
| `a`             | Select all                             |
| `n`             | Deselect all                           |
| `Enter`         | Confirm selection                      |
| `q`             | Cancel                                 |

## Target Path Mapping

Paths below are for **User scope**. In **Project scope**, Antigravity routes all
items under the workspace root `./.agents/` (e.g. `./.agents/skills`), matching
`agy`'s workspace discovery.

> **Known limitation:** Antigravity and Codex do not load shared Markdown persona
> `agents` — those are only consumed by Claude and Opencode. Selecting
> `agents` for Antigravity/Codex creates links the tool reports as synced but
> those platforms ignore them. Native Codex custom agents use standalone TOML
> definitions instead; put them under `codex-agents/` to sync them exclusively
> to `~/.codex/agents/` (user scope) or `./.codex/agents/` (project scope).
>
> **Known limitation:** Antigravity user-scope `rules` are synced to
> `~/.gemini/config/rules`, but `agy` documents global rules as a single
> `~/.gemini/GEMINI.md` file (only workspace rules use `.agents/rules`). Until
> that is verified against a live `agy` run, user-scope `rules` for Antigravity
> may not be loaded. Project-scope rules (`.agents/rules`) are unaffected.

| Platform | Skills Path | Agent/Rule Path | Config File |
| --- | --- | --- | --- |
| **Claude** | `~/.claude/skills` | `~/.claude/agents` | `CLAUDE.md` |
| **Antigravity** | `~/.gemini/config/skills` | `~/.gemini/config/agents` | `GEMINI.md` |
| **Codex** | `~/.agents/skills` | `~/.codex/agents` | `AGENTS.md` |
| **Opencode** | `~/.config/opencode/skills` | `~/.config/opencode/agents` | `AGENTS.md` |

## How It Works

- **`go/internal/config/`** — Platform-specific routing and multi-agent configuration.
- **`go/internal/ui/`** — Bubbletea-based platform selection checklist.
- **`go/internal/sync/`** — Handles both symlink management and the template merging engine.

## License

MIT
