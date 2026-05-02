# claude-sync

Interactive CLI for selectively syncing AI agent skills, agents, and rules across multiple platforms (Claude Code, Gemini CLI, Codex, Opencode) via symlinks.

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
└── templates/        # Configuration templates (common.md + platform.md)
```

### 2. Platform Selection

Choose one or more platforms to sync to:

```
  Which platforms would you like to sync to?

  [x] Claude
  [x] Gemini
  [ ] Codex
  [ ] Opencode

  (Space to toggle, Enter to confirm, a to select all)
```

### 3. Scope & Item Selection

1.  **Scope**: Choose **User scope** (global) or **Project scope** (current directory).
2.  **Item Type**: Choose what to sync (`skills`, `agents`, `rules`, or `templates`).

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

## TUI Controls

| Key             | Action                                 |
| --------------- | -------------------------------------- |
| `↑` / `↓`       | Navigate up/down                       |
| `Space`         | Toggle item / Platform                 |
| `a`             | Select all                             |
| `n`             | Deselect all                           |
| `Enter`         | Confirm selection                      |
| `q`             | Cancel                                 |

## Target Path Mapping

| Platform | Skills Path | Agent/Rule Path | Config File |
| --- | --- | --- | --- |
| **Claude** | `~/.claude/skills` | `~/.claude/agents` | `CLAUDE.md` |
| **Gemini** | `~/.agents/skills` | `~/.gemini/agents` | `GEMINI.md` |
| **Codex** | `~/.agents/skills` | `~/.codex/agents` | `AGENTS.md` |
| **Opencode** | N/A | `~/.config/opencode/agents` | `AGENTS.md` |

## How It Works

- **`go/internal/config/`** — Platform-specific routing and multi-agent configuration.
- **`go/internal/ui/`** — Bubbletea-based platform selection checklist.
- **`go/internal/sync/`** — Handles both symlink management and the template merging engine.

## License

MIT
