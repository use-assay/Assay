# Serving Assay

`assay serve` runs the HTTP API and the embedded UI. There are two supported
ways to run it: the released binary with a Go toolchain, or the container image
built from the repository. Neither is a public hosting service — see
[public-instance.md](public-instance.md) for the decision about that.

## The container image

The image is a multi-stage build: a pinned Go toolchain compiles a static
binary, and the runtime stage is `scratch`. It contains only the binary, CA
roots (every upstream Assay reads is HTTPS), timezone data, and an empty
`/data` directory. There is no shell, package manager, or curl.

```sh
docker build -t assay .
docker run --rm -p 8080:8080 assay
```

The image runs as UID/GID `65532:65532`, never as root. `docker inspect` shows
it:

```sh
docker inspect --format '{{.Config.User}}' assay
# 65532:65532
```

### Healthcheck

The runtime has no shell, so the image's `HEALTHCHECK` invokes the binary
itself:

```sh
assay healthcheck
```

It requests `http://127.0.0.1:8080/healthz` and succeeds only on `200` with the
body `ok`. A non-200 answer, a connection failure, or an unexpected body is an
error, so "something replied" never reads as healthy. Point it elsewhere with
`-url`, or bound the request with `-timeout`:

```sh
assay healthcheck -url http://127.0.0.1:9090/healthz -timeout 2s
```

The container starts with `serve -addr :8080 -history /data/history.jsonl`, so
a mounted volume keeps the observation history across container replacement.

### Persisting history

```sh
docker volume create assay-history
docker run --rm -p 8080:8080 -v assay-history:/data assay
```

The directory is created with the runtime UID/GID in the image, so a fresh
named volume inherits an owner the process can write to. A host bind mount must
be writable by `65532:65532`; without a mount, history is written inside the
container and disappears with it. The retention and file format are documented
in [history.md](history.md).

## Running from source

```sh
go run ./cmd/assay serve -addr :8080
go run ./cmd/assay serve -addr :8080 -history /var/lib/assay/history.jsonl
```

`make run` builds the binary and starts it on `:8080`.

## What the surface exposes

| Path | Purpose |
| --- | --- |
| `/` | The single-file UI |
| `/api/v1/meta` | The report schema and route version the server speaks |
| `/api/v1/scan?asset=CODE-ISSUER` | Scan one asset |
| `/api/v1/history?asset=CODE-ISSUER` | Recorded observations and transitions |
| `/api/v1/exposure?account=G...` | Per-asset exposure for one account |
| `/metrics` | Scan outcome and source-failure counters |
| `/healthz` | Liveness; checks nothing else |
| `/readyz` | Readiness; asks the upstreams |

[docs/api.md](api.md) documents every endpoint, and
[docs/api-versioning.md](api-versioning.md) states what `/api/v1` and
`schema_version` promise.

The embedded UI calls `/api/v1/meta` on load and checks each scan response
before rendering it. A page whose expected `schema_version` differs from the
server's shows a reload prompt and refuses to render the body rather than
misreading it.

## Caching and freshness

Dynamic responses carry `Cache-Control: no-store`; the UI carries
`Cache-Control: no-cache` so a browser revalidates it instead of pairing an old
page with a newer API. A scan response also carries
`X-Assay-Oldest-Evidence-Age`, the age in seconds of the oldest evidence
timestamp that contributed to it. That header is data age, not the RFC 9111
`Age` of a cached response.

## Before exposing it to anyone

The server has no authentication, no TLS termination, and no global per-client
rate limit (only `/api/v1/exposure` is bounded). It is intended for local use,
CI, and a reverse proxy that supplies those controls. A public instance would
change the threat model; the prerequisites and the current decision are in
[public-instance.md](public-instance.md).
