# syntax=docker/dockerfile:1

# =============================================================================
# builder
# =============================================================================
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git npm

WORKDIR /app

# GODEBUG=http2client=0 works around proxy.golang.org resetting HTTP/2
# streams mid-download ("stream error ... INTERNAL_ERROR"). Retry loop
# covers other transient proxy/network failures.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    export GODEBUG=http2client=0 && \
    n=0; \
    until go mod download; do \
        n=$((n+1)); \
        if [ "$n" -ge 3 ]; then \
            echo "go mod download failed after ${n} attempts" >&2; \
            exit 1; \
        fi; \
        echo "go mod download failed (attempt ${n}/3), retrying in 5s..."; \
        sleep 5; \
    done

COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    npm ci

COPY . .

RUN npm run package

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    export GODEBUG=http2client=0 && \
    go run github.com/google/go-licenses/v2@v2.0.1 report ./... \
      --template .github/third-party-licenses.tpl \
      --ignore github.com/axllent/mailpit \
      > internal/licenses/third-party.txt

ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X github.com/axllent/mailpit/config.Version=${VERSION}" \
      -o /mailpit

# =============================================================================
# runtime
# =============================================================================
FROM alpine:3.22

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

ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.version="${VERSION}" \
  org.opencontainers.image.revision="${REVISION}"

USER mailpit

EXPOSE 1025/tcp 1110/tcp 8025/tcp

HEALTHCHECK --interval=15s --start-period=10s --start-interval=1s CMD ["/mailpit", "readyz"]

ENTRYPOINT ["/mailpit"]