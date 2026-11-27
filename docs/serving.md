# Operating `assay serve`

`assay serve` runs the HTTP API and the single-page UI from one binary. This
page is for whoever stands an instance up: which flags matter, where state
lives, which probe to wire to what, what to scrape, and what a public instance
must add. The endpoints themselves are specified in [api.md](api.md); this page
does not repeat them.

Container images (#432) and a hosted instance (#433) are out of scope here.

## Flags

Every flag below is defined by `assay serve` itself; `assay serve -h` prints the
same set. [cli.md](cli.md) carries the usage text verbatim.

| Flag | Default | What it does |
| --- | --- | --- |
| `-addr` | `:8080` | Listen address. Bind `127.0.0.1:PORT` behind a reverse proxy so the server is not reachable around it. |
| `-history PATH` | empty | Observation history log (JSON Lines). Empty keeps history **in memory only**: it is lost on restart. See [State](#state-the-history-log). |
| `-history-retention N` | `256` | Observations kept per asset (minimum 1). |
| `-observe FILE` | empty | Asset list (one `CODE-ISSUER` per line, `#` comments) to scan on a schedule, recording each result in history. |
| `-observe-interval D` | `6h` | Interval between scheduled scans of `-observe`. The first round runs at startup. |
| `-trusted-proxies CIDR[,CIDR...]` | empty | Peers whose `X-Forwarded-For` is trusted when keying rate limits. Required behind a proxy; see [Reverse proxy](#reverse-proxy). |
| `-attested FILE` | empty | Attested-set file in the [`attested.json`](../attested.json) format. `/readyz` then reports the registry's freshness for that set. |
| `-registry-rpc URL` | `$ASSAY_RPC_URL`, else `https://soroban-testnet.stellar.org` | Soroban RPC that `-attested` reads `get_safety` through. |
| `-staleness-window D` | the file's `cadence` | Age after which an attestation in the `-attested` set counts as stale. |
| `-asset-lists URL[,URL...]` | none | SEP-0042 asset lists consumed as a curation source ([asset-lists.md](asset-lists.md)). |
| `-cache-directory-ttl D` | `20s` | How long a StellarExpert directory answer is reused ([caching.md](caching.md)). |
| `-cache-blocklist-ttl D` | `20s` | How long a StellarExpert blocklist answer is reused. |
| `-no-cache` | off | Re-fetch curated sources and asset lists on every scan. |

Fixed behaviour, not configurable by flag:

- **Timeouts.** Read-header 10s, write 60s, idle 120s. The write timeout bounds
  every response, including a `GET /api/v1/events` stream, so SSE clients must
  expect to reconnect at least once a minute.
- **Shutdown.** `SIGINT` or `SIGTERM` starts a graceful shutdown: the listener
  closes and in-flight requests get up to 10s to finish. The history log is
  closed (and its lock released) on the way out.
- **Logs.** `log/slog` text lines on **stderr**. Each request's lines carry its
  `X-Request-Id` ([api.md](api.md#request-identifiers)) and, behind a proxy,
  the `client_key` used for rate limiting. There is no log file; let the
  supervisor (journald, the container runtime) collect stderr.

## State: the history log

The only state the server writes is the observation history. Every successful
`GET /api/v1/scan` and every scheduled `-observe` scan appends one observation.
[history.md](history.md) is the full specification; what an operator needs:

- **Where it lives.** At `-history PATH`. Without it, history is in memory and
  every restart starts empty — fine for a throwaway instance, wrong for one
  whose `/api/v1/history` anyone relies on.
- **How it is written.** The whole log is rewritten on each append to a
  temporary file that is renamed over `PATH`, and the directory is synced, so
  after each append the file on disk is complete. Put it on a local filesystem
  you can write and `fsync`.
- **One writer.** `serve` takes an exclusive lock on a sidecar `PATH.lock` for
  as long as it runs. A second process pointed at the same path fails at
  startup with "already in use by another process" rather than silently
  overwriting the first one's observations (#391). Consequence: **replicas
  cannot share a history file.** Give each replica its own path, or run one
  instance. On a platform without `flock` the lock is a no-op and the
  guarantee is yours to keep.
- **Size.** Bounded by two caps: `-history-retention` observations per asset and
  512 assets, least-recently-observed evicted first, so the log never exceeds
  `512 × retention` records whatever outside callers scan.
- **Backups.** Because every append replaces the file by rename, copying
  `PATH` at any moment yields a complete log (the last complete append). A
  periodic `cp` or snapshot of the state directory is enough. To restore, stop
  the server, put the file back at `PATH`, start it. Do not copy `PATH.lock`.

## Probes

| Endpoint | Answers | Checks | Wire it to |
| --- | --- | --- | --- |
| `GET /healthz` | Is the process up? | Nothing; always `200 ok` while the process can answer. | **Liveness.** Restart the process when this fails. |
| `GET /readyz` | Can scans succeed right now? | Horizon and the consumed sources, cached for 10s between rounds. | **Readiness**, with care — see below. Also a dashboard. |

`/readyz` returns `503` for `unavailable` (Horizon unreachable: no scan can
succeed) **and** for `degraded` (a consumed source such as the StellarExpert
directory is unreachable: scans still succeed, with the reputation axis
`undetermined`). Every replica reads the same upstreams, so a StellarExpert
outage makes **every** replica `degraded` at once. A readiness probe that acts
on the status code alone then pulls every replica out of rotation and turns a
partial outage — scans still served, honestly marked undetermined — into a
total one.

Choose deliberately:

- If serving undetermined answers during a curated-source outage is acceptable
  (it usually is: the report says so per scan), make readiness parse the body
  and fail only on `"status": "unavailable"`, or use `/healthz` for both and
  alert on `/readyz` separately.
- If a degraded answer is worse than none for your consumers, keep the plain
  status-code probe and accept that a shared upstream outage takes the service
  down.

Never point a liveness probe at `/readyz`: an upstream outage would restart
healthy processes in a loop, and the restart fixes nothing.

`/readyz` also carries `registry_staleness`, which never affects its status
code. Configure it with `-attested` to see the on-chain attested set going stale
before consumer gates start refusing it ([api.md](api.md#get-readyz)).

## Metrics

`GET /metrics` returns JSON counters accumulated since process start:
`scan_outcomes` (per class: `complete`, `undetermined`, `not_found`,
`upstream_failure`) and `source_failures` (per upstream that did not answer).
They are monotonic and reset on restart; derive rates from consecutive scrapes.
It is not the Prometheus text format, so a Prometheus scrape needs a JSON
exporter in front.

Worth alerting on: a rising share of `undetermined` or `upstream_failure`, and
any single `source_failures` entry climbing — that is the same signal
`/readyz` reports as `degraded`, measured over real traffic instead of a probe.

`/metrics` is unauthenticated. On a public instance, do not proxy it; scrape it
on the loopback address.

## Rate limits

- **`/api/v1/exposure`** is always rate limited per client: a burst of 5, then
  1 request per second. One request runs a full scan per trustline on the
  account, which is why. Excess requests get `429` with `Retry-After`.
- **`/api/v1/events`** accepts at most 100 concurrent SSE connections per
  instance; the 101st gets `429`.
- **`/api/v1/scan`** is not rate limited by `assay serve`. Each request makes
  live calls to Horizon and the consumed sources, so on a public instance put
  a per-client limit on it at the proxy (below), or upstream providers will
  rate-limit *you*.

Limits are keyed on the client address. Behind a proxy that is the proxy's
address — every client sharing one bucket — unless `-trusted-proxies` names the
proxy ([api.md](api.md#reverse-proxies-and-rate-limiting)).

## Reverse proxy

A public instance should sit behind a proxy that terminates TLS, with `assay`
bound to loopback and the proxy named in `-trusted-proxies`:

```sh
assay serve -addr 127.0.0.1:8080 -trusted-proxies 127.0.0.1/32 \
  -history /var/lib/assay/history.jsonl
```

```nginx
limit_req_zone $binary_remote_addr zone=assay_scan:10m rate=2r/s;

server {
    listen 443 ssl;
    http2 on;
    server_name assay.example.com;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Scans make live upstream calls; assay serve does not limit them.
    location = /api/v1/scan {
        limit_req zone=assay_scan burst=10 nodelay;
        limit_req_status 429;
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }

    # Server-Sent Events: do not buffer, and outlive the 15s heartbeat.
    location = /api/v1/events {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
        proxy_read_timeout 1h;
    }

    # Operator-only endpoints stay on loopback.
    location = /metrics { return 404; }
}
```

The rate above is an example, not a recommendation measured against upstream
quotas; size it to your traffic.

## systemd

[`examples/systemd/assay.service`](../examples/systemd/assay.service) is a
hardened unit. Its load-bearing settings:

- `DynamicUser=yes` — a transient, unprivileged user; nothing to create or
  clean up.
- `StateDirectory=assay` — systemd creates `/var/lib/assay` owned by that user
  and keeps it across restarts; the unit points `-history` there (`%S/assay`).
- `ProtectSystem=strict` — the whole filesystem is read-only to the service
  except the state directory.
- `NoNewPrivileges=yes` — nothing the process executes can gain privileges.

It also drops all capabilities (the default port is unprivileged), restricts
address families to IP and Unix sockets, and limits system calls to the
`@system-service` set. Install:

```sh
sudo install -m 0755 assay /usr/local/bin/assay
sudo install -m 0644 examples/systemd/assay.service /etc/systemd/system/assay.service
sudo systemd-analyze verify /etc/systemd/system/assay.service
sudo systemctl daemon-reload
sudo systemctl enable --now assay
journalctl -u assay -f
```

To add flags (`-attested`, `-observe`, `-asset-lists`), use
`systemctl edit assay` and override `ExecStart=` (an empty `ExecStart=` line
first clears the original). Files those flags read must be readable by the
dynamic user: put them under `/etc/assay/` (world-readable) or in the state
directory. `-attested`'s registry reads and `-asset-lists` fetches need
outbound HTTPS, which the unit allows.

## Checklist for a public instance

- [ ] `-addr` on loopback, TLS at a proxy, `-trusted-proxies` naming it.
- [ ] A per-client limit on `/api/v1/scan` at the proxy.
- [ ] `/metrics` not exposed publicly.
- [ ] `-history` on persistent storage, one writer per file, backed up.
- [ ] Liveness on `/healthz`; a readiness policy chosen with the
      [degraded case](#probes) in mind.
- [ ] Logs collected from stderr.
- [ ] Optionally `-attested` so `/readyz` shows registry staleness.

## Related

- [api.md](api.md) — every endpoint and its responses.
- [cli.md](cli.md) — the usage text for every command, `serve` included.
- [history.md](history.md) — history semantics, durability and bounds.
- [caching.md](caching.md) and [asset-lists.md](asset-lists.md) — the scan-side
  flags `serve` shares with `scan`.
- [source-outage.md](source-outage.md) — what to do when an upstream fails.
