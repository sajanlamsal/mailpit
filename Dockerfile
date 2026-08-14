# syntax=docker/dockerfile:1

# =============================================================================
# builder
# =============================================================================
# Pin to the minor version in go.mod's `go` directive. `golang:alpine` floats
# across Go minor releases, so a silent toolchain upgrade can break the build
# with no change on your side.
FROM golang:1.24-alpine AS builder

# Toolchain. Changes only when this line changes, so it caches ~forever.
# `apk upgrade` removed deliberately: it makes the same Dockerfile produce
# different images depending on the day. Bump the base tag instead.
RUN apk add --no-cache git npm

WORKDIR /app

# Go dependencies. Cache key is go.mod/go.sum only, so this survives every
# build that does not change dependencies.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# JS dependencies. Cache key is the lockfiles only.
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

# Source. Everything below this line reruns when any tracked file changes;
# everything above it does not.
COPY . .

RUN npm run package

# `go run pkg@version` instead of `go install`: with the module cache mounted
# this needs the network only on a cold cache, and it does not leave the
# go-licenses binary in the layer. Keeps the report always correct with no
# committed-file staleness to police.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go run github.com/google/go-licenses/v2@v2.0.1 report ./... \
      --template .github/third-party-licenses.tpl \
      --ignore github.com/axllent/mailpit \
      > internal/licenses/third-party.txt

# VERSION is declared as late as possible. Any layer that references a build
# arg is invalidated when that arg changes - and the image tag is unique on
# every build - so this must be the last layer, not the first.
#
# The import path in -X must match go.mod's `module` line exactly. If it does
# not, -X silently does nothing and Version stays "dev". Do not "fork-ify" it.
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/axllent/mailpit/config.Version=${VERSION}" \
      -o /mailpit

# =============================================================================
# runtime
# =============================================================================
# Pin this too - verify the current stable Alpine and bump deliberately.
FROM alpine:3.22

# Placed BEFORE the COPY: the binary changes every build, so anything after it
# is invalidated every build. Package installs belong above.
# ca-certificates is needed for mailpit's SMTP relay / webhook TLS - drop it if
# you use neither.
RUN apk add --no-cache tzdata ca-certificates \
 && adduser -D -u 10001 -h /home/mailpit mailpit

LABEL org.opencontainers.image.title="Mailpit (JoBins)" \
  org.opencontainers.image.description="An email and SMTP testing tool with API for developers - JoBins fork" \
  org.opencontainers.image.source="https://github.com/JoBinsJP/mailpit" \
  org.opencontainers.image.url="https://github.com/JoBinsJP/mailpit" \
  org.opencontainers.image.documentation="https://mailpit.axllent.org/docs/" \
  org.opencontainers.image.vendor="JoBins" \
  org.opencontainers.image.licenses="MIT"

COPY --from=builder /mailpit /mailpit

# Version-bearing labels last: they change every build, so they must not sit
# above anything worth caching.
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${REVISION}"

# Mailpit binds 1025/1110/8025, all above 1024, so no capability is needed.
# NOTE: if your helm chart mounts a volume for MP_DATABASE, that volume must be
# writable by UID 10001 - set fsGroup: 10001 in the pod securityContext.
USER mailpit

EXPOSE 1025/tcp 1110/tcp 8025/tcp

# Inert under Kubernetes (it ignores Docker HEALTHCHECK and uses the chart's
# probes instead). Kept for local `docker run`.
HEALTHCHECK --interval=15s --start-period=10s --start-interval=1s CMD ["/mailpit", "readyz"]

ENTRYPOINT ["/mailpit"]