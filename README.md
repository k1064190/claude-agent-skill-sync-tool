# claude-agent-skill-sync-tool

Interactive CLI for selectively syncing Claude Code agents and skills to your environment via symlinks.

## Why?

Claude Code loads every agent and skill found in `~/.claude/agents/` and `~/.claude/skills/`. When you collect many skills across different domains, problems start to appear:

- **Context bloat** — Unused skills still consume context window space, leaving less room for actual work.
- **Interference** — Skills designed for different workflows can conflict or produce unexpected behavior.
- **No per-project control** — There is no built-in way to say "this project only needs these 5 skills."

This tool solves that. You keep all your agents and skills in one source directory, then use an interactive tree TUI to pick exactly which ones to sync — either globally (user scope) or per-project (project scope). Switching between configurations takes seconds.

## Installation

Download the latest binaries from the [releases page](https://github.com/k1064190/claude-agent-skill-sync-tool/releases) and place them somewhere on your `PATH`:

```bash
# Linux x86-64 example
curl -L https://github.com/k1064190/claude-agent-skill-sync-tool/releases/latest/download/sync-skills-linux-amd64 -o ~/.local/bin/sync-skills
curl -L https://github.com/k1064190/claude-agent-skill-sync-tool/releases/latest/download/sync-agents-linux-amd64 -o ~/.local/bin/sync-agents
chmod +x ~/.local/bin/sync-skills ~/.local/bin/sync-agents
```

```bash
# macOS Apple Silicon example
curl -L https://github.com/k1064190/claude-agent-skill-sync-tool/releases/latest/download/sync-skills-darwin-arm64 -o ~/.local/bin/sync-skills
curl -L https://github.com/k1064190/claude-agent-skill-sync-tool/releases/latest/download/sync-agents-darwin-arm64 -o ~/.local/bin/sync-agents
chmod +x ~/.local/bin/sync-skills ~/.local/bin/sync-agents
```

Make sure `~/.local/bin` is in your `PATH`:
```bash
# Add to your shell profile (~/.bashrc, ~/.zshrc, etc.) if not already present
export PATH="$HOME/.local/bin:$PATH"
```

Then run from anywhere:
```bash
sync-skills
sync-agents
```

<details>
<summary>Build from source (requires Go 1.24+)</summary>

```bash
git clone https://github.com/k1064190/claude-agent-skill-sync-tool.git
cd claude-agent-skill-sync-tool/go
go build -o ~/.local/bin/sync-skills ./cmd/sync-skills
go build -o ~/.local/bin/sync-agents ./cmd/sync-agents
```
</details>

## Usage

### 1. First Run — Configuration

On first launch, the tool asks for a **source root** — the directory containing your `skills/` and `agents/` folders.

```
  ___ _      _   _   _ ___  ___   _____   ___  _  ___
 / __| |    /_\ | | | |   \| __| / __\ \ / / \| |/ __|
| (__| |__ / _ \| |_| | |) | _|  \__ \\ V /| .  | (__
 \___|____/_/ \_\\___/|___/|___| |___/ |_| |_|\_|\___|

  Setting up configuration...

  Source root [/home/user/my-skills-repo]:
  >
```

The default is the current working directory. The source root should contain:

```
<source-root>/
├── skills/
│   ├── AI-Research-SKILLs/
│   │   ├── 01-model-architecture/
│   │   │   ├── litgpt/
│   │   │   └── ...
│   │   └── ...
│   └── Agent-Skills-for-Context-Engineering/
│       └── skills/
│           └── ...
└── agents/
    ├── agent-organizer.md
    ├── business/
    │   └── pm.md
    └── ...
```

Configuration is saved to `~/.config/claude-sync/config.json` and reused on subsequent runs.

### 2. Scope Selection

After configuration, choose where to sync:

```
  [1] User scope    (~/.claude/)
  [2] Project scope (./.claude/)
  [3] Set configuration

  Select [1/2/3]:
```

- **User scope** — Symlinks to `~/.claude/skills/` or `~/.claude/agents/`. Available globally to Claude Code.
- **Project scope** — Symlinks to `./.claude/skills/` or `./.claude/agents/` in the current directory. Only active when Claude Code runs in this project.
- **Set configuration** — Re-configure the source root path.

### 3. Tree Selection

The interactive TUI shows all discovered items in a hierarchical tree. Items already synced to the destination are pre-checked.

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

### 4. Confirmation

After pressing Enter, the tool shows your selection and asks for confirmation before applying changes.

```
Selected 12 skill(s):
  - AI-Research-SKILLs/08-distributed-training/deepspeed
  - AI-Research-SKILLs/08-distributed-training/pytorch-fsdp2
  ...

Proceed with sync? [y/N]: y

Done. Linked 12, removed 3 skill(s) in ~/.claude/skills
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
sync-skills          # Select [2] Project scope → pick ML-related skills only
```

**Switching user-wide skills:**
```bash
sync-skills          # Select [1] User scope → toggle skills on/off as needed
```

**Working with agents:**
```bash
sync-agents          # Same workflow, for agent .md files
```

## Cross-Compilation

Pre-built binaries are available on the [releases page](https://github.com/k1064190/claude-agent-skill-sync-tool/releases). To build manually:

```bash
cd go

# Linux x86-64
GOOS=linux GOARCH=amd64 go build -o sync-skills-linux-amd64 ./cmd/sync-skills

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o sync-skills-darwin-arm64 ./cmd/sync-skills

# Windows x86-64
GOOS=windows GOARCH=amd64 go build -o sync-skills-windows-amd64.exe ./cmd/sync-skills
```

## How It Works

Built with [bubbletea](https://github.com/charmbracelet/bubbletea) (v1.3) for the terminal UI.

- **`go/internal/config/`** — Persistent config (`~/.config/claude-sync/config.json`), scope selection, and existing symlink detection.
- **`go/internal/tree/`** — Tree TUI with hierarchical display, cascading directory toggle, and description preview.
- **`go/internal/sync/`** — Symlink-based sync that links selected items and removes deselected ones (only removes symlinks it created).
- **`go/cmd/sync-skills/`** — Discovers leaf skill directories (auto-detection, no marker files needed).
- **`go/cmd/sync-agents/`** — Discovers `.md` agent files.

## License

MIT
