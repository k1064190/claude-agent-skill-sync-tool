# claude-sync

Interactive CLI for selectively syncing Claude Code skills, agents, commands, and rules to your environment via symlinks.

## Why?

Claude Code loads every skill, agent, command, and rule found in `~/.claude/`. When you collect many of these across different domains, problems start to appear:

- **Context bloat** — Unused skills still consume context window space, leaving less room for actual work.
- **Interference** — Skills designed for different workflows can conflict or produce unexpected behavior.
- **No per-project control** — There is no built-in way to say "this project only needs these 5 skills."

This tool solves that. You keep all your items in one source directory, then use an interactive tree TUI to pick exactly which ones to sync — either globally (user scope) or per-project (project scope). Switching between configurations takes seconds.

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
# Add to your shell profile (~/.bashrc, ~/.zshrc, etc.) if not already present
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

On first launch, the tool asks for a **source root** — the directory containing your `skills/`, `agents/`, `commands/`, and/or `rules/` folders.

```
  ___ _      _   _   _ ___  ___   _____   ___  _  ___
 / __| |    /_\ | | | |   \| __| / __\ \ / / \| |/ __|
| (__| |__ / _ \| |_| | |) | _|  \__ \\ V /| .  | (__
 \___|____/_/ \_\\___/|___/|___| |___/ |_| |_|\_|\___|

  Setting up configuration...

  Source root [/home/user/my-claude-collection]:
  >
```

The default is the current working directory. The source root should contain one or more of:

```
<source-root>/
├── skills/           # Skill directories (leaf dirs auto-discovered)
├── agents/           # Agent .md files
├── commands/         # Command .md files (become slash commands)
└── rules/            # Rule .md files
```

Configuration is saved to `~/.config/claude-sync/config.json` and reused on subsequent runs.

### 2. Scope Selection

Choose where to sync:

```
  [1] User scope    (~/.claude/)
  [2] Project scope (./.claude/)
  [3] Set configuration

  Select [1/2/3]:
```

- **User scope** — Syncs to `~/.claude/`. Available globally to Claude Code.
- **Project scope** — Syncs to `./.claude/` in the current directory. Only active when Claude Code runs in this project.
- **Set configuration** — Re-configure the source root path.

### 3. Item Type Selection

Choose what to sync. Only types with existing source directories are shown:

```
  What to sync?
  [1] skills
  [2] agents
  [3] commands
  [4] rules

  Select [1-4]:
```

### 4. Tree Selection

The interactive TUI shows all discovered items in a hierarchical tree. Items already synced to the destination are pre-checked; new items start unchecked.

```
 [12/103]  ↑↓=navigate  PgUp/PgDn=page  Space=toggle  a=all  n=none  Enter=confirm  q=cancel  →=preview

 ▶ [~] AI-Research-SKILLs/
     [ ] 01-model-architecture/
       [ ] litgpt
       [ ] nanogpt
     [x] 08-distributed-training/
       [x] deepspeed
       [x] pytorch-fsdp2
   ...

── description ──
 DeepSpeed ZeRO optimization for distributed training
```

### 5. Confirmation

After pressing Enter, review your selection and confirm:

```
Selected 12 skill(s):
  - AI-Research-SKILLs/08-distributed-training/deepspeed
  - AI-Research-SKILLs/08-distributed-training/pytorch-fsdp2
  ...

Proceed with sync? [y/N]: y

Done. Linked 12, removed 3 skills in ~/.claude/skills
```

Selected items are symlinked. Previously synced items that are now deselected are removed.

## TUI Controls

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate up/down |
| `PgUp` / `PgDn` | Page up/down |
| `Space` | Toggle item (cascades for directories) |
| `→` | Show description preview |
| `←` | Hide description preview |
| `a` | Select all |
| `n` | Deselect all |
| `Enter` | Confirm selection |
| `q` | Cancel |

## Typical Workflows

**Per-project skill sets:**
```bash
cd ~/projects/ml-research
claude-sync    # → User/Project → skills → pick ML-related skills only
```

**Switching user-wide agents:**
```bash
claude-sync    # → User scope → agents → toggle agents on/off
```

**Managing custom commands:**
```bash
claude-sync    # → Project scope → commands → pick project-specific commands
```

## Cross-Compilation

Pre-built binaries are available on the [releases page](https://github.com/k1064190/claude-agent-skill-sync-tool/releases). To build manually:

```bash
cd go

# Linux x86-64
GOOS=linux GOARCH=amd64 go build -o claude-sync-linux-amd64 ./cmd/claude-sync

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o claude-sync-darwin-arm64 ./cmd/claude-sync

# Windows x86-64
GOOS=windows GOARCH=amd64 go build -o claude-sync-windows-amd64.exe ./cmd/claude-sync
```

## How It Works

Built with [bubbletea](https://github.com/charmbracelet/bubbletea) (v1.3) for the terminal UI.

- **`go/internal/config/`** — Persistent config, scope/type selection, existing symlink detection.
- **`go/internal/tree/`** — Tree TUI with hierarchical display, cascading directory toggle, and description preview.
- **`go/internal/sync/`** — Symlink-based sync that links selected items and removes deselected ones (only removes symlinks it created).
- **`go/cmd/claude-sync/`** — Unified entry point handling all item types. Skills use leaf directory auto-detection; agents/commands/rules discover `.md` files.

## License

MIT
