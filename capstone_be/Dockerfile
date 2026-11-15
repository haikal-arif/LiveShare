# syntax=docker/dockerfile:1

# ---- build: static Go binary ----
FROM golang:1.26.5-alpine AS build
WORKDIR /src

# Module download first, so it is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd cmd
COPY internal internal
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /webserver ./cmd/webserver

# Empty directory for the gateway blocklist. Created here so the named volume
# that backs it starts out owned by the nonroot user the webserver runs as.
RUN mkdir -p /out/blocklist

# ---- runtime: the binary and nothing else, non-root ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /webserver /webserver
COPY --from=build --chown=65532:65532 /out/blocklist /var/lib/capstone/blocklist

# Configuration is environment only: ROOM_PASSCODE, JWT_SECRET and
# SFU_BASE_URL are required (see cmd/webserver/main.go).
ENV ADDR=:8080 \
    GIN_MODE=release
# Where blocked IPs are written for the gateway to enforce (see gateway/).
ENV BLOCKLIST_FILE=/var/lib/capstone/blocklist/blocklist.conf
EXPOSE 8080

ENTRYPOINT ["/webserver"]
