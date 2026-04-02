# claude-agent-skill-sync-tool

Interactive CLI tools for selectively syncing Claude Code agents and skills to your local environment.

## Overview

Instead of syncing all agents/skills at once, these tools let you pick exactly which ones to install via an interactive tree TUI — arrow keys to navigate, Space to toggle items or entire directories, and right arrow to preview descriptions.

```
 [36/36]  ↑↓=navigate  Space=toggle  a=all  n=none  Enter=confirm  q=cancel  →=preview

 ▶ [x] AI-Research-SKILLs/
     [x] 01-model-architecture/
       [x] litgpt
       [x] mamba
       [x] nanogpt
     [x] 02-tokenization/
       [x] huggingface-tokenizers
       [x] sentencepiece
   ...

─── description ─
 A skill for training and fine-tuning large language models
 using the LitGPT framework.
```

## Tools

| Binary | Purpose | Destination |
|--------|---------|-------------|
| `go/sync-agents` | Sync Claude agents | `~/.claude/agents/` |
| `go/sync-skills` | Sync Claude skills | `~/.claude/skills/` |

Selected items are symlinked to the destination. Deselected items that were previously synced are removed.

## Requirements

- A terminal with ANSI color support

Pre-built binaries have zero runtime dependencies. To build from source, Go 1.21+ is required.

## Setup

Place your agents and skills in the expected source directories:

```
claude-agent-skill-sync-tool/
├── go/                     # Go source and binaries
│   ├── sync-agents
│   ├── sync-skills
│   └── ...
└── claude/
    ├── agents/             # Source agents
    │   ├── agent-organizer.md
    │   ├── business/
    │   └── ...
    └── skills/             # Source skills (each subdir contains SKILL.md)
        ├── 01-model-architecture/
        │   ├── litgpt/
        │   └── ...
        └── ...
```

## Usage

```bash
# Build (one-time, or use pre-built binaries)
cd go && go build ./cmd/sync-skills && go build ./cmd/sync-agents && cd ..

# Sync skills interactively
./go/sync-skills

# Sync agents interactively
./go/sync-agents
```

Override the target home directory for testing:

```bash
SYNC_TARGET_HOME=/tmp/test ./go/sync-skills
```

### Cross-compilation

Build for any platform from any platform:

```bash
cd go

# Linux x86-64
GOOS=linux GOARCH=amd64 go build -o sync-skills-linux-amd64 ./cmd/sync-skills

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o sync-skills-darwin-arm64 ./cmd/sync-skills

# Windows x86-64
GOOS=windows GOARCH=amd64 go build -o sync-skills-windows-amd64.exe ./cmd/sync-skills
```

## TUI Controls

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate up/down |
| `Space` | Toggle current item (cascades for directories) |
| `→` | Show description preview |
| `←` | Hide description preview |
| `a` | Select all |
| `n` | Deselect all |
| `Enter` | Confirm selection |
| `q` | Cancel |

## How It Works

The Go implementation uses [bubbletea](https://github.com/charmbracelet/bubbletea) for a robust terminal UI that handles resize, scrolling, and keyboard input natively.

- **`go/cmd/sync-skills/`**: Discovers all `SKILL.md` directories under `claude/skills/`, presents a tree TUI, then symlinks selected skills to `~/.claude/skills/`.
- **`go/cmd/sync-agents/`**: Discovers all `.md` files under `claude/agents/`, presents a tree TUI, then symlinks selected agents to `~/.claude/agents/`.
- **`go/internal/tree/`**: Tree TUI model with hierarchical display, directory cascade toggle, and description preview.
- **`go/internal/sync/`**: Symlink-based sync that links selected items and removes deselected ones.

## License

MIT
