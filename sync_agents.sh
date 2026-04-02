#!/usr/bin/env bash
# ABOUTME: Interactive script to selectively sync Claude agents to ~/.claude/agents/
# ABOUTME: Arrow-key TUI with all agents pre-selected; Space to toggle, Enter to confirm

set -euo pipefail

if (( BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 3) )); then
    printf 'Error: bash 4.3 or newer required (found %s)\n' "$BASH_VERSION" >&2; exit 1
fi

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

_tts_tmp=$(mktemp)
tui_tree_select ALL_AGENTS get_agent_desc >"$_tts_tmp"; _tts_rc=$?
mapfile -t SELECTED <"$_tts_tmp"; rm -f "$_tts_tmp"
(( _tts_rc == 0 )) || { echo "Selection cancelled."; exit 0; }

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

mkdir -p "$AGENTS_DEST"

# Build a lookup set of selected agents for O(1) membership test
declare -A SELECTED_SET=()
for agent in "${SELECTED[@]}"; do
    SELECTED_SET["$agent"]=1
done

linked=0
removed=0

# For every agent in this repo: symlink if selected, remove our symlink if not
for agent in "${ALL_AGENTS[@]}"; do
    src="$AGENTS_SRC/$agent"
    dest="$AGENTS_DEST/$agent"

    if [[ -n "${SELECTED_SET[$agent]+x}" ]]; then
        mkdir -p "$(dirname "$dest")"
        ln -sf "$src" "$dest"
        echo "  linked: $agent"
        linked=$(( linked + 1 ))
    else
        if [[ -L "$dest" ]]; then
            target=$(readlink "$dest" 2>/dev/null || true)
            if [[ "$target" == "$src" ]]; then
                rm -f "$dest"
                echo "  removed: $agent"
                removed=$(( removed + 1 ))
            fi
        fi
    fi
done

echo ""
echo "Done. Linked $linked, removed $removed agent(s) in $AGENTS_DEST"
