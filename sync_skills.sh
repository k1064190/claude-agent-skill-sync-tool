#!/usr/bin/env bash
# ABOUTME: Interactive script to selectively sync Claude skills to ~/.claude/skills/
# ABOUTME: Arrow-key TUI with all skills pre-selected; Space to toggle, Enter to confirm

set -euo pipefail

if (( BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 3) )); then
    printf 'Error: bash 4.3 or newer required (found %s)\n' "$BASH_VERSION" >&2; exit 1
fi

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

_tts_tmp=$(mktemp)
tui_tree_select ALL_SKILLS get_skill_desc >"$_tts_tmp"; _tts_rc=$?
mapfile -t SELECTED <"$_tts_tmp"; rm -f "$_tts_tmp"
(( _tts_rc == 0 )) || { echo "Selection cancelled."; exit 0; }

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

mkdir -p "$SKILLS_DEST"

# Build a lookup set of selected skills for O(1) membership test
declare -A SELECTED_SET=()
for skill in "${SELECTED[@]}"; do
    SELECTED_SET["$skill"]=1
done

linked=0
removed=0

# For every skill in this repo: symlink if selected, remove our symlink if not
for skill in "${ALL_SKILLS[@]}"; do
    src="$SKILLS_SRC/$skill"
    dest="$SKILLS_DEST/$skill"

    if [[ -n "${SELECTED_SET[$skill]+x}" ]]; then
        mkdir -p "$(dirname "$dest")"
        ln -sf "$src" "$dest"
        echo "  linked: $skill"
        linked=$(( linked + 1 ))
    else
        if [[ -L "$dest" ]]; then
            target=$(readlink "$dest" 2>/dev/null || true)
            if [[ "$target" == "$src" ]]; then
                rm -f "$dest"
                echo "  removed: $skill"
                removed=$(( removed + 1 ))
            fi
        fi
    fi
done

echo ""
echo "Done. Linked $linked, removed $removed skill(s) in $SKILLS_DEST"
