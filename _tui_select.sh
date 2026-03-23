#!/usr/bin/env bash
# ABOUTME: Shared TUI checkbox selector used by sync scripts
# ABOUTME: Arrow keys to navigate, Space to toggle, Right arrow for description preview

# _tui_yaml_desc - Extract description field from YAML frontmatter (between first ---)
# Args:
#   $1 (str): Path to the markdown file
# Output (stdout): Description string, or "(no description)" if not found
_tui_yaml_desc() {
    local file="$1"
    [[ -f "$file" ]] || { echo "(file not found)"; return; }
    awk '/^---$/{c++; next} c==1 && /^description:/{sub(/^description:[[:space:]]*/,""); print; exit}' "$file"
}

# _tui_fold - Word-wrap text to a given width (uses fold if available)
# Args:
#   $1 (str): Text to wrap
#   $2 (int): Column width
# Output (stdout): Wrapped text
_tui_fold() {
    local text="$1" width="$2"
    if command -v fold >/dev/null 2>&1; then
        # printf '%s\n' ensures trailing newline so fold always terminates
        # all output lines with \n, preventing while-read from missing the last line
        printf '%s\n' "$text" | fold -s -w "$width"
    else
        while [[ -n "$text" ]]; do
            printf '%s\n' "${text:0:$width}"
            text="${text:$width}"
        done
    fi
}

# tui_checkbox_select - Interactive checkbox TUI with arrow key navigation
# Args:
#   $1 (str): Name of bash array variable containing items to display
#   $2 (str, optional): Name of function that, given an item string, outputs its description.
#                       Enables right-arrow description preview panel when provided.
# Output (stdout): Selected item strings, one per line
# Returns: 0 on confirm, 1 on cancel
tui_checkbox_select() {
    local -n _ti="$1"
    local _desc_fn="${2:-}"
    local _n="${#_ti[@]}"
    local _tty="/dev/tty"

    [[ $_n -eq 0 ]] && return 1

    # Terminal dimensions
    local _rows _cols
    _rows=$(tput lines 2>/dev/null || echo 24)
    _cols=$(tput cols 2>/dev/null || echo 80)

    # Preview panel is always reserved when _desc_fn is provided (5 fixed lines)
    local _prev_height=0
    [[ -n "$_desc_fn" ]] && _prev_height=5

    local _vis=$(( _rows - 4 - _prev_height ))
    (( _vis > _n )) && _vis=$_n
    (( _vis < 3  )) && _vis=3
    local _total=$(( _vis + 3 + _prev_height ))

    # State: all pre-selected
    local _cur=0 _scr=0 _ok=0 _preview=0
    local -a _sel=()
    for (( i=0; i<_n; i++ )); do _sel+=( 1 ); done

    # ANSI codes
    local R=$'\033[0m' REV=$'\033[7m' GRN=$'\033[32m' YLW=$'\033[33m' CYN=$'\033[36m' BD=$'\033[1m' DIM=$'\033[2m'
    local CL=$'\033[2K\r'

    # Count selected items
    _tui_cnt() {
        local c=0; for v in "${_sel[@]}"; do c=$(( c + v )); done; echo "$c"
    }

    # Draw the preview panel area (_prev_height lines)
    _tui_draw_preview() {
        [[ $_prev_height -eq 0 ]] && return

        if (( _preview )); then
            local _desc=""
            [[ -n "$_desc_fn" ]] && _desc=$("$_desc_fn" "${_ti[$_cur]}" 2>/dev/null)
            [[ -z "$_desc" ]] && _desc="(no description)"

            local _pw=$(( _cols - 3 ))
            printf "${CL}${BD}${CYN}─── description ─${R}\n" >"$_tty"

            # Read wrapped lines into array
            local -a _wlines=()
            while IFS= read -r _wl; do _wlines+=("$_wl"); done < <(_tui_fold "$_desc" "$_pw")

            # Show up to 3 lines, padding the rest
            for (( i=0; i<3; i++ )); do
                if (( i < ${#_wlines[@]} )); then
                    printf "${CL} %s\n" "${_wlines[$i]}" >"$_tty"
                else
                    printf "${CL}\n" >"$_tty"
                fi
            done
            printf "${CL}\n" >"$_tty"
        else
            # Hint line when preview is hidden
            printf "${CL}  ${DIM}→ right arrow to preview description${R}\n" >"$_tty"
            for (( i=1; i<_prev_height; i++ )); do printf "${CL}\n" >"$_tty"; done
        fi
    }

    # Full redraw of the entire TUI area
    _tui_draw() {
        printf '\033[%dA' "$_total" >"$_tty"

        # Header / help line
        local _hint=""
        [[ -n "$_desc_fn" ]] && _hint="  →=preview"
        printf "${CL}${BD}${CYN} [%d/%d]  ↑↓=navigate  Space=toggle  a=all  n=none  Enter=confirm  q=cancel%s${R}\n" \
            "$(_tui_cnt)" "$_n" "$_hint" >"$_tty"

        # Scroll up indicator
        if (( _scr > 0 )); then
            printf "${CL}  ${YLW}↑ %d more above${R}\n" "$_scr" >"$_tty"
        else
            printf "${CL}\n" >"$_tty"
        fi

        local _end=$(( _scr + _vis ))
        (( _end > _n )) && _end=$_n

        for (( i=_scr; i<_end; i++ )); do
            local _m="[ ]" _c=""
            if (( _sel[i] )); then
                _m="[x]"
                _c="$GRN"
            fi
            if (( i == _cur )); then
                printf "${CL}${REV} ▶ %s %-$(( _cols - 9 ))s ${R}\n" "$_m" "${_ti[$i]}" >"$_tty"
            else
                printf "${CL}   ${_c}%s${R} %s\n" "$_m" "${_ti[$i]}" >"$_tty"
            fi
        done

        # Pad unused visible lines
        for (( i=_end; i<_scr+_vis; i++ )); do printf "${CL}\n" >"$_tty"; done

        # Scroll down indicator
        if (( _scr + _vis < _n )); then
            printf "${CL}  ${YLW}↓ %d more below${R}\n" "$(( _n - _scr - _vis ))" >"$_tty"
        else
            printf "${CL}\n" >"$_tty"
        fi

        _tui_draw_preview
    }

    # Read one keypress, handling escape sequences for arrow keys
    # Output (stdout): key identifier string
    _tui_key() {
        local k seq
        IFS= read -rsn1 k <"$_tty"
        if [[ "$k" == $'\033' ]]; then
            IFS= read -rsn2 -t 0.1 seq <"$_tty" || true
            k="${k}${seq}"
        fi
        printf '%s' "$k"
    }

    # Save terminal state and hide cursor
    local _stty; _stty=$(stty -g <"$_tty" 2>/dev/null) || _stty=""
    printf '\033[?25l' >"$_tty"

    # Reserve space on screen, then initial draw
    for (( i=0; i<_total; i++ )); do printf '\n' >"$_tty"; done
    _tui_draw

    while true; do
        local _k; _k=$(_tui_key)
        case "$_k" in
            $'\033[A'|k)  # up arrow
                if (( _cur > 0 )); then
                    _cur=$(( _cur - 1 ))
                    if (( _cur < _scr )); then _scr=$(( _scr - 1 )); fi
                fi
                ;;
            $'\033[B'|j)  # down arrow
                if (( _cur < _n - 1 )); then
                    _cur=$(( _cur + 1 ))
                    if (( _cur >= _scr + _vis )); then _scr=$(( _scr + 1 )); fi
                fi
                ;;
            $'\033[C')  # right arrow — show preview
                [[ -n "$_desc_fn" ]] && _preview=1
                ;;
            $'\033[D')  # left arrow — hide preview
                _preview=0
                ;;
            ' ')  # space — toggle current item
                if (( _sel[_cur] )); then _sel[$_cur]=0; else _sel[$_cur]=1; fi
                ;;
            a|A)  for (( i=0; i<_n; i++ )); do _sel[$i]=1; done ;;
            n|N)  for (( i=0; i<_n; i++ )); do _sel[$i]=0; done ;;
            ''|$'\n')  _ok=1; break ;;
            q|Q)  break ;;
        esac
        _tui_draw
    done

    # Restore cursor and terminal state
    printf '\033[?25h' >"$_tty"
    [[ -n "$_stty" ]] && stty "$_stty" <"$_tty" 2>/dev/null || true

    if (( _ok )); then
        for (( i=0; i<_n; i++ )); do
            (( _sel[i] )) && printf '%s\n' "${_ti[$i]}"
        done
        return 0
    fi
    return 1
}
