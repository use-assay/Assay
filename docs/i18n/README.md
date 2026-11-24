# Translations

English is authoritative. These translations exist so readers in francophone
Africa and Latin America can evaluate Assay in their own language; where a
translation and the English source disagree, the English document wins.

| Language | Source section | File | Source commit |
| --- | --- | --- | --- |
| Français | `README.md` → "Status" | [`fr/README-status.md`](fr/README-status.md) | `8ddcb44` |
| Français | `docs/glossary.md` | [`fr/glossary.md`](fr/glossary.md) | `8ddcb44` |
| Français | `docs/integrating.md` | [`fr/integrating.md`](fr/integrating.md) | `8ddcb44` |
| Español | `README.md` → "Status" | [`es/README-status.md`](es/README-status.md) | `8ddcb44` |
| Español | `docs/glossary.md` | [`es/glossary.md`](es/glossary.md) | `8ddcb44` |
| Español | `docs/integrating.md` | [`es/integrating.md`](es/integrating.md) | `8ddcb44` |

Each file records the source commit it tracks in a note at the top. That commit
is the English text the translation was made from, not the commit that added
the translation. A translation is **stale** when the English source has changed
since the recorded commit; the note says so explicitly rather than implying
that an old translation still describes the current code.

## Checking for stale translations

`scripts/check-translations.sh` (`make check-translations`) computes what the
paragraph above promises. For each row of the table it:

- fails when the translated file is missing, is not in the table, or its note
  names a different source, section or source commit than the table;
- fails when the recorded commit does not resolve — in CI that means a shallow
  checkout, which is why the `Translations` job uses `fetch-depth: 0`;
- counts the commits since the recorded commit that changed the English
  source and lists every translation that is behind, with that count. For
  `README.md` → "Status" only the text of that section is compared at each
  commit, so a change elsewhere in the README does not make it stale.

Staleness is a warning, not a failure: an English fix is never blocked on a
translator. In CI a stale translation becomes a warning annotation on a pull
request that touches its English source (`--changed-since origin/<base>`);
the table and note disagreeing fails the job.

```sh
./scripts/check-translations.sh
make test-check-translations   # the script's own tests, against a scratch repository
```

## Updating a translation

1. Read the current English document and the recorded source commit.
2. Translate the changed sections. Do not change code, commands, identifiers,
   URLs, issue numbers or quoted output.
3. Update the `Source commit` note in the translated file and the table above
   to the same commit, then run `./scripts/check-translations.sh`.
4. Leave the English document unchanged unless the change itself is a
   correction; a translation must not become a second source of truth.

Terminology follows the English glossary even when a literal translation would
be shorter: `clear`, `stale`, `undetermined` and `not listed` are states with
specific meanings, and the translated explanation keeps the English term in
parentheses the first time it appears.
