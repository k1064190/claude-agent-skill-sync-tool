#!/usr/bin/env bash
# ABOUTME: Interactive script to selectively sync Claude agents to ~/.claude/agents/
# ABOUTME: Arrow-key TUI with all agents pre-selected; Space to toggle, Enter to confirm

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_tui_select.sh
source "$ROOT/_tui_select.sh"

AGENTS_SRC="$ROOT/claude/agents"
TARGET_HOME="${SYNC_TARGET_HOME:-$HOME}"
AGENTS_DEST="$TARGET_HOME/.claude/agents"

# Collect all agent .md files relative to AGENTS_SRC, sorted
mapfile -t ALL_AGENTS < <(find "$AGENTS_SRC" -name "*.md" | sort | sed "s|$AGENTS_SRC/||")

if [[ ${#ALL_AGENTS[@]} -eq 0 ]]; then
    echo "No agents found in $AGENTS_SRC" >&2
    exit 1
fi

# Return description for a given agent item (relative path like "business/product-manager.md")
# Args:
#   $1 (str): Agent path relative to AGENTS_SRC
# Output (stdout): Description string from YAML frontmatter
get_agent_desc() {
    _tui_yaml_desc "$AGENTS_SRC/$1"
}

echo "=== Claude Agent Sync ==="
echo "Source : $AGENTS_SRC"
echo "Dest   : $AGENTS_DEST"
echo ""

mapfile -t SELECTED < <(tui_checkbox_select ALL_AGENTS get_agent_desc)

if [[ ${#SELECTED[@]} -eq 0 ]]; then
    echo "No agents selected. Exiting."
    exit 0
fi

echo ""
echo "Selected ${#SELECTED[@]} agent(s):"
for a in "${SELECTED[@]}"; do
    echo "  - $a"
done

echo ""
read -r -p "Proceed with sync? [y/N]: " confirm </dev/tty
[[ "$confirm" =~ ^[yY]$ ]] || { echo "Aborted."; exit 0; }

# Clear destination so only selected agents remain
if [[ -d "$AGENTS_DEST" ]]; then
    rm -rf "$AGENTS_DEST"
fi
mkdir -p "$AGENTS_DEST"

synced=0
for agent in "${SELECTED[@]}"; do
    src="$AGENTS_SRC/$agent"
    dest_dir="$AGENTS_DEST/$(dirname "$agent")"
    mkdir -p "$dest_dir"
    cp "$src" "$dest_dir/"
    echo "  synced: $agent"
    synced=$(( synced + 1 ))
done

echo ""
echo "Done. Synced $synced agent(s) to $AGENTS_DEST"
