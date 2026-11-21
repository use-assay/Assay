# Should Assay host a public `assay serve` instance?

**Decision (2026-11-20): no public instance for now.** The project publishes
the scanner, the container image, and the on-chain registry, but does not
operate a shared HTTP endpoint. An evaluator runs `assay serve` locally or from
the container; there is no link to a hosted scanner.

This is a decision, not an oversight. It is recorded here because a public
instance would change a threat-model assumption, and because "just host it"
looks cheap until the upstream and abuse costs are written down.

## What exists today

| Surface | Hosted by the project? |
| --- | --- |
| Testnet safety registry (`get_safety`) | Yes — a contract on Stellar testnet |
| Scanner binary and container image | Published, run by the user |
| `assay serve` HTTP API and UI | No hosted instance |
| Attestation writes | Operator-run, not a public service |

The registry answers on-chain reads without an Assay-operated web service. The
HTTP API is a convenience for humans and integrations, not a dependency of the
gate.

## Why a public instance would help

- **Evaluation is friction.** A link is the shortest path from "what is this?"
  to a rendered verdict. Requiring a Go toolchain or Docker filters out most
  curious readers.
- **One consistent answer.** A hosted instance can pin the binary version,
  curated-source configuration, and clock, so two people comparing a verdict
  are comparing the same scanner rather than two builds.
- **Demonstration of freshness.** A public instance with visible
  `attested_at`/evidence-age headers shows the temporal model working, which a
  screenshot cannot.
- **Bug reports get a reproduction URL.** "Scan this asset on the public
  instance" is more precise than a description.

## What it would cost

### Upstream load

Every uncached scan calls Horizon and StellarExpert. Those are shared public
services with rate limits that Assay does not control. A public instance turns
each visitor into upstream traffic, and `/api/v1/exposure` multiplies one
request into up to 50 scans. The caching policy in [caching.md](caching.md)
reduces repeated work, but it does not make an unbounded public audience safe
for the upstreams.

### Abuse

The API has no authentication. A public endpoint can be used to:

- drive scan volume and exhaust the instance's upstream allowance;
- enumerate assets or probe the scanner's behavior at scale;
- consume CPU and memory through the exposure endpoint;
- use the project's domain and reputation as a proxy for someone else's
  traffic.

Only `/api/v1/exposure` has a per-client token bucket today. There is no global
per-IP limit, concurrency cap, request-body limit beyond the standard library's
defaults, or egress budget tied to a public audience.

### Operational surface

A public instance needs monitoring, alerting, log retention, an abuse contact,
a patching cadence, and a budget. None of those exist for a service the project
does not run. `/metrics` and `/readyz` provide the measurements; they are not a
monitoring system.

### Threat model

The threat model currently puts denial of service out of scope for a specific
reason:

> Denial of service against Assay's own HTTP API — Pre-1.0 and not deployed as a
> service.

Hosting one would make that sentence false. The threat model would have to move
API DoS in scope, state the mitigations, and say what an outage does to a
consumer. That is a design change, not a deployment detail.

## Prerequisites

The proposed change for this decision names rate limiting, CORS, a container,
and monitoring. Their status at the time of the decision:

| Prerequisite | Status | Tracking issue |
| --- | --- | --- |
| Container image (static, non-root, healthcheck, history volume) | Implemented; documented in [serving.md](serving.md) | [#432](https://github.com/use-assay/Assay/issues/432) |
| `Cache-Control` and oldest-evidence-age headers | Implemented; documented in [api.md](api.md) | [#430](https://github.com/use-assay/Assay/issues/430) |
| UI/API schema agreement with a loud reload prompt | Implemented; `/api/v1/meta` and a per-response check | [#431](https://github.com/use-assay/Assay/issues/431) |
| CORS policy for browser callers | Implemented; documented in [api.md](api.md) | [#433](https://github.com/use-assay/Assay/issues/433) (decision record) |
| Global per-client rate limit and concurrency cap | Not implemented; only `/api/v1/exposure` is bounded | [#433](https://github.com/use-assay/Assay/issues/433) (umbrella until a dedicated issue exists) |
| Monitoring, alerting, and an abuse contact | Endpoints exist (`/metrics`, `/readyz`); no hosted monitoring or contact | [#433](https://github.com/use-assay/Assay/issues/433) (umbrella until dedicated issues exist) |
| Threat-model revision moving API DoS in scope | Not done; required before hosting | [#433](https://github.com/use-assay/Assay/issues/433) |

The container and header work are necessary but not sufficient: the remaining
prerequisites are operational, and they are the reason the decision is still
"no".

## What would have to be true first

A public instance is reconsidered only when all of these hold:

1. **A bounded inbound budget.** A global per-client rate limit, a concurrency
   cap, and an explicit upstream request budget exist and are tested. The
   exposure endpoint's limiter is not a substitute.
2. **Monitoring and response.** `/metrics` and `/readyz` are scraped, alerts
   fire on error rate and upstream failure, and someone is named to respond.
3. **Threat model updated.** API denial of service is in scope, with its
   mitigations and the consumer-visible consequence of an outage.
4. **Abuse handling.** A contact address, a takedown/blocking procedure, and a
   log-retention policy exist.
5. **Coverage worth linking.** The registry covers enough assets that a public
   lookup is useful, and the status page states the coverage limit rather than
   letting "not found" read as "safe". The current 10 attested assets do not
   meet that bar.
6. **A stable deployment.** The image is built reproducibly, its digest is
   published, and a redeploy does not lose history.

Until then, the honest public artifact is the source, the image, and the
on-chain registry — not a URL that implies a service the project does not run.

## What an evaluator should do instead

```sh
docker build -t assay .
docker run --rm -p 8080:8080 assay
# open http://127.0.0.1:8080
```

Or, with a Go toolchain:

```sh
go run ./cmd/assay serve -addr :8080
```

Both are documented in [serving.md](serving.md). The on-chain registry is
callable directly on testnet; see [deployment.md](deployment.md).
