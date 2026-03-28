#!/usr/bin/env bash
# ABOUTME: Shared TUI checkbox selector used by sync scripts
# ABOUTME: Provides flat checkbox (tui_checkbox_select) and hierarchical tree (tui_tree_select)

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

# _tui_tree_build - Parse input items into parallel display arrays (module-level, testable)
# Sorts items lexicographically; inserts dir header nodes lazily on first encounter.
# Supports paths up to 3 levels deep: leaf, dir/leaf, or dir/subdir/leaf.
# Args:
#   $1 (str): Name of the input items array (read via nameref)
# Outputs (globals — read by tui_tree_select via namerefs):
#   _TTB_LABEL  (array): display label per slot — basename or "dirname/"
#   _TTB_TYPE   (array): "dir" | "leaf" per slot
#   _TTB_INDENT (array): indentation depth 0, 1, or 2 per slot
#   _TTB_ORIG   (array): original path per slot (leaves); "" for dir slots
#   _TTB_SEL    (array): selection bit per slot (leaves: 0/1; dirs: always 0)
#   _TTB_CACHE  (array): dir-state cache string per slot ("" = stale)
#   _TTB_N      (int):   total display slot count
#   _TTB_LEAF_N (int):   leaf slot count
_tui_tree_build() {
    local -n _btb_ti="$1"
    _TTB_LABEL=(); _TTB_TYPE=(); _TTB_INDENT=(); _TTB_ORIG=(); _TTB_SEL=(); _TTB_CACHE=()
    _TTB_N=0; _TTB_LEAF_N=0

    local -a _sorted=()
    mapfile -t _sorted < <(printf '%s\n' "${_btb_ti[@]}" | sort)
    local -A _dir_seen=()
    local _item _d1 _d12 _depth
    local -a _parts

    for _item in "${_sorted[@]}"; do
        IFS='/' read -ra _parts <<< "$_item"
        _depth=$(( ${#_parts[@]} - 1 ))

        if (( _depth == 0 )); then
            _TTB_LABEL+=("${_parts[0]}")
            _TTB_TYPE+=("leaf")
            _TTB_INDENT+=(0)
            _TTB_ORIG+=("$_item")
            _TTB_SEL+=(1)
            _TTB_CACHE+=("")
            _TTB_LEAF_N=$(( _TTB_LEAF_N + 1 ))

        elif (( _depth == 1 )); then
            _d1="${_parts[0]}"
            if [[ -z "${_dir_seen[${_d1}]+x}" ]]; then
                _dir_seen["${_d1}"]=1
                _TTB_LABEL+=("${_d1}/")
                _TTB_TYPE+=("dir")
                _TTB_INDENT+=(0)
                _TTB_ORIG+=("")
                _TTB_SEL+=(0)
                _TTB_CACHE+=("")
            fi
            _TTB_LABEL+=("${_parts[1]}")
            _TTB_TYPE+=("leaf")
            _TTB_INDENT+=(1)
            _TTB_ORIG+=("$_item")
            _TTB_SEL+=(1)
            _TTB_CACHE+=("")
            _TTB_LEAF_N=$(( _TTB_LEAF_N + 1 ))

        elif (( _depth >= 2 )); then
            _d1="${_parts[0]}"
            _d12="${_parts[0]}/${_parts[1]}"
            if [[ -z "${_dir_seen[${_d1}]+x}" ]]; then
                _dir_seen["${_d1}"]=1
                _TTB_LABEL+=("${_d1}/")
                _TTB_TYPE+=("dir")
                _TTB_INDENT+=(0)
                _TTB_ORIG+=("")
                _TTB_SEL+=(0)
                _TTB_CACHE+=("")
            fi
            if [[ -z "${_dir_seen[${_d12}]+x}" ]]; then
                _dir_seen["${_d12}"]=1
                _TTB_LABEL+=("${_parts[1]}/")
                _TTB_TYPE+=("dir")
                _TTB_INDENT+=(1)
                _TTB_ORIG+=("")
                _TTB_SEL+=(0)
                _TTB_CACHE+=("")
            fi
            _TTB_LABEL+=("${_parts[2]}")
            _TTB_TYPE+=("leaf")
            _TTB_INDENT+=(2)
            _TTB_ORIG+=("$_item")
            _TTB_SEL+=(1)
            _TTB_CACHE+=("")
            _TTB_LEAF_N=$(( _TTB_LEAF_N + 1 ))
        fi
    done
    _TTB_N=${#_TTB_LABEL[@]}
}

# tui_tree_select - Interactive tree-view checkbox TUI with hierarchical navigation
# Renders slash-delimited paths as a collapsible tree. Space on a directory toggles
# all its descendants; [~] indicates partial selection within a category.
# Args:
#   $1 (str): Name of bash array variable containing leaf item paths (dirs are inferred)
#   $2 (str, optional): Name of function that, given a leaf path, outputs its description.
#                       Right arrow shows preview for leaves; dirs show item count.
# Output (stdout): Selected leaf item original paths, one per line
# Returns: 0 on confirm, 1 on cancel
# Requires: bash 4.3+ (declare -A, mapfile, local -n)
tui_tree_select() {
    local -n _tts_check="$1"
    [[ ${#_tts_check[@]} -eq 0 ]] && return 1
    unset -n _tts_check

    local _desc_fn="${2:-}"
    local _tty="/dev/tty"

    # Build display structure; results are in _TTB_* globals
    _tui_tree_build "$1"
    [[ $_TTB_N -eq 0 ]] && return 1

    # Alias _TTB_* globals as locals (arrays via nameref, integers via copy)
    local -n _disp_label=_TTB_LABEL
    local -n _disp_type=_TTB_TYPE
    local -n _disp_indent=_TTB_INDENT
    local -n _disp_orig=_TTB_ORIG
    local -n _sel=_TTB_SEL
    local -n _dir_state_cache=_TTB_CACHE
    local _n=$_TTB_N
    local _leaf_n=$_TTB_LEAF_N

    # Side-channel return value used by _tui_tree_dir_state (avoids subshell)
    local _tts_state=""

    # _tui_tree_leaf_descendants - Print leaf display indices under a dir node
    # Scans forward from the dir's slot while indent is deeper than the dir's.
    # Args:
    #   $1 (int): Display index of the dir node
    # Output (stdout): Leaf display indices, one per line
    _tui_tree_leaf_descendants() {
        local _di="$1" _dind _i
        _dind="${_disp_indent[$_di]}"
        for (( _i=_di+1; _i<_n; _i++ )); do
            (( _disp_indent[_i] <= _dind )) && break
            [[ "${_disp_type[$_i]}" == "leaf" ]] && echo "$_i"
        done
    }

    # _tui_tree_dir_state - Compute and cache a dir node's checkbox state
    # Sets _tts_state to "checked", "unchecked", or "partial".
    # Reads _dir_state_cache; writes back when recomputing.
    # Args:
    #   $1 (int): Display index of the dir node
    _tui_tree_dir_state() {
        local _di="$1"
        if [[ -n "${_dir_state_cache[$_di]}" ]]; then
            _tts_state="${_dir_state_cache[$_di]}"
            return
        fi
        local -a _descs=()
        mapfile -t _descs < <(_tui_tree_leaf_descendants "$_di")
        local _total="${#_descs[@]}" _cnt=0 _d
        if (( _total == 0 )); then
            _tts_state="unchecked"
        else
            for _d in "${_descs[@]}"; do _cnt=$(( _cnt + _sel[_d] )); done
            if (( _cnt == 0 )); then _tts_state="unchecked"
            elif (( _cnt == _total )); then _tts_state="checked"
            else _tts_state="partial"
            fi
        fi
        _dir_state_cache[$_di]="$_tts_state"
    }

    # _tui_tree_invalidate_cache - Clear all cached dir states
    _tui_tree_invalidate_cache() {
        local _i
        for (( _i=0; _i<_n; _i++ )); do _dir_state_cache[$_i]=""; done
    }

    # Terminal dimensions
    local _rows _cols
    _rows=$(tput lines 2>/dev/null || echo 24)
    _cols=$(tput cols 2>/dev/null || echo 80)

    local _prev_height=0
    [[ -n "$_desc_fn" ]] && _prev_height=5

    local _vis=$(( _rows - 4 - _prev_height ))
    (( _vis > _n )) && _vis=$_n
    (( _vis < 3  )) && _vis=3
    local _total=$(( _vis + 3 + _prev_height ))

    local _cur=0 _scr=0 _ok=0 _preview=0

    # ANSI codes
    local R=$'\033[0m' REV=$'\033[7m' GRN=$'\033[32m' YLW=$'\033[33m' CYN=$'\033[36m' BD=$'\033[1m' DIM=$'\033[2m'
    local CL=$'\033[2K\r'

    # Count selected leaves for the header counter
    # Output (stdout): integer count
    _tui_tree_cnt() {
        local _c=0 _i
        for (( _i=0; _i<_n; _i++ )); do
            [[ "${_disp_type[$_i]}" == "leaf" ]] && _c=$(( _c + _sel[_i] ))
        done
        echo "$_c"
    }

    # Draw the preview panel (_prev_height lines)
    # _tui_tree_draw_preview writes to stdout (caller must redirect to _tty)
    _tui_tree_draw_preview() {
        [[ $_prev_height -eq 0 ]] && return

        if (( _preview )); then
            local _preview_text=""
            if [[ "${_disp_type[$_cur]}" == "dir" ]]; then
                local -a _pdescs=()
                mapfile -t _pdescs < <(_tui_tree_leaf_descendants "$_cur")
                _preview_text="${#_pdescs[@]} items in ${_disp_label[$_cur]}"
            elif [[ -n "$_desc_fn" ]]; then
                _preview_text=$("$_desc_fn" "${_disp_orig[$_cur]}" 2>/dev/null)
                [[ -z "$_preview_text" ]] && _preview_text="(no description)"
            fi
            local _pw=$(( _cols - 3 ))
            printf "${CL}${BD}${CYN}─── description ─${R}\n"
            local -a _wlines=()
            while IFS= read -r _wl; do _wlines+=("$_wl"); done < <(_tui_fold "$_preview_text" "$_pw")
            local _wi
            for (( _wi=0; _wi<3; _wi++ )); do
                if (( _wi < ${#_wlines[@]} )); then
                    printf "${CL} %s\n" "${_wlines[$_wi]}"
                else
                    printf "${CL}\n"
                fi
            done
            printf "${CL}\n"
        else
            printf "${CL}  ${DIM}→ right arrow to preview description${R}\n"
            local _pi
            for (( _pi=1; _pi<_prev_height; _pi++ )); do printf "${CL}\n"; done
        fi
    }

    # Full redraw of the tree TUI (single O(n) pass over display slots).
    # All output is written in one grouped redirect to avoid partial renders.
    _tui_tree_draw() {
        {
            printf '\033[%dA' "$_total"

            local _hint=""
            [[ -n "$_desc_fn" ]] && _hint="  →=preview"
            printf "${CL}${BD}${CYN} [%d/%d]  ↑↓=navigate  Space=toggle  a=all  n=none  Enter=confirm  q=cancel%s${R}\n" \
                "$(_tui_tree_cnt)" "$_leaf_n" "$_hint"

            if (( _scr > 0 )); then
                printf "${CL}  ${YLW}↑ %d more above${R}\n" "$_scr"
            else
                printf "${CL}\n"
            fi

            local _end=$(( _scr + _vis ))
            (( _end > _n )) && _end=$_n

            local _i
            for (( _i=_scr; _i<_end; _i++ )); do
                local _mark _c="" _istr=""
                local _ii
                for (( _ii=0; _ii<_disp_indent[_i]; _ii++ )); do _istr+="  "; done

                if [[ "${_disp_type[$_i]}" == "dir" ]]; then
                    _tui_tree_dir_state "$_i"
                    case "$_tts_state" in
                        checked)  _mark="[x]"; _c="$GRN" ;;
                        partial)  _mark="[~]"; _c="$YLW" ;;
                        *)        _mark="[ ]"; _c="" ;;
                    esac
                else
                    if (( _sel[_i] )); then _mark="[x]"; _c="$GRN"; else _mark="[ ]"; _c=""; fi
                fi

                local _lw=$(( _cols - 8 - ${#_istr} ))
                (( _lw < 1 )) && _lw=1
                if (( _i == _cur )); then
                    printf "${CL}${REV} ▶ %s%s %-${_lw}s ${R}\n" \
                        "$_istr" "$_mark" "${_disp_label[$_i]}"
                else
                    printf "${CL}   ${_c}%s%s${R} %s\n" \
                        "$_istr" "$_mark" "${_disp_label[$_i]}"
                fi
            done

            for (( _i=_end; _i<_scr+_vis; _i++ )); do printf "${CL}\n"; done

            if (( _scr + _vis < _n )); then
                printf "${CL}  ${YLW}↓ %d more below${R}\n" "$(( _n - _scr - _vis ))"
            else
                printf "${CL}\n"
            fi

            _tui_tree_draw_preview
        } >"$_tty"
    }

    # Read one keypress, handling escape sequences for arrow keys
    # Output (stdout): key identifier string
    _tui_tree_key() {
        local _k _seq
        IFS= read -rsn1 _k <"$_tty"
        if [[ "$_k" == $'\033' ]]; then
            IFS= read -rsn2 -t 0.1 _seq <"$_tty" || true
            _k="${_k}${_seq}"
        fi
        printf '%s' "$_k"
    }

    local _stty; _stty=$(stty -g <"$_tty" 2>/dev/null) || _stty=""

    # Restore terminal on unexpected exit (SIGINT, SIGTERM, etc.)
    _tui_tree_cleanup() {
        printf '\033[?25h' >"$_tty"
        [[ -n "$_stty" ]] && stty "$_stty" <"$_tty" 2>/dev/null || true
        trap - EXIT INT TERM
    }
    trap _tui_tree_cleanup EXIT INT TERM

    printf '\033[?25l' >"$_tty"
    for (( _i=0; _i<_total; _i++ )); do printf '\n' >"$_tty"; done
    _tui_tree_draw

    # _tui_tree_process_key - Apply one keypress to TUI state
    # Args:
    #   $1 (str): Key identifier string from _tui_tree_key
    # Returns: 0 to continue loop, 1 to exit loop (Enter or q)
    _tui_tree_process_key() {
        case "$1" in
            $'\033[A'|k)  # up arrow
                if (( _cur > 0 )); then
                    _cur=$(( _cur - 1 ))
                    (( _cur < _scr )) && _scr=$(( _scr - 1 ))
                fi
                ;;
            $'\033[B'|j)  # down arrow
                if (( _cur < _n - 1 )); then
                    _cur=$(( _cur + 1 ))
                    (( _cur >= _scr + _vis )) && _scr=$(( _scr + 1 ))
                fi
                ;;
            $'\033[C')  # right arrow — show preview
                _preview=1
                ;;
            $'\033[D')  # left arrow — hide preview
                _preview=0
                ;;
            ' ')  # space — toggle current item
                if [[ "${_disp_type[$_cur]}" == "dir" ]]; then
                    local -a _descs=()
                    mapfile -t _descs < <(_tui_tree_leaf_descendants "$_cur")
                    _tui_tree_dir_state "$_cur"
                    local _new_val
                    if [[ "$_tts_state" == "checked" ]]; then _new_val=0; else _new_val=1; fi
                    local _d
                    for _d in "${_descs[@]}"; do _sel[$_d]=$_new_val; done
                    _tui_tree_invalidate_cache
                else
                    _sel[$_cur]=$(( 1 - _sel[_cur] ))
                    _tui_tree_invalidate_cache
                fi
                ;;
            a|A)  # select all leaves
                local _i
                for (( _i=0; _i<_n; _i++ )); do
                    [[ "${_disp_type[$_i]}" == "leaf" ]] && _sel[$_i]=1
                done
                _tui_tree_invalidate_cache
                ;;
            n|N)  # deselect all leaves
                local _i
                for (( _i=0; _i<_n; _i++ )); do
                    [[ "${_disp_type[$_i]}" == "leaf" ]] && _sel[$_i]=0
                done
                _tui_tree_invalidate_cache
                ;;
            ''|$'\n')  _ok=1; return 1 ;;
            q|Q)  return 1 ;;
        esac
        return 0
    }

    local _done=0
    while true; do
        local _k; _k=$(_tui_tree_key)
        if ! _tui_tree_process_key "$_k"; then
            break
        fi

        # Drain any buffered keypresses before redrawing. When a key is held
        # down the terminal queues multiple events; consuming them all here
        # means we redraw once per visual frame instead of once per keypress,
        # eliminating the jitter caused by rapid clear-and-repaint cycles.
        local _k2 _seq2
        while IFS= read -rsn1 -t 0 _k2 <"$_tty" 2>/dev/null; do
            if [[ "$_k2" == $'\033' ]]; then
                IFS= read -rsn2 -t 0.05 _seq2 <"$_tty" 2>/dev/null || true
                _k2="${_k2}${_seq2}"
            fi
            if ! _tui_tree_process_key "$_k2"; then
                _done=1
                break
            fi
        done
        (( _done )) && break

        _tui_tree_draw
    done

    trap - EXIT INT TERM
    printf '\033[?25h' >"$_tty"
    [[ -n "$_stty" ]] && stty "$_stty" <"$_tty" 2>/dev/null || true

    if (( _ok )); then
        local _i
        for (( _i=0; _i<_n; _i++ )); do
            [[ "${_disp_type[$_i]}" == "leaf" ]] && (( _sel[_i] )) && printf '%s\n' "${_disp_orig[$_i]}"
        done
        return 0
    fi
    return 1
}
