# claude-sync

Interactive CLI for selectively syncing AI agent skills, agents, rules, instruction templates, and settings across multiple platforms (Claude Code, Antigravity, Codex, Opencode).

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
├── templates/        # Configuration templates (common.md + platform.md)
└── settings/         # Settings fragments merged into each platform's settings file
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
2.  **Item Type**: Choose what to sync (`skills`, `agents`, `rules`, `templates`, or `settings`).

### 4. Template Builder (Special Case)

If you select `templates`, `claude-sync` automatically merges:
- `templates/common.md` (shared instructions)
- `templates/<platform>.md` (platform-specific instructions)

It then writes the merged file to the correct target (e.g., `CLAUDE.md`, `GEMINI.md`, or `AGENTS.md`).

**Cross-Platform Compatibility (Project Scope):**
In Project Scope, if only one instruction file is generated, `claude-sync` automatically creates symlinks for the others (e.g., `AGENTS.md -> CLAUDE.md`) so all tools can see the same rules.

### 5. Tree Selection & Sync

For `skills`, `agents`, and `rules`, use the interactive TUI to pick items. The tool will then:
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
run that would change nothing writes nothing. Only Claude has a managed settings
file today; other platforms are skipped.

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

> **Known limitation:** Antigravity and Codex do not load Markdown persona
> `agents` — those are only consumed by Claude and Opencode. Selecting
> `agents` for Antigravity/Codex creates links the tool reports as synced but
> those platforms ignore them.
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
