# claude-agent-skill-sync-tool

Interactive CLI tools for selectively syncing Claude Code agents and skills to your local environment.

## Overview

Instead of syncing all agents/skills at once, these tools let you pick exactly which ones to install via an interactive terminal UI — arrow keys to navigate, Space to toggle, and right arrow to preview descriptions.

```
 [36/36]  ↑↓=navigate  Space=toggle  a=all  n=none  Enter=confirm  q=cancel  →=preview

 ▶ [x] agent-organizer.md
   [x] business/product-manager.md
   [x] data-ai/ai-engineer.md
   ...

─── description ─
 A highly advanced AI agent that functions as a master orchestrator for
 complex, multi-agent tasks. Analyzes project requirements, defines a team
 of specialized AI agents, and manages their collaborative workflow.
```

## Tools

| Script | Purpose | Destination |
|--------|---------|-------------|
| `sync_agents.sh` | Sync Claude agents | `~/.claude/agents/` |
| `sync_skills.sh` | Sync Claude skills | `~/.claude/skills/` |

Both scripts clear the destination before syncing so only your selected items remain.

## Requirements

- bash 4.3+
- `rsync` (for `sync_skills.sh`)
- A terminal with ANSI color support

## Setup

Place your agents and skills in the expected source directories relative to the scripts:

```
claude-agent-skill-sync-tool/
├── _tui_select.sh          # Shared TUI helper (sourced by both scripts)
├── sync_agents.sh
├── sync_skills.sh
└── claude/
    ├── agents/             # Source agents
    │   ├── agent-organizer.md
    │   ├── business/
    │   ├── data-ai/
    │   ├── development/
    │   └── ...
    └── skills/             # Source skills (each subdir contains SKILL.md)
        ├── 01-model-architecture/
        │   ├── litgpt/
        │   └── ...
        └── ...
```

## Usage

```bash
# Sync agents interactively
./sync_agents.sh

# Sync skills interactively
./sync_skills.sh
```

Override the target home directory for testing:

```bash
SYNC_TARGET_HOME=/tmp/test ./sync_agents.sh
```

## TUI Controls

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate up/down |
| `Space` | Toggle current item |
| `→` | Show description preview |
| `←` | Hide description preview |
| `a` | Select all |
| `n` | Deselect all |
| `Enter` | Confirm selection |
| `q` | Cancel |

## How It Works

- **`_tui_select.sh`**: Shared library providing `tui_checkbox_select` — a pure-bash interactive checkbox TUI using ANSI escape codes. All items are pre-selected by default. Supports an optional description callback for right-arrow previews.
- **`sync_agents.sh`**: Discovers all `.md` files under `claude/agents/`, shows the TUI, then copies selected files to `~/.claude/agents/` (clearing first).
- **`sync_skills.sh`**: Discovers all `SKILL.md` directories under `claude/skills/`, shows the TUI, then rsyncs selected skill directories to `~/.claude/skills/` (clearing first).

## License

MIT
