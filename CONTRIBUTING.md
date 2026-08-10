# Contributing to Assay

## Ground rules

Assay makes claims about whether someone's money can be taken. Two rules
follow from that, and they are not negotiable:

**Never display a value you did not fetch.** No placeholder balances, no
example ratings, no "typical" flag sets. If a source is unreachable, the
output says the source is unreachable.

**Never re-derive a consumed signal.** StellarExpert's directory, ratings,
and blocklist are inputs. Assay attributes them to their source and passes
them through. If you find yourself writing a scam heuristic over domain
names, stop — that layer already exists and is better maintained than
anything we would write.

## Adding a check

A check is not done when it detects something. It is done when its
*judgment* has been measured.

1. Implement `mechanics.Check`.
2. Verify every flag name, field name, and endpoint against a live source
   before encoding it. Cite what you verified in the PR.
3. Add it to the labelled set in `docs/eval.md` with at least one asset that
   should trip it and one that has the same mechanics *legitimately* and
   should not be over-flagged.
4. Record the eval result. **A check whose judgment isn't evaluated against
   that set doesn't ship.**

Point 3 is the whole discipline. Any check can find `auth_revocable: true`;
the reason to have a check at all is that it knows when that is fine.

## Development

```sh
make test     # tests with -race
make lint     # golangci-lint
make cover    # coverage report
make run      # start the API on :8080
```

Run `make fmt` before committing; CI enforces `gofmt -l` being empty.

Tests must not require network access. Fetchers are interfaces; tests use
fixtures captured from real responses under `internal/*/testdata/`. When you
capture a new fixture, note the date and the URL it came from.

## Commits

Present tense, explain the why when it isn't obvious. Keep unrelated changes
in separate commits. Don't commit binaries, coverage output, or logs.
