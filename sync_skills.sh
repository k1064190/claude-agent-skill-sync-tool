#!/usr/bin/env bash
# ABOUTME: Interactive script to selectively sync Claude skills to ~/.claude/skills/
# ABOUTME: Arrow-key TUI with all skills pre-selected; Space to toggle, Enter to confirm

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=_tui_select.sh
source "$ROOT/_tui_select.sh"

SKILLS_SRC="$ROOT/claude/skills"
TARGET_HOME="${SYNC_TARGET_HOME:-$HOME}"
SKILLS_DEST="$TARGET_HOME/.claude/skills"

# Collect all skill directories (parent of each SKILL.md) relative to SKILLS_SRC
mapfile -t ALL_SKILLS < <(
    find "$SKILLS_SRC" -name "SKILL.md" \
        | sort \
        | sed "s|$SKILLS_SRC/||; s|/SKILL\.md$||"
)

if [[ ${#ALL_SKILLS[@]} -eq 0 ]]; then
    echo "No skills found in $SKILLS_SRC" >&2
    exit 1
fi

# Return description for a given skill item (relative path like "01-model-architecture/litgpt")
# Args:
#   $1 (str): Skill path relative to SKILLS_SRC
# Output (stdout): Description string from YAML frontmatter
get_skill_desc() {
    _tui_yaml_desc "$SKILLS_SRC/$1/SKILL.md"
}

echo "=== Claude Skill Sync ==="
echo "Source : $SKILLS_SRC"
echo "Dest   : $SKILLS_DEST"
echo ""

mapfile -t SELECTED < <(tui_checkbox_select ALL_SKILLS get_skill_desc)

if [[ ${#SELECTED[@]} -eq 0 ]]; then
    echo "No skills selected. Exiting."
    exit 0
fi

echo ""
echo "Selected ${#SELECTED[@]} skill(s):"
for s in "${SELECTED[@]}"; do
    echo "  - $s"
done

echo ""
read -r -p "Proceed with sync? [y/N]: " confirm </dev/tty
[[ "$confirm" =~ ^[yY]$ ]] || { echo "Aborted."; exit 0; }

# Clear destination so only selected skills remain
if [[ -d "$SKILLS_DEST" ]]; then
    rm -rf "$SKILLS_DEST"
fi
mkdir -p "$SKILLS_DEST"

synced=0
for skill in "${SELECTED[@]}"; do
    src="$SKILLS_SRC/$skill"
    dest="$SKILLS_DEST/$skill"
    mkdir -p "$(dirname "$dest")"
    rsync -a --itemize-changes "$src/" "$dest/"
    echo "  synced: $skill"
    synced=$(( synced + 1 ))
done

echo ""
echo "Done. Synced $synced skill(s) to $SKILLS_DEST"
