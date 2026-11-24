#!/usr/bin/env bash
# check-translations.sh — report translations under docs/i18n/ that have fallen
# behind their English source, and fail when the bookkeeping disagrees.
#
# docs/i18n/README.md holds a table of language, source, file and source
# commit, and each translated file repeats its source and source commit in a
# note at the top. This script:
#
#   1. parses the table;
#   2. fails when a listed file is missing, or its note names a different source
#      or source commit than the table (the bookkeeping is wrong);
#   3. fails when a recorded commit does not resolve (in CI that means a
#      shallow checkout: the job needs fetch-depth: 0);
#   4. for each translation, counts the commits since the recorded commit that
#      changed the English source — for a `FILE → "Section"` source, only
#      commits that changed that section's text — and lists every translation
#      that is behind, with how many commits.
#
# Staleness is reported, not failed: an English fix must not be blocked on a
# translator. Under GitHub Actions a stale translation becomes a ::warning
# annotation when its English source changed in the range given by
# --changed-since REF (REF...HEAD); otherwise it is listed in the output.
#
# Usage: scripts/check-translations.sh [--changed-since REF] [--head REV]
#
# Exit status: 0 when the table and notes agree (stale or not), 1 when they
# disagree or a recorded commit cannot be resolved, 2 on a usage error.
set -euo pipefail

cd "$(dirname "$0")/.."

I18N_DIR=docs/i18n
TABLE="$I18N_DIR/README.md"
HEAD_REV=HEAD
CHANGED_SINCE=""

usage() {
    sed -n '2,/^set -euo/{/^set -euo/d;s/^# \{0,1\}//;p}' "$0" >&2
    exit 2
}

while [ $# -gt 0 ]; do
    case "$1" in
        --changed-since) [ $# -ge 2 ] || usage; CHANGED_SINCE="$2"; shift 2 ;;
        --head) [ $# -ge 2 ] || usage; HEAD_REV="$2"; shift 2 ;;
        -h|--help) usage ;;
        *) echo "check-translations: unknown argument: $1" >&2; usage ;;
    esac
done

annotate=false
if [ "${GITHUB_ACTIONS:-}" = "true" ]; then
    annotate=true
fi

errors=0
fail() {
    # fail FILE MESSAGE
    errors=$((errors + 1))
    if $annotate; then
        echo "::error file=$1::$2"
    else
        echo "error: $1: $2" >&2
    fi
}

if ! git rev-parse --verify --quiet "$HEAD_REV^{commit}" >/dev/null; then
    echo "check-translations: cannot resolve $HEAD_REV" >&2
    exit 1
fi

# section_text REV FILE HEADING prints the body of the "## HEADING" section of
# FILE at REV, up to the next level-2 heading. Empty output means the file or
# the section does not exist at REV.
section_text() {
    git show "$1:$2" 2>/dev/null | awk -v want="## $3" '
        $0 == want { on = 1; next }
        on && /^## / { exit }
        on { print }
    '
}

# commits_behind RECORDED PATH [HEADING] prints how many commits in
# RECORDED..HEAD changed PATH or, with HEADING, changed that section's text.
commits_behind() {
    local recorded=$1 path=$2 heading=${3:-} count=0 rev prev cur
    if [ -z "$heading" ]; then
        git rev-list --count "$recorded..$HEAD_REV" -- "$path"
        return
    fi
    # Walk oldest first, comparing each commit's section with the text at the
    # previous commit that touched the file (or the recorded commit).
    prev=$(section_text "$recorded" "$path" "$heading")
    while IFS= read -r rev; do
        [ -n "$rev" ] || continue
        cur=$(section_text "$rev" "$path" "$heading")
        if [ "$cur" != "$prev" ]; then
            count=$((count + 1))
        fi
        prev=$cur
    done < <(git rev-list --reverse "$recorded..$HEAD_REV" -- "$path")
    echo "$count"
}

# touched_since PATH reports whether PATH changed in CHANGED_SINCE...HEAD.
touched_since() {
    [ -n "$CHANGED_SINCE" ] || return 1
    ! git diff --quiet "$CHANGED_SINCE...$HEAD_REV" -- "$1"
}

# Table rows look like:
#   | Français | `docs/glossary.md` | [`fr/glossary.md`](fr/glossary.md) | `8ddcb44` |
#   | Français | `README.md` → "Status" | [`fr/README-status.md`](fr/README-status.md) | `8ddcb44` |
rows=$(awk -F'|' '
    NF >= 6 && $5 ~ /`[0-9a-f]+`/ {
        src = $3; file = $4; commit = $5
        gsub(/^[ \t]+|[ \t]+$/, "", src)
        if (match(file, /\]\([^)]*\)/)) file = substr(file, RSTART + 2, RLENGTH - 3)
        else file = ""
        gsub(/[` \t]/, "", commit)
        print src "\t" file "\t" commit
    }
' "$TABLE")

if [ -z "$rows" ]; then
    fail "$TABLE" "no translation rows found in the table"
fi

checked=0
stale=0
listed=()

while IFS=$'\t' read -r src file commit; do
    [ -n "$src" ] || continue
    checked=$((checked + 1))
    path="$I18N_DIR/$file"
    listed+=("$path")

    # Source is `PATH` or `PATH` → "Section".
    # shellcheck disable=SC2016 # backticks are literal Markdown, not expansions
    src_path=$(printf '%s' "$src" | sed -n 's/^`\([^`]*\)`.*/\1/p')
    heading=$(printf '%s' "$src" | sed -n 's/.*→ *"\([^"]*\)".*/\1/p')
    if [ -z "$src_path" ] || [ -z "$file" ]; then
        fail "$TABLE" "cannot parse row: source '$src', file '$file'"
        continue
    fi
    if [ ! -f "$path" ]; then
        fail "$TABLE" "$path is listed in the table but does not exist"
        continue
    fi

    # The note at the top of each file: a "Source"/"Fuente" line naming the
    # English path, and a line naming the commit ("Commit source : abc1234",
    # "Commit de origen: abc1234").
    note=$(sed -n '1,/^-->/p' "$path")
    note_src=$(printf '%s\n' "$note" | sed -n 's/^\(Source\|Fuente\)[[:space:]]*:[[:space:]]*\([^,[:space:]]*\).*/\2/p' | head -n 1)
    note_commit=$(printf '%s\n' "$note" | sed -n 's/^Commit[^:]*:[[:space:]]*\([0-9a-f]\{7,40\}\).*/\1/p' | head -n 1)
    note_section=$(printf '%s\n' "$note" | sed -n 's/^\(Source\|Fuente\).*##[[:space:]]*\([^»]*[^»[:space:]]\)[[:space:]]*».*/\2/p' | head -n 1)

    if [ -z "$note_commit" ]; then
        fail "$path" "the note at the top records no source commit; the table says $commit"
    elif [ "$note_commit" != "$commit" ]; then
        fail "$path" "the note records source commit $note_commit but $TABLE says $commit"
    fi
    if [ -z "$note_src" ]; then
        fail "$path" "the note at the top names no source document; the table says $src_path"
    elif [ "$note_src" != "$src_path" ]; then
        fail "$path" "the note names source $note_src but $TABLE says $src_path"
    fi
    if [ "$note_section" != "$heading" ]; then
        fail "$path" "the note names section '${note_section:-none}' but $TABLE says '${heading:-none}'"
    fi

    if ! git rev-parse --verify --quiet "$commit^{commit}" >/dev/null; then
        fail "$path" "source commit $commit does not resolve; a shallow checkout needs fetch-depth: 0"
        continue
    fi
    if ! git merge-base --is-ancestor "$commit" "$HEAD_REV"; then
        fail "$path" "source commit $commit is not an ancestor of $HEAD_REV"
        continue
    fi

    if [ -n "$heading" ] && [ -z "$(section_text "$HEAD_REV" "$src_path" "$heading")" ]; then
        fail "$path" "$src_path has no '## $heading' section at $HEAD_REV"
        continue
    fi

    behind=$(commits_behind "$commit" "$src_path" "$heading")
    [ "$behind" -gt 0 ] || continue
    stale=$((stale + 1))

    what=$src_path
    [ -z "$heading" ] || what="$src_path → \"$heading\""
    msg="stale translation: $what changed in $behind commit(s) since $commit (latest $(git log -1 --format=%h "$HEAD_REV" -- "$src_path"))"
    if $annotate && touched_since "$src_path"; then
        echo "::warning file=$path,title=Stale translation::$msg"
    else
        echo "$path: $msg"
    fi
done <<< "$rows"

# Every translated file must be in the table, or nothing checks it.
while IFS= read -r f; do
    found=false
    for l in "${listed[@]+"${listed[@]}"}"; do
        if [ "$l" = "$f" ]; then found=true; break; fi
    done
    $found || fail "$f" "translation is not listed in $TABLE"
done < <(find "$I18N_DIR" -mindepth 2 -type f -name '*.md' | sort)

echo "check-translations: $checked translation(s), $stale stale, $errors error(s)"
[ "$errors" -eq 0 ]
