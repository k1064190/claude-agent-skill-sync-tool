#!/usr/bin/env bats
# ABOUTME: Tests for tui_tree_select and _tui_tree_build (module-level)
# ABOUTME: Exercises the real source functions — no inline logic copies

setup() {
    ROOT="$(cd "$(dirname "$BATS_TEST_FILENAME")/.." && pwd)"
    # Stub tput so tests run without a real terminal
    tput() { echo 24; }
    export -f tput
    source "$ROOT/_tui_select.sh"
}

# ---------------------------------------------------------------------------
# Helper: call _tui_tree_build and dump the display table as "type indent label orig"
# Resets _TTB_* globals before each call.
# ---------------------------------------------------------------------------
dump_build() {
    # $@ = items to build from
    local -a INPUT=("$@")
    _tui_tree_build INPUT
    local _i
    for (( _i=0; _i<_TTB_N; _i++ )); do
        printf '%s %d %s %s\n' "${_TTB_TYPE[$_i]}" "${_TTB_INDENT[$_i]}" "${_TTB_LABEL[$_i]}" "${_TTB_ORIG[$_i]}"
    done
}

# ---------------------------------------------------------------------------
# _tui_tree_build: flat input (no dirs)
# ---------------------------------------------------------------------------
@test "_build_tree: flat input produces all leaf nodes at indent 0" {
    run dump_build "beta.md" "alpha.md" "gamma.md"
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "leaf 0 alpha.md alpha.md" ]
    [ "${lines[1]}" = "leaf 0 beta.md beta.md" ]
    [ "${lines[2]}" = "leaf 0 gamma.md gamma.md" ]
    [ "${#lines[@]}" -eq 3 ]
}

# ---------------------------------------------------------------------------
# _tui_tree_build: 2-level input
# ---------------------------------------------------------------------------
@test "_build_tree: 2-level input inserts dir node before leaves" {
    run dump_build "dev/frontend.md" "dev/backend.md"
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "dir 0 dev/ " ]
    [ "${lines[1]}" = "leaf 1 backend.md dev/backend.md" ]
    [ "${lines[2]}" = "leaf 1 frontend.md dev/frontend.md" ]
    [ "${#lines[@]}" -eq 3 ]
}

# ---------------------------------------------------------------------------
# _tui_tree_build: mixed root-level leaves and a dir
# ---------------------------------------------------------------------------
@test "_build_tree: root-level leaves and dirs are interleaved in sorted order" {
    run dump_build "organizer.md" "business/pm.md" "zz-standalone.md"
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "dir 0 business/ " ]
    [ "${lines[1]}" = "leaf 1 pm.md business/pm.md" ]
    [ "${lines[2]}" = "leaf 0 organizer.md organizer.md" ]
    [ "${lines[3]}" = "leaf 0 zz-standalone.md zz-standalone.md" ]
}

# ---------------------------------------------------------------------------
# _tui_tree_build: 3-level input
# ---------------------------------------------------------------------------
@test "_build_tree: 3-level path produces dir/subdir/leaf hierarchy" {
    run dump_build "dev/backend/architect.md" "dev/frontend.md"
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "dir 0 dev/ " ]
    [ "${lines[1]}" = "dir 1 backend/ " ]
    [ "${lines[2]}" = "leaf 2 architect.md dev/backend/architect.md" ]
    [ "${lines[3]}" = "leaf 1 frontend.md dev/frontend.md" ]
    [ "${#lines[@]}" -eq 4 ]
}

# ---------------------------------------------------------------------------
# _tui_tree_build: shared parent dirs inserted only once
# ---------------------------------------------------------------------------
@test "_build_tree: shared parent dirs are inserted only once" {
    run dump_build "dev/backend/arch.md" "dev/backend/db.md" "dev/frontend.md"
    [ "$status" -eq 0 ]
    local dir_count leaf_count
    dir_count=$(printf '%s\n' "${lines[@]}" | grep -c '^dir' || true)
    leaf_count=$(printf '%s\n' "${lines[@]}" | grep -c '^leaf' || true)
    [ "$dir_count" -eq 2 ]
    [ "$leaf_count" -eq 3 ]
}

# ---------------------------------------------------------------------------
# _TTB_LEAF_N count is accurate
# ---------------------------------------------------------------------------
@test "_build_tree: _TTB_LEAF_N counts only leaf slots" {
    local -a INPUT=("dev/a.md" "dev/b.md" "organizer.md")
    _tui_tree_build INPUT
    [ "$_TTB_LEAF_N" -eq 3 ]
    [ "$_TTB_N" -eq 4 ]   # 1 dir + 3 leaves
}

# ---------------------------------------------------------------------------
# Output format: only selected leaf original paths, not dir labels
# ---------------------------------------------------------------------------

# Simulate confirm: build, keep all pre-selected, emit output
emit_selected() {
    local -a INPUT=("$@")
    _tui_tree_build INPUT
    local _i
    for (( _i=0; _i<_TTB_N; _i++ )); do
        [[ "${_TTB_TYPE[$_i]}" == "leaf" ]] && (( _TTB_SEL[_i] )) && printf '%s\n' "${_TTB_ORIG[$_i]}"
    done
}

@test "output format: only selected leaf original paths are printed" {
    run emit_selected "dev/frontend.md" "dev/backend.md" "organizer.md"
    [ "$status" -eq 0 ]
    ! printf '%s\n' "${lines[@]}" | grep -q '^dev/$'
    printf '%s\n' "${lines[@]}" | grep -q 'dev/frontend.md'
    printf '%s\n' "${lines[@]}" | grep -q 'dev/backend.md'
    printf '%s\n' "${lines[@]}" | grep -q 'organizer.md'
    [ "${#lines[@]}" -eq 3 ]
}

@test "output format: dir nodes produce no output" {
    run emit_selected "a/b.md" "a/c.md"
    [ "$status" -eq 0 ]
    ! printf '%s\n' "${lines[@]}" | grep -q '^a/$'
    [ "${#lines[@]}" -eq 2 ]
}

# ---------------------------------------------------------------------------
# Cascade toggle: partial dir → Space checks all descendants
# ---------------------------------------------------------------------------

simulate_cascade_toggle() {
    # Build items; set first leaf to 0 (partial); toggle dir; print sel values
    local _toggle_idx="$1"; shift
    local -a INPUT=("$@")
    _tui_tree_build INPUT

    # Create local aliases for readability
    local -n _type=_TTB_TYPE _indent=_TTB_INDENT _sel=_TTB_SEL

    # Helper: get leaf descendants of a dir
    get_descs() {
        local _di="$1" _dind _i
        _dind="${_indent[$_di]}"
        for (( _i=_di+1; _i<_TTB_N; _i++ )); do
            (( _indent[_i] <= _dind )) && break
            [[ "${_type[$_i]}" == "leaf" ]] && echo "$_i"
        done
    }

    # Set first descendant to 0 → partial state
    local -a _first=()
    mapfile -t _first < <(get_descs "$_toggle_idx")
    [[ ${#_first[@]} -gt 0 ]] && _sel[${_first[0]}]=0

    # Compute dir state
    local -a _descs=()
    mapfile -t _descs < <(get_descs "$_toggle_idx")
    local _total="${#_descs[@]}" _cnt=0 _d
    for _d in "${_descs[@]}"; do _cnt=$(( _cnt + _sel[_d] )); done
    local _state
    if (( _cnt == 0 )); then _state="unchecked"
    elif (( _cnt == _total )); then _state="checked"
    else _state="partial"; fi

    # Toggle
    local _new_val
    [[ "$_state" == "checked" ]] && _new_val=0 || _new_val=1
    for _d in "${_descs[@]}"; do _sel[$_d]=$_new_val; done

    # Print leaf sel values
    local _i
    for (( _i=0; _i<_TTB_N; _i++ )); do
        [[ "${_type[$_i]}" == "leaf" ]] && printf '%s\n' "${_sel[$_i]}"
    done
}

@test "cascade toggle: partial dir → Space checks all descendants" {
    run simulate_cascade_toggle 0 "dev/a.md" "dev/b.md" "dev/c.md"
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "1" ]
    [ "${lines[1]}" = "1" ]
    [ "${lines[2]}" = "1" ]
}

# ---------------------------------------------------------------------------
# Cascade toggle: fully-checked dir → Space unchecks all descendants
# ---------------------------------------------------------------------------

simulate_fully_checked_toggle() {
    local _toggle_idx="$1"; shift
    local -a INPUT=("$@")
    _tui_tree_build INPUT

    local -n _type=_TTB_TYPE _indent=_TTB_INDENT _sel=_TTB_SEL

    get_descs2() {
        local _di="$1" _dind _i
        _dind="${_indent[$_di]}"
        for (( _i=_di+1; _i<_TTB_N; _i++ )); do
            (( _indent[_i] <= _dind )) && break
            [[ "${_type[$_i]}" == "leaf" ]] && echo "$_i"
        done
    }

    # All leaves pre-selected (default) → dir is "checked"
    local -a _descs=()
    mapfile -t _descs < <(get_descs2 "$_toggle_idx")
    local _total="${#_descs[@]}" _cnt=0 _d
    for _d in "${_descs[@]}"; do _cnt=$(( _cnt + _sel[_d] )); done
    local _state
    (( _cnt == _total )) && _state="checked" || _state="partial"

    local _new_val
    [[ "$_state" == "checked" ]] && _new_val=0 || _new_val=1
    for _d in "${_descs[@]}"; do _sel[$_d]=$_new_val; done

    local _i
    for (( _i=0; _i<_TTB_N; _i++ )); do
        [[ "${_type[$_i]}" == "leaf" ]] && printf '%s\n' "${_sel[$_i]}"
    done
}

@test "cascade toggle: fully-checked dir → Space unchecks all descendants" {
    run simulate_fully_checked_toggle 0 "dev/a.md" "dev/b.md"
    [ "$status" -eq 0 ]
    [ "${lines[0]}" = "0" ]
    [ "${lines[1]}" = "0" ]
}

# ---------------------------------------------------------------------------
# Empty input → return 1
# ---------------------------------------------------------------------------
@test "empty input returns exit code 1" {
    run bash -c "
        source '$ROOT/_tui_select.sh'
        local -a EMPTY=()
        tui_tree_select EMPTY
    "
    [ "$status" -eq 1 ]
}

# ---------------------------------------------------------------------------
# a/n: select-all and deselect-all work on leaves only, dir sel stays 0
# ---------------------------------------------------------------------------
@test "select-all keeps dir slots at 0" {
    local -a INPUT=("dev/a.md" "dev/b.md")
    _tui_tree_build INPUT
    local -n _type=_TTB_TYPE _sel=_TTB_SEL

    # Simulate 'n' then 'a'
    local _i
    for (( _i=0; _i<_TTB_N; _i++ )); do [[ "${_type[$_i]}" == "leaf" ]] && _sel[$_i]=0; done
    for (( _i=0; _i<_TTB_N; _i++ )); do [[ "${_type[$_i]}" == "leaf" ]] && _sel[$_i]=1; done

    # Dir slot (index 0) must remain 0
    [ "${_sel[0]}" -eq 0 ]
    # Leaf slots must be 1
    [ "${_sel[1]}" -eq 1 ]
    [ "${_sel[2]}" -eq 1 ]
}
