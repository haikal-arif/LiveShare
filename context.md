# Project context

Read this first. It summarizes the project so you don't need a full repo scan. The API contract is in `contracts.md` and is the source of truth for every endpoint and message; this file only points to it and records decisions, layout and known gaps.

## What the app is

A video share app ("FlashLive"). One room per server. The admin opens a room, streamers publish camera/screen, and watchers view the streams laid out on a venue stage. Room state lives in memory only (lost on restart).

## Components and boundaries

| Component | Dir | Role |
| --- | --- | --- |
| Go webserver | `capstone_be/` | Public REST + SSE. Owns rooms, admin auth, streamer registry, stage layout, blacklist. |
| Rust SFU | `capstone_sfu/` | Media only (WebRTC/RTP), signaling over websocket, simulcast layer selection. |
| Frontend | `capstone_fe/` | SvelteKit 2 / Svelte 5 single-page app (static build). Keeps `admin_token` and `streamer_id` in `localStorage`. |
| Gateway | `gateway/` | nginx. The only public entry point in the Docker setup; puts the three above behind one origin. |
| TURN relay | `turn` service in `docker-compose.yml` | coturn. Optional (`TURN=1`); relays media for clients whose network blocks direct UDP. |
| Terraform | `terraform/ec2-deploy/` | Creates the EC2 instance, security group and Elastic IP for the public deployment. |
| Docs | `capstone_docs/` | Weekly reports, product overview, architecture, API contract copy, EC2 and TURN guides. |

Since 2026-11-15 this folder is one git repo (monorepo); the components are plain folders in it, not separate repos or submodules.

Three boundaries (details in `contracts.md`):
1. Browser → Go: REST, public.
2. Go → SFU: REST (`/internal/*`), internal only, never public.
3. Browser → SFU: websocket signaling (`/ws`), public. Media itself is UDP, browser ↔ SFU.

## Running it

Two ways; both use the same ports and env names.

**Local dev** (needs Go, Rust, Node):
- `make setup` writes a `.env` in `capstone_be/`, `capstone_sfu/`, `capstone_fe/` (never overwrites) and installs frontend deps.
- `make dev` runs all three (`make sfu|be|fe` for one); `make test` runs all unit tests; `make build` builds all three. `start-dev.sh` does the same as `make dev` with prefixed logs.
- The backend reads `ROOM_PASSCODE`, `JWT_SECRET`, `SFU_BASE_URL` (required) and `ADDR`, `SFU_WS_URL`, `ALLOWED_ORIGINS`, `TRUSTED_PROXIES`, `ICE_SERVERS`, `TURN_URLS`/`TURN_USERNAME`/`TURN_CREDENTIAL` (optional TURN entry appended to `ice_servers`) and `BLOCKLIST_FILE` (optional; where blocked IPs are written for the gateway) from `capstone_be/.env` or the environment.
- The SFU reads `BIND_ADDR` (default `0.0.0.0:9000`), `SFU_UDP_PORTS` (default `50000-50199`), `SFU_PUBLIC_IPS` (comma-separated, default none; see "Media addresses" below), `ICE_SERVERS_JSON`, `TURN_URLS`/`TURN_USERNAME`/`TURN_CREDENTIAL` (same meaning as the backend's) and `MAX_PARTICIPANTS` (most peers per room, sharers and watchers together; `0` or unset = no limit; a join past it gets `error` `invalid_state` "room is full"). `make setup` writes `BIND_ADDR=127.0.0.1:9000` into `capstone_sfu/.env`, and the Makefile loads that file (and `capstone_be/.env`) and exports it to `make sfu|be|dev`; neither program reads a `.env` itself. Run outside `make` without `BIND_ADDR`, the SFU binds all interfaces and `/internal/*` is reachable from the network. A `capstone_sfu/.env` created before 2026-11-17 still has the old `SFU_ADDR` key, which nothing reads: delete it and run `make setup`.

**Docker** (everything behind the gateway):
- `make docker-setup` creates or updates the compose `.env` through `scripts/docker-env.sh` (existing values are kept unless an option changes them) and writes the gateway's cert (`gateway/gen-cert.sh`: signed by mkcert when it's installed, self-signed otherwise; covers localhost and this machine's IPs). Options: `HOST_IP=`, `EC2=1` (public IP from instance metadata), `DOMAIN=`, `HTTPS_PORT=`, `HTTP_PORT=`, `TURN=1|0`. `make docker-up` builds and starts; `make docker-down` stops; `make docker-logs` tails; `make tls DOMAIN= EMAIL=` installs a Let's Encrypt cert (EC2); `make run-all` is the full LAN run (detect IP, refresh cert, open firewall, up, tear down on Ctrl-C).
- Open `https://<APP_HOST_IP>:8443` (HTTPS is required for camera/mic; with a self-signed cert each device must accept it once). Locally port 80 is not used because a host nginx may hold it; on EC2 the ports are 443 and 80.
- TURN: `TURN=1` sets `APP_TURN_ENABLED`, `APP_TURN_URLS` and `COMPOSE_PROFILES=turn`, which starts the `turn` service (coturn, host networking, 3478 udp/tcp, 5349 tls, relay UDP 49152-49200). See `capstone_docs/turn-relay.md`.
- EC2: `capstone_docs/ec2-deployment.md` is the procedure (Terraform in `terraform/ec2-deploy/`, then `make docker-setup EC2=1 ...`, `make tls`, `make docker-up`). Terraform state is local and git-ignored.
- `.env` variables are all `APP_*` so they never clash with the per-component `.env` files that the Makefile exports: `APP_HOST_IP` (LAN IP, detected from the default route), `APP_ROOM_PASSCODE`, `APP_JWT_SECRET`, `APP_HTTPS_PORT` (8443), `APP_HTTP_PORT` (8088), `APP_UDP_PORTS` (50000-50199), `APP_SUBNET` (172.28.0.0/24), `APP_PUBLIC_HOST` (hostname for links and the cert; same as the IP until `DOMAIN=` is given), `APP_EXTRA_IPS` (optional more addresses clients reach the machine on, set with `EXTRA_IPS=`; the derived `APP_SFU_PUBLIC_IPS` and `APP_ALLOWED_ORIGINS` include them, and the backend hands out `SFU_WS_URL=/ws` so each browser's socket goes to the host it loaded the page from), `APP_MAX_PARTICIPANTS` (0), `APP_TURN_ENABLED`, `APP_TURN_USER`, `APP_TURN_PASSWORD`, `APP_TURN_URLS`, `APP_TURN_RELAY_MIN`/`_MAX`, `COMPOSE_PROFILES`. If the LAN IP changes: delete `.env` and `gateway/certs/capstone.*`, run `make docker-setup`, then `make docker-up`.
- See "Docker setup" below for how it is wired.

## Where things are

- `contracts.md`: full API contract, settled decisions, open items. Read before changing any endpoint.
- `docker-compose.yml`, `Makefile`: Docker stack and dev tasks. `gateway/`: nginx image, config template, cert script, blocklist watcher. `scripts/docker-env.sh`: writes the compose `.env`. `terraform/ec2-deploy/main.tf`: the EC2 server. `loadtest/`: k6 + headless Chromium publisher load test.
- `capstone_be/`: Go module `capstonewebserver` (Go 1.26.5), layered:
  - `cmd/webserver/`: `main.go` (env config, starts server and liveness poll), `router.go` (builds repositories, usecases, handlers, middleware; registers every route).
  - `internal/domain/`: types and errors (`room.go`, `auth.go`, `layout.go`, `event.go`).
  - `internal/dto/`: request/response shapes.
  - `internal/usecase/`: all business rules: `room`, `streamer`, `layout`, `blacklist`, `events`, `liveness` (polls the SFU, see decision 11), `jwt` (admin token sign/verify). `usecase.go` declares every port (interface) the usecases depend on; `//go:generate` produces `mock_ports_test.go` with mockgen. usecase never imports `repository` or `httpclient`.
  - `internal/repository/`: in-memory `Store` (room, stage, streamers) and `BlacklistStore`. `GatewayBlocklist` wraps the blacklist and mirrors it into `BLOCKLIST_FILE` as nginx `deny` lines. Depends only on `domain`/`dto`.
  - `internal/httpclient/`: `HttpClient`, the SFU's internal REST API as `usecase.RequestClient`: `GetPublishers`, `GetPeerIP` (404 → `domain.ErrPeerNotFound`), `DisconnectPeer`, `CloseRoom`.
  - `internal/events/`: in-memory hub fanning events out to SSE streams.
  - `internal/handler/`: HTTP handlers (`room`, `layout`, `streamer`, `blacklist`, `events`) and their tests.
  - `internal/middleware/`: `auth` (admin JWT), `blacklist` (403 for blocked IPs), `ipsniffer` (client IP from `X-Real-IP`), `cors`. There is no rate limiter in Go; the gateway does it.
  - `internal/reqctx/`: caller-IP context key shared by `middleware` and `usecase`.
  - `e2e/`: black-box tests (`go test -tags e2e ./e2e/...`) that build the binary and run it against a fake SFU.
- `capstone_sfu/`: Rust workspace (`crates/common` shared message types, `crates/server`). Layered, enforced in CI by a grep that fails if `webrtc::`, `rtc::` or `axum::` appears under `domain/` or `service/`:
  - `domain/`: ids, peer, quality, typed `DomainError` (wire `code`), and the media **ports** (`MediaGateway`, `MediaSession`, ...).
  - `service/`: `RoomService` registry and the room actor (`actor`, `negotiation`, `quality`, `state`).
  - `infra/media/`: everything that touches `webrtc-rs` (engine, session, relay, keyframe, layer switcher, published track).
  - `transport/`: `ws` (signaling) and `http` (internal REST).
  - `tests/`: integration tests (signaling, media with real WebRTC peers, internal peers, shutdown).
- `capstone_fe/`: SvelteKit app, routes `home`, `create`, `join/[role]`, `scan/[role]`, `room/[id]`, `demo`. `src/lib`: `api.ts` (REST), `signaling.ts` (SFU websocket), `webrtc.ts`, `quality.ts`/`connection-stats.ts` (simulcast), `admin.ts`, `stage/` and `components/` (stage editor/map, video tiles). Built with `adapter-static` as an SPA (`ssr = false`, `index.html` fallback), so any static server can host it. `PUBLIC_API_BASE_URL` is baked in at build time; empty means same origin.

## Docker setup

```
browser ──https:8443──▶ gateway (nginx) ─┬─ /api/*      ▶ be  :8080   (Go)
                                          ├─ /ws         ▶ sfu :9000   (Rust, websocket)
                                          ├─ /internal/* ▶ 404
                                          └─ /*          ▶ fe  :80     (nginx + static SPA)
browser ──udp 50000-50199──────────────────────────────▶ sfu            (media, direct)
be ──http://sfu:9000/internal/*──▶ sfu                                  (internal network only)
```

- **Images are multi-stage and small.** `capstone_sfu`: `rust:1-slim-bookworm` build (cache mounts, stripped, thin LTO) → `distroless/cc-debian12:nonroot`. `capstone_be`: `golang:1.26.5-alpine` build (`CGO_ENABLED=0`, `-s -w`) → `distroless/static-debian12:nonroot`. `capstone_fe`: `node:24-alpine` build (`npm ci`, `vite build`) → `nginx:1.30.3-alpine` with only the static files. `gateway`: `nginx:1.30.3-alpine` plus one config template. Runtime images of the SFU and backend have no shell or package manager and run as non-root. Sizes: sfu ~50 MB, be ~36 MB, fe ~93 MB, gateway ~92 MB (the nginx base is most of the last two).
- **One public origin.** Only the gateway's HTTP/HTTPS ports and the SFU's UDP range are published on the host. `be`, `fe` and the SFU's TCP port exist only on the compose network. The gateway proxies `/ws` to the SFU but blocks `/internal/*` (404), which is how the SFU's REST API stays private even though it shares a port with `/ws`.
- **Client IP.** The gateway sets `X-Real-IP` from the connection (never from the client), via `gateway/snippets/proxy-headers.conf`, which every proxied location includes. This is deliberate: nginx drops *all* inherited `proxy_set_header` lines in a location that sets its own (the `/ws` one does), so setting them at server level silently left the SFU without the client IP and the blacklist blocked the gateway's address instead of the peer's. The backend only trusts it from `APP_SUBNET` (`TRUSTED_PROXIES`); the SFU takes the peer IP from the same header on websocket upgrade. The blacklist and admin reclaim depend on this.
- **Hostnames resolve per request** (Docker DNS, `resolver 127.0.0.11`), so restarting `be`, `fe` or `sfu` doesn't leave the gateway holding a stale address.
- **Media addresses.** Inside a container the SFU only sees its own bridge address, which browsers can't reach. With `SFU_PUBLIC_IPS=<APP_HOST_IP>` the SFU replaces the address in every *host* ICE candidate it sends clients (`rewrite_host_candidate` in `infra/media/session.rs`), and the UDP range is published 1:1 (same port inside and out), so a browser on the LAN sends media to the host and Docker forwards it. Other candidate kinds (srflx from STUN) pass through unchanged. This only works when `APP_HOST_IP` is an address clients can reach. Don't rely on `SettingEngine::with_nat_1to1_ips`: webrtc-rs 0.21 stores it but never applies it.
- **Firewall.** Docker publishes ports itself; if the host runs `ufw` for LAN access, the gateway port (`APP_HTTPS_PORT`) and the UDP range must be open to the LAN.
- **Verified:** a headless Chromium publisher and subscriber exchanged real video through the gateway (`wss`, ICE host candidate = `APP_HOST_IP`, frames decoded); `/internal/*` returns 404 and `be`/`sfu` TCP ports are closed on the host.
- **Blocklist at the gateway.** `be` writes blocked IPs to a volume shared with the gateway; `gateway/docker-entrypoint.d/40-blocklist-watch.sh` reloads nginx when the file changes. A blocked IP gets 403 `blacklisted` on every path, including `/ws`.
- **Rate limits at the gateway.** 20 r/s per client address for `/api` and `/ws`; 10 r/min for `POST /api/rooms` and admin reclaim. Over the limit is 429.
- **SSE and websockets** get their own locations (`proxy_buffering off`, 1 h read timeout), otherwise nginx's 60 s default cuts idle `/ws` and `/events` streams.

## Key decisions (summary; full list in `contracts.md`)

- **Room creation:** `POST /api/rooms` needs the server passcode (from `.env`). Only one room may be open per server (409 otherwise). Room id format: 3+3 characters from `A-Z` and `2-9`, no look-alikes (so no `1`, `I`, `O`, `0`).
- **Admin auth:** the admin token is a JWT with claims `admin: true` and `room_id`, sent as `X-Admin-Token`. The server verifies the signature and that `room_id` matches the open room. Tokens are not stored. Reclaim works only from the creator's IP, read from `X-Real-IP` set by nginx, never from client-supplied headers.
- **Streamer identity:** `peer_id` is assigned by the SFU per websocket connection and changes on every reconnect. Streamers are keyed by a client-generated `streamer_id` (UUID in `localStorage`). A streamer is "live" while its `peer_id` is in the SFU's publisher list.
- **Stage:** Go stores the template (`concert`, `conference`, `theater`, `blank`, or none) and placements and pushes changes over SSE (`GET /api/rooms/:room_id/events`: `stage_updated`, `room_closed`). The SFU has no stage logic. Positions are fractions 0.0–1.0.
- **Liveness (decision 11):** while at least one events stream is open, Go polls the SFU every 2 s and pushes `stage_updated` when a registered streamer goes live or offline. A new stream gets the contract's initial `stage_updated`, then one more from the poller's first baseline poll.
- **Simulcast:** the publisher sends `h`/`m`/`l` layers. The subscriber picks a layer per publisher (`set_preferred_layer`). The SFU steps it down from `connection_stats` and never steps back up on its own. Layer switches don't renegotiate. Don't report `available_incoming_bitrate_bps` from browsers.
- **Negotiation:** publisher ↔ SFU uses perfect negotiation (publisher impolite, SFU polite). Subscriber ↔ SFU is strictly SFU-offers, subscriber-answers.
- **Blacklist:** the admin blocks by a `peer_id` they can see. Go asks the SFU for its IP (`GET /internal/rooms/:room_id/peers/:peer_id`), blocks that IP and has the SFU disconnect the peer. Enforcement is in Go: `GET /api/rooms/:room_id` returns 403 `blacklisted`. Blocks last until the room closes or the admin unblocks.

## Known gaps (accepted for now)

These were reviewed and accepted; don't treat them as bugs to fix unless asked.

- **Viewers can't be blacklisted:** the admin view lists publishers only, so a subscriber's `peer_id` is never shown to the admin.
- **Direct SFU joins bypass the blacklist only without the gateway:** in the Docker stack the gateway refuses blocked IPs on `/ws` too. In local `make dev` a client that opens the SFU websocket directly still skips Go's check.
- **TURN credentials are shared:** one username and password for every client, handed out in `ice_servers`.
- **Admin tokens never expire:** they stay valid until their room closes. There is no `exp` claim.
- **No rate limiting outside the gateway;** local `make dev` has none.

## Known issues (not decisions)

- `capstone_fe/API-Contract.md` is an old copy and far behind `contracts.md`. `capstone_docs/API-Contract.md` and `capstone_sfu/API-Contract.md` were synced on 2026-11-17.
- "Room is full" is reported as `invalid_state`; it has no error code of its own.
- Test coverage is thinnest in `internal/repository` and `internal/middleware`.

## Working conventions

- Put any new contract change in `contracts.md` first, then update the code.
- Keep the "Settled decisions" and "Still open" sections of `contracts.md` in sync with any change.
- Don't commit or push without asking.

### Go test style

- Every `_test.go` file uses the table-driven format gopls/gotests scaffolds:
  `tests := []struct { name string; ... }{...}` followed by
  `for _, tt := range tests { t.Run(tt.name, func(t *testing.T) { ... }) }`.
- Only add a `setup` field (a function) to the test case struct when
  different cases need different setup/priming. Only add a `checkResult`
  field (a function that runs the assertions) when different cases need
  different result checking. Don't add either field if every case in the
  table shares the same setup/check — call it inline in the loop instead.
- Never factor test scaffolding (building a `*gin.Context`/request, priming
  state, assertions) into a shared helper function, even across test cases or
  files. Inline it directly at each call site, repeated as many times as
  needed.
- Middleware tests must not chain another middleware in front of the one
  under test to populate context (e.g. don't run `IpMiddleware()` before
  `BlackListMiddleware()`). Build the context with `gin.CreateTestContext`
  and set whatever the middleware reads directly. If that means reaching an
  unexported context key (like `ctxIpKey` in `ipsniffer.go`), put the test
  file in the internal `package middleware` (white-box) instead of
  `middleware_test`. Call the middleware directly (`mw(c)`) and assert on
  `c.IsAborted()`/`w.Code`, not a dummy next-handler in an engine chain.
