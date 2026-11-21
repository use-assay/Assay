# syntax=docker/dockerfile:1

# Build stage. The toolchain is pinned to go.mod's version so the image and CI
# compile with the same Go release.
FROM golang:1.26.11-bookworm AS build

WORKDIR /src

# Download modules first so a source-only change does not invalidate the cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Same reproducibility flags as `make build-release`: no absolute paths, no VCS
# stamp, no C toolchain. The binary is static, which is what lets the runtime
# stage be scratch.
RUN CGO_ENABLED=0 go build \
        -trimpath \
        -buildvcs=false \
        -ldflags='-s -w' \
        -o /out/assay ./cmd/assay

# The history volume's mount point. It is created with the runtime UID/GID so a
# fresh named volume inherits an owner the non-root process can write to.
RUN install -d -o 65532 -g 65532 /out/data

# Runtime stage: scratch, with only the binary, CA roots (upstreams are HTTPS),
# timezone data, and the writable data directory.
FROM scratch

LABEL org.opencontainers.image.title="Assay" \
      org.opencontainers.image.description="Mechanical trap-scanner and on-chain safety gate for Stellar assets" \
      org.opencontainers.image.source="https://github.com/use-assay/Assay" \
      org.opencontainers.image.licenses="Apache-2.0"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/assay /assay
COPY --from=build --chown=65532:65532 /out/data /data

# Numeric, so the image needs no /etc/passwd and never runs as root.
USER 65532:65532

EXPOSE 8080

# Named volumes preserve the observation history across container replacement.
VOLUME ["/data"]

# The runtime is scratch, so there is no shell or curl. The binary probes its
# own /healthz; an unhealthy answer fails the container's health status.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/assay", "healthcheck"]

ENTRYPOINT ["/assay"]
CMD ["serve", "-addr", ":8080", "-history", "/data/history.jsonl"]
