# syntax=docker/dockerfile:1

# ---- build: compile the server crate in release mode ----
FROM rust:1-slim-bookworm AS build
WORKDIR /src

# Smaller binary: strip symbols, thin LTO. Set here so Cargo.toml (and
# plain `cargo build`) stay untouched.
ENV CARGO_PROFILE_RELEASE_STRIP=symbols \
    CARGO_PROFILE_RELEASE_LTO=thin

COPY Cargo.toml Cargo.lock ./
COPY crates crates

# The registry and target dir live in cache mounts, so rebuilds only
# recompile what changed. The binary is copied out because cache mounts
# are not part of the image.
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/src/target \
    cargo build --release --locked -p server \
    && cp target/release/server /server

# ---- runtime: glibc + libgcc only, no shell or package manager, non-root ----
FROM gcr.io/distroless/cc-debian12:nonroot

COPY --from=build /server /server

# Signaling websocket and internal REST (/internal/*) share this TCP port;
# only the reverse proxy in front of it should ever reach it. Media is
# UDP, one port per PeerConnection, from SFU_UDP_PORTS.
ENV BIND_ADDR=0.0.0.0:9000 \
    SFU_UDP_PORTS=50000-50199
EXPOSE 9000/tcp
EXPOSE 50000-50199/udp

ENTRYPOINT ["/server"]
