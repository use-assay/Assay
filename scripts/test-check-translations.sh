#!/usr/bin/env bash
# test-check-translations.sh — exercise scripts/check-translations.sh against a
# throwaway git repository, so its counting and its failure rules are pinned
# without depending on the real translations' history.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

pass=0
failures=0
ok() { pass=$((pass + 1)); }
bad() { failures=$((failures + 1)); echo "FAIL: $*" >&2; }

repo="$TMP/repo"
mkdir -p "$repo/scripts" "$repo/docs/i18n/fr"
cp "$ROOT/scripts/check-translations.sh" "$repo/scripts/"
cd "$repo"
git init --quiet
git config user.email test@example.invalid
git config user.name test
git config commit.gpgsign false

commit() { git add -A && git commit --quiet -m "$1"; }

printf '# Project\n\n## Status\n\nIt works.\n\n## Other\n\nOne.\n' > README.md
printf '# Guide\n\nStep one.\n' > docs/guide.md
commit "english sources"
SRC=$(git rev-parse --short HEAD)

note() {
    # note SOURCE-LINE COMMIT
    printf '<!--\nTraduction.\n\n%s\nCommit source : %s\n-->\n\nTexte.\n' "$1" "$2"
}
note 'Source : docs/guide.md' "$SRC" > docs/i18n/fr/guide.md
note 'Source : README.md, section « ## Status »' "$SRC" > docs/i18n/fr/README-status.md
table() {
    cat > docs/i18n/README.md <<EOF
# Translations

| Language | Source section | File | Source commit |
| --- | --- | --- | --- |
| Français | \`docs/guide.md\` | [\`fr/guide.md\`](fr/guide.md) | \`$1\` |
| Français | \`README.md\` → "Status" | [\`fr/README-status.md\`](fr/README-status.md) | \`$1\` |
EOF
}
table "$SRC"
commit "translations"

run() { out=$(./scripts/check-translations.sh 2>&1) && rc=0 || rc=$?; }

# 1. Fresh translations: exit 0, nothing stale.
run
if [ "$rc" -eq 0 ] && grep -q "2 translation(s), 0 stale, 0 error(s)" <<<"$out"; then ok; else bad "fresh: rc=$rc $out"; fi

# 2. A change outside the tracked README section does not make it stale.
printf '# Project\n\n## Status\n\nIt works.\n\n## Other\n\nTwo.\n' > README.md
commit "other section"
run
if [ "$rc" -eq 0 ] && ! grep -q "README-status" <<<"$out"; then ok; else bad "untracked section: rc=$rc $out"; fi

# 3. Two changes to the guide and one to the Status section are counted.
printf '# Guide\n\nStep one, revised.\n' > docs/guide.md
commit "guide 1"
printf '# Guide\n\nStep one, revised again.\n' > docs/guide.md
commit "guide 2"
printf '# Project\n\n## Status\n\nIt works well.\n\n## Other\n\nTwo.\n' > README.md
commit "status"
run
if [ "$rc" -eq 0 ] &&
    grep -q "fr/guide.md: stale translation: docs/guide.md changed in 2 commit(s)" <<<"$out" &&
    grep -q "fr/README-status.md: stale translation: README.md → \"Status\" changed in 1 commit(s)" <<<"$out"; then
    ok
else
    bad "stale counts: rc=$rc $out"
fi

# 4. Under GitHub Actions a stale translation whose source changed in the
#    range is a warning annotation; one whose source did not is plain text.
out=$(GITHUB_ACTIONS=true ./scripts/check-translations.sh --changed-since HEAD~1 2>&1) && rc=0 || rc=$?
if [ "$rc" -eq 0 ] &&
    grep -q '^::warning file=docs/i18n/fr/README-status.md,' <<<"$out" &&
    grep -q '^docs/i18n/fr/guide.md: stale' <<<"$out"; then
    ok
else
    bad "annotations: rc=$rc $out"
fi

# 5. The table and a note disagreeing on the commit fails.
cp docs/i18n/fr/guide.md "$TMP/guide.bak"
sed -i.orig "s/^Commit source : .*/Commit source : $(git rev-parse --short HEAD)/" docs/i18n/fr/guide.md
run
if [ "$rc" -eq 1 ] && grep -q "note records source commit" <<<"$out"; then ok; else bad "commit disagreement: rc=$rc $out"; fi
cp "$TMP/guide.bak" docs/i18n/fr/guide.md
rm -f docs/i18n/fr/guide.md.orig

# 6. The note naming a different source fails.
sed -i.orig 's|^Source : docs/guide.md|Source : docs/other.md|' docs/i18n/fr/guide.md
run
if [ "$rc" -eq 1 ] && grep -q "note names source docs/other.md" <<<"$out"; then ok; else bad "source disagreement: rc=$rc $out"; fi
cp "$TMP/guide.bak" docs/i18n/fr/guide.md
rm -f docs/i18n/fr/guide.md.orig

# 7. A translation missing from the table fails.
cp docs/i18n/fr/guide.md docs/i18n/fr/extra.md
run
if [ "$rc" -eq 1 ] && grep -q "extra.md.*not listed" <<<"$out"; then ok; else bad "unlisted file: rc=$rc $out"; fi
rm docs/i18n/fr/extra.md

# 8. A recorded commit that does not resolve (a shallow clone) fails.
table deadbee
sed -i.orig "s/^Commit source : .*/Commit source : deadbee/" docs/i18n/fr/guide.md docs/i18n/fr/README-status.md
run
if [ "$rc" -eq 1 ] && grep -q "does not resolve" <<<"$out"; then ok; else bad "unresolvable commit: rc=$rc $out"; fi

echo "test-check-translations: $pass passed, $failures failed"
[ "$failures" -eq 0 ]
