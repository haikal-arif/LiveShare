# API Contract Draft — Video Share App

Three separate boundaries, each with its own contract:

1. **Browser/mock-client → Go webserver** — REST, public
2. **Go webserver → Rust SFU** — REST, internal (not exposed past the reverse proxy)
3. **Browser/mock-client → Rust SFU** — WebSocket, public (signaling)

All of this eventually sits behind a reverse proxy; paths below assume that's handled later (e.g. `/api/*` routed to Go, `/sfu-ws` routed/upgraded to the SFU) and don't hardcode ports/hosts.

---

## 1. Browser → Go webserver (public REST)

### Conventions

- **Rooms must be created.** `POST /api/rooms` creates one. Every `/api/rooms/:room_id...` endpoint returns **404** for a room id that was never created (or was closed). A created room with nobody in it is still a valid room: 200 with an empty `publishers` list.
- **One room per server** (product NFR 4). While a room is open, `POST /api/rooms` returns 409.
- **Room state lives in Go's memory** (product NFR 5): rooms, streamers and stage layouts are gone when the Go process restarts.
- **Errors** always use this body, with a machine-readable `code`:
  ```json
  {
    "error": {
      "code": "room_not_found",
      "message": "room ABC-234 does not exist"
    }
  }
  ```
- **SFU down:** any endpoint that needs the SFU answers 502 `sfu_unreachable` if it can't reach it.
- **Admin endpoints** need the room's admin token in an `X-Admin-Token` header. Missing header → 401 `admin_token_required`, invalid token, or a token issued for a different room → 403 `not_admin`.
- **CORS:** allowed origins may call `GET, POST, PUT, DELETE` and send the `Content-Type` and `X-Admin-Token` headers.

### `POST /api/rooms`

Creates the server's room. Creating a room is gated by the server's passcode (set via `.env`), so only people who know it can open one. Whoever creates the room is its admin: the returned `admin_token` is the proof. The frontend keeps it in `localStorage`.

**Request:**

```json
{
  "passcode": "the-passcode-from-env"
}
```

**Response 201**

```json
{
  "room_id": "ABC-234",
  "admin_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZX0.c2lnbmF0dXJl"
}
```

`room_id` is 3 + 3 characters from `A-Z` and `2-9` (no look-alikes such as `O`/`0`, or `1`/`I`), the format the QR codes and manual entry already use. `admin_token` is a JWT signed by the server with two claims: `admin: true`, and `room_id` set to the room it was issued for. On each admin request the server checks the signature and that `room_id` matches the open room, so a token from a closed room is rejected. The server does not store tokens.

**Response 403** `invalid_passcode` — the passcode is missing or wrong. Checked before anything else, so a wrong passcode never reveals whether a room is open.

**Response 409** — a room is already open on this server. `room_id` tells the frontend where to send the user instead (join as streamer/watcher):

```json
{
  "error": {
    "code": "room_exists",
    "message": "a room is already open on this server"
  },
  "room_id": "ABC-234"
}
```

### `GET /api/rooms/:room_id`

Fetch everything a client needs to join a room: current publishers, and where/how to reach the SFU.

**Response 200**

`ice_servers` entries are `RTCIceServer`-shaped: `urls` is required; `username` and `credential` are present only on TURN entries (long-term credentials, same for every client). STUN entries have neither.

```json
{
  "room_id": "ABC-234",
  "publishers": [{ "peer_id": "pub-abc123" }, { "peer_id": "pub-def456" }],
  "sfu_ws_url": "wss://sfu.example.com/ws",
  "ice_servers": [{ "urls": ["stun:stun.example.com:3478"] }]
}
```

Created but empty (e.g. every streamer and the admin have left):

```json
{
  "room_id": "ABC-234",
  "publishers": [],
  "sfu_ws_url": "wss://sfu.example.com/ws",
  "ice_servers": [{ "urls": ["stun:stun.example.com:3478"] }]
}
```

**Response 403** `blacklisted` — the caller's IP is blocked from this room (see `POST /api/blacklist`). Nothing about the room is returned.

**Response 404** `room_not_found` — the room was never created, or has been closed.

### `GET /api/rooms/:room_id/publishers`

Just the publisher list, for polling/refresh without re-fetching ICE config every time.

**Response 200**

```json
{
  "room_id": "ABC-234",
  "publishers": [{ "peer_id": "pub-abc123" }]
}
```

**Response 404** `room_not_found`.

### `DELETE /api/rooms/:room_id` — admin

Closes the room. Go first asks the SFU to close it (`POST /internal/rooms/:room_id/close`, section 2), which sends `room_closed` to everyone connected there and disconnects them. Then Go drops the room's stage and streamers, sends `room_closed` on every open `events` stream, and the server can host a new room. Every `/api/rooms/:room_id...` call returns 404 from then on.

**Response 204**, or 401/403 (admin), or 404 `room_not_found`. **502** `sfu_unreachable` if the SFU couldn't be told; the room then stays open, so the admin can retry.

### `POST /api/rooms/:room_id/admin/reclaim`

For an admin who lost their token (cleared storage, new tab). If the request comes from the same IP address that created the room, a new admin token is handed back (product US-3: "reconnects from the same IP → admin privileges are reclaimed"). Behind nginx the client IP is read from the `X-Real-IP` header the proxy sets, never from anything the client can send itself.

**Request:** no body.

**Response 200**

```json
{ "room_id": "ABC-234", "admin_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhZG1pbiI6dHJ1ZX0.c2lnbmF0dXJl" }
```

**Response 403** `ip_mismatch` — a different IP. The admin can still paste the token manually (the "secret key" fallback in US-3). **404** `room_not_found`.

### Streamers

A **streamer** is a publisher with a name and a stable identity. `peer_id` changes every time a publisher (re)connects to the SFU, so the stage can't be keyed by it: a streamer who refreshes the page would lose their spot. Instead the publisher's browser generates a `streamer_id` once (a UUID kept in `localStorage`) and registers it with Go after every SFU `joined`, together with its current `peer_id`.

A streamer is **live** while its `peer_id` is in the SFU's publisher list (Go checks with `GET /internal/rooms/:room_id/publishers`). A streamer who leaves stays registered, just not live, so their stage spot survives until they come back or the room closes.

### `PUT /api/rooms/:room_id/streamers/:streamer_id`

Registers a streamer, or updates it after a reconnect (new `peer_id`) or a rename. Idempotent; no admin token needed. Called by the publisher right after the SFU's `joined` message.

**Request**

```json
{ "peer_id": "pub-abc123", "display_name": "Hana A" }
```

`display_name` is 1–40 characters after trimming whitespace. `peer_id` must be a live publisher in this room.

**Response 200** — the streamer:

```json
{
  "streamer_id": "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
  "peer_id": "pub-abc123",
  "display_name": "Hana A",
  "live": true
}
```

**Response 422** `invalid_display_name`, or `peer_not_live` (the `peer_id` isn't a publisher in this room right now). **404** `room_not_found`.

Every successful call pushes a `stage_updated` event (names and liveness are part of the stage payload).

### Stage layout

The room's presentation for watchers: a built-in venue **template** as the background, and **placements** putting streamers at spots on it.

- `template` is one of `"concert"`, `"conference"`, `"theater"`, `"blank"`, or `null` for "no stage set". With `null`, watchers get the plain grid (the behaviour before stages existed). Templates are drawn by the frontend; the backend only stores the id.
- A placement's `x`/`y` are fractions of the template's width/height, `0.0`–`1.0`, with `(0, 0)` the top-left corner (the back of the stage) and `(1, 1)` the bottom-right. That keeps positions right on any screen size.
- Registered streamers without a placement are "not on stage yet"; watchers see them in the "More streams" row.
- A placement for a streamer who isn't live is kept; watchers see its pin greyed out.
- `version` goes up by 1 on every change to the layout or the streamers. Concurrent admin saves are last-write-wins (product 4.3 edge case); `version` lets clients ignore out-of-order events.

### `GET /api/rooms/:room_id/stage`

The whole stage in one call: template, placements, and every registered streamer with its liveness.

**Response 200**

```json
{
  "room_id": "ABC-234",
  "version": 7,
  "template": "concert",
  "placements": [
    {
      "streamer_id": "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
      "x": 0.3,
      "y": 0.23
    },
    {
      "streamer_id": "c2f4a9e1-0b7d-4e3a-8c61-2d9f5b1e7a40",
      "x": 0.75,
      "y": 0.45
    }
  ],
  "streamers": [
    {
      "streamer_id": "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
      "peer_id": "pub-abc123",
      "display_name": "Hana A",
      "live": true
    },
    {
      "streamer_id": "c2f4a9e1-0b7d-4e3a-8c61-2d9f5b1e7a40",
      "peer_id": "pub-def456",
      "display_name": "User C",
      "live": false
    },
    {
      "streamer_id": "9d1a7b3c-5e2f-4a8b-b6c0-4f7e1d2a9c55",
      "peer_id": "pub-ghi789",
      "display_name": "User D",
      "live": true
    }
  ]
}
```

No stage set yet: `"version": 0, "template": null, "placements": []` (and whatever streamers have registered).

**Response 404** `room_not_found`.

### `PUT /api/rooms/:room_id/stage` — admin

Replaces the template and all placements at once: what the admin sees on "Save Layout" is exactly what's stored.

**Request**

```json
{
  "template": "concert",
  "placements": [
    {
      "streamer_id": "5b0e0c1e-6a7c-4f1a-9f55-0f6d3c7a2b11",
      "x": 0.3,
      "y": 0.23
    },
    {
      "streamer_id": "c2f4a9e1-0b7d-4e3a-8c61-2d9f5b1e7a40",
      "x": 0.75,
      "y": 0.45
    }
  ]
}
```

Send `"template": null, "placements": []` to remove the stage and go back to the grid.

**Response 200** — the new stage, same body as `GET /api/rooms/:room_id/stage`. Pushes `stage_updated` to every watcher.

**Response 422** with one of:

- `invalid_template` — not one of the four templates or `null`.
- `invalid_position` — an `x`/`y` outside `0.0`–`1.0`.
- `unknown_streamer` — a `streamer_id` that isn't registered in this room.
- `duplicate_streamer` — the same streamer placed twice.
- `placements_without_template` — placements sent with `"template": null`.

Plus 401/403 (admin) and 404 `room_not_found`.

### `GET /api/rooms/:room_id/events`

Live updates as [Server-Sent Events](https://html.spec.whatwg.org/multipage/server-sent-events.html) (`Content-Type: text/event-stream`), opened with the browser's `EventSource`. Everyone in the room keeps one open: watchers, streamers and the admin.

On connect the server immediately sends one `stage_updated` with the current stage, so a client needs no separate `GET .../stage` first. After that:

```
event: stage_updated
data: { ...same body as GET /api/rooms/:room_id/stage... }

event: room_closed
data: {"room_id":"ABC-234"}
```

- `stage_updated` — sent after every stage `PUT`, every streamer `PUT`, and whenever Go notices a streamer going live or offline. Clients replace their whole stage with the payload, skipping any whose `version` is lower than the one they have.
- `room_closed` — the admin closed the room. The server ends the stream after sending it. Clients should leave the room.
- A `: keepalive` comment line every 25 s keeps proxies from closing an idle stream. Responses carry `X-Accel-Buffering: no` so nginx doesn't buffer events.
- `EventSource` reconnects on its own after a dropped connection. The first event after reconnecting is always a fresh `stage_updated`, so nothing is missed.

**Response 404** `room_not_found` (as a normal JSON error, before any event is sent).

### `POST /api/blacklist` — admin

Blocks a peer's IP address from joining the open room, and disconnects that peer. The admin names the peer by its current `peer_id` (the one shown in the room's publisher list). Go asks the SFU which IP that peer connected from (`GET /internal/rooms/:room_id/peers/:peer_id`, section 2), blocks that IP, then asks the SFU to disconnect the peer (`POST /internal/rooms/:room_id/peers/:peer_id/disconnect`, section 2). Only the room's admin can do this. The list lives in Go's memory and is cleared when the room closes.

**Headers:** `X-Admin-Token` (required).

**Request**

```json
{ "peer_id": "pub-abc123" }
```

Blocking disconnects the named peer at once: the SFU closes its websocket, so a publisher drops out of the publisher list and its streamer goes not live (its registration and stage spot stay, as with any leave). Other peers already connected from the same IP are not disconnected, but nobody from that IP can join afterwards (`GET /api/rooms/:room_id` returns 403 `blacklisted`). Because the block is by IP, everyone behind the same IP (for example a shared network) is blocked from joining too. Blocking an IP that's already blocked returns the existing entry.

**Response 200** — the blocked entry:

```json
{
  "room_id": "ABC-234",
  "ip": "203.0.113.7",
  "peer_id": "pub-abc123"
}
```

**Response 404** `peer_not_found` — that `peer_id` isn't connected to this room right now. **502** `sfu_unreachable` if the SFU couldn't resolve or disconnect the peer; if the IP was already stored by then, the block stays and the admin can retry. Plus 401 `admin_token_required` / 403 `not_admin` (admin). With no room open, no admin token is valid, so the answer is 403 `not_admin`.

### `GET /api/blacklist` — admin

The current block list. The admin needs the `ip` from here to unblock.

**Response 200**

```json
{
  "room_id": "ABC-234",
  "blacklist": [{ "ip": "203.0.113.7", "peer_id": "pub-abc123" }]
}
```

Empty list: `"blacklist": []`. Plus 401/403 (admin); with no room open the answer is 403 `not_admin`.

### `DELETE /api/blacklist/:ip` — admin

Unblocks an IP, so it can join the room again. `:ip` is URL-encoded (IPv6 addresses contain colons, e.g. `DELETE /api/blacklist/2001%3Adb8%3A%3A1`).

**Response 204**, or **404** `not_blacklisted` if that IP isn't on the list. Plus 401/403 (admin); with no room open the answer is 403 `not_admin`.

### `GET /api/health`

Standard liveness check for the Go service itself.

---

## 2. Go webserver → Rust SFU (internal REST)

Mirrors what Go needs to answer the public endpoints above, one level down. Only reachable from Go, not the public internet.

**Errors** use the same body as the public API, with a machine-readable `code`:

```json
{ "error": { "code": "peer_not_found", "message": "peer pub-abc123 is not connected to room ABC-234" } }
```

### `GET /internal/rooms/:room_id/publishers`

**Response 200**

```json
{
  "room_id": "ABC-234",
  "publishers": [{ "peer_id": "pub-abc123" }, { "peer_id": "pub-def456" }]
}
```

Same shape reused by `common::internal::RoomStats`-style data already in the Rust code — this endpoint is basically that struct serialized.

### `GET /internal/rooms/:room_id/stats`

**Response 200**

```json
{
  "room_id": "ABC-234",
  "publisher_count": 2,
  "subscriber_count": 5,
  "subscriber_quality": { "good": 3, "poor": 1, "bad": 1 }
}
```

`subscriber_quality` counts subscriber↔publisher pairings by their current `connection_quality` level (see section 3), so Go/admin can see how many subscribers are on low bandwidth. A subscriber watching two publishers counts once per pairing, so these counts can sum to more than `subscriber_count`.

### `GET /internal/rooms/:room_id/peers/:peer_id`

Returns the IP address a connected peer (publisher or subscriber) opened its websocket from. Go uses it to resolve a `peer_id` for the blacklist. The SFU reads the IP from the websocket upgrade request, using the `X-Real-IP` header the reverse proxy sets (the same rule as the reclaim endpoint), and keeps it only while the peer is connected.

**Response 200**

```json
{
  "room_id": "ABC-234",
  "peer_id": "pub-abc123",
  "ip": "203.0.113.7"
}
```

**Response 404** `peer_not_found` — no connected peer with that id in this room.

### `POST /internal/rooms/:room_id/peers/:peer_id/disconnect`

Called by Go when the admin blocks a peer. The SFU closes that peer's peer connections and its websocket and removes it from the room, exactly as if it had left: a publisher triggers `publisher_left` for the room's subscribers. Nothing is sent to the peer before the close.

**Request:** no body. **Response 204**, or **404** `peer_not_found` if no such peer is connected (Go treats that as "already gone", not an error).

### `POST /internal/rooms/:room_id/close`

Called by Go when the admin closes a room. The SFU sends `room_closed` (section 3) to every publisher and subscriber in the room, then closes all of the room's peer connections and websockets and drops the room.

**Request:** no body. **Response 204** — also when the SFU has no such room (nobody ever connected, or it's already closed), so Go can retry safely.

### `GET /internal/health`

Liveness/readiness for the SFU process, so Go (or the reverse proxy / orchestrator) can check it's up.

---

## 3. Browser/mock-client → Rust SFU (WebSocket signaling)

One websocket connection per client. A client decides its role (publisher or subscriber) _before_ opening this connection — that decision is expressed by which join message it sends first (`join_as_publisher` vs `join_as_subscriber`), and it can't switch roles on the same connection.

**Asymmetry:** a **publisher** has exactly one peer connection — with the SFU itself, no target needed. A **subscriber** may have _multiple_ concurrent peer connections — one per publisher it's watching — so every subscriber-side signaling message needs to say which publisher it's about.

`target_peer` is `null`/omitted for the publisher's own connection to the SFU, and set to a publisher's `peer_id` for anything on the subscriber side.

### Negotiation roles (perfect-negotiation pattern)

- **Publisher ↔ SFU:** the publisher is always **impolite**, the SFU is **polite** on this pairing. The publisher always initiates (`sdp_offer` with `target_peer: null`), including for later renegotiation (e.g. adding a screen-share track) — same message, same rule. If a glare ever occurred (both sides offering at once), the SFU as the polite side would roll back its own offer in favor of the publisher's; in practice the SFU is not expected to spontaneously offer to a publisher, so this mostly matters for correctness/future-proofing rather than a case that happens often.
- **Subscriber ↔ SFU (per publisher):** strictly one-directional — the SFU is always the offerer, the subscriber always the answerer. A subscriber never sends an unsolicited `sdp_offer`; it sends `request_track`, and the SFU responds with `sdp_offer` as soon as that publisher has at least one live track (if the publisher hasn't started sending media yet, the offer is deferred until it does — no error, no empty offer). Whenever the publisher's tracks change, the SFU re-offers on the same pairing (`sdp_offer` with the same `from_peer`), so a subscriber must answer every `sdp_offer`, not just the first. Because the subscriber never offers, there's no glare case to resolve on this pairing — polite/impolite doesn't apply here, only on the publisher/SFU pairing.

### Connection quality & simulcast

The SFU relays RTP and never transcodes, so it can't make a lower-quality version of a stream itself. The lower-quality versions come from the **publisher, through simulcast**. The **subscriber picks** a layer for each publisher it watches (`set_preferred_layer`, default `high`). The SFU forwards that layer and steps the pairing down on its own when the subscriber's `connection_stats` show it can't keep up. It never steps back up by itself: picking a higher layer again is the subscriber's retry.

- **Publisher:** SHOULD offer simulcast with 3 video encodings, rids `h` / `m` / `l` (`scaleResolutionDownBy` 1 / 2 / 4, suggested `maxBitrate` ≈ 2.5 Mbps / 800 kbps / 250 kbps). This goes through the existing `sdp_offer` message, so there's no new message type. Each layer's RTP must carry the `sdes:mid` and `sdes:rtp-stream-id` header extensions, which browsers add by default; that's how the SFU tells layers apart. Rids other than `h`/`m`/`l` are ignored. If a publisher sends only one encoding, the SFU treats it as `high` only: quality signals still flow, but the layer never changes.
- **Subscriber:** the SFU's `sdp_offer` to a subscriber still contains a single video stream. Layer switches happen inside the SFU: it forwards a different rid, sends a PLI to the publisher, and switches at the next keyframe. **A layer switch never triggers renegotiation.**
- **Tracking source:** mainly the subscriber's `connection_stats` reports (below). The SFU may also use the RTCP receiver reports it already sees, as a fallback when reports stop arriving.
- **Initial thresholds** (tunable, evaluated per subscriber↔publisher pairing), and what 2 consecutive reports at that level do:

  | Quality | Condition                                                      | Action                                   |
  | ------- | -------------------------------------------------------------- | ---------------------------------------- |
  | `good`  | loss < 2% and available bitrate ≥ the `h` layer's bitrate      | none                                     |
  | `poor`  | loss 2–10%, or available bitrate below the `h` layer's bitrate | step down one layer from the current one |
  | `bad`   | loss > 10%, or available bitrate below the `m` layer's bitrate | go straight to `low`                     |

  Of the 2 reports, the milder one counts, so one spike doesn't cause a `bad` jump. Each step needs 2 fresh reports. There is **no automatic step up**: after a downgrade the pairing stays put until the subscriber sends `set_preferred_layer` again, which applies the picked layer at once and clears the downgrade.

  **Don't report `available_incoming_bitrate_bps` from a browser.** Chrome's `availableIncomingBitrate` only tracks what it's currently receiving (the SFU doesn't probe for bandwidth), so on a healthy link it sits below the `h` layer's bitrate and the subscriber gets downgraded for nothing. The reference client reports loss only.

### Client → Server messages

```json
{ "type": "join_as_publisher", "room": "ABC-234" }
```

```json
{ "type": "join_as_subscriber", "room": "ABC-234" }
```

```json
{
  "type": "sdp_offer",
  "target_peer": null,
  "sdp": "v=0..."
}
```

(publisher offering its stream to the SFU; `target_peer: null` because there's only one counterparty)

```json
{
  "type": "sdp_answer",
  "target_peer": "pub-abc123",
  "sdp": "v=0..."
}
```

(subscriber answering the SFU's offer for a specific publisher's stream)

```json
{
  "type": "ice_candidate",
  "target_peer": "pub-abc123",
  "candidate": "candidate:..."
}
```

```json
{
  "type": "request_track",
  "target_peer": "pub-abc123"
}
```

(subscriber explicitly asking to start watching a given publisher — triggers the SFU to create an offer for that pair)

```json
{
  "type": "connection_stats",
  "target_peer": "pub-abc123",
  "available_incoming_bitrate_bps": 1200000,
  "packet_loss_pct": 3.5,
  "round_trip_time_ms": 45,
  "jitter_ms": 12
}
```

```json
{
  "type": "set_preferred_layer",
  "target_peer": "pub-abc123",
  "layer": "high"
}
```

(subscriber picking the layer it wants from one publisher: `high` | `mid` | `low`. Applied at once, even above a layer the SFU downgraded it to, so this is also the retry after a downgrade. The SFU always replies with `connection_quality`. Same errors as `connection_stats`.)

(`connection_stats` is subscriber only, sent about every 2 s for each watched publisher, using values from `RTCPeerConnection.getStats()`; the SFU uses it to pick that pairing's simulcast layer. Only `packet_loss_pct` is required. `available_incoming_bitrate_bps`, `round_trip_time_ms` and `jitter_ms` may be omitted, since not every browser exposes them, and a report without a bitrate is judged on loss alone. `round_trip_time_ms` and `jitter_ms` are accepted but the SFU doesn't use them yet. A publisher sending it gets `error`, and so does a subscriber whose `target_peer` isn't a publisher it has sent `request_track` for.)

```json
{ "type": "leave" }
```

### Server → Client messages

```json
{
  "type": "joined",
  "peer_id": "sub-xyz789",
  "ice_servers": [{ "urls": ["stun:stun.example.com:3478"] }],
  "publishers": ["pub-abc123", "pub-def456"]
}
```

(sent right after join; `publishers` is the room snapshot at join time so a subscriber isn't blind until the first `publisher_joined` event)

```json
{ "type": "publisher_joined", "peer_id": "pub-ghi789" }
```

```json
{ "type": "publisher_left", "peer_id": "pub-abc123" }
```

```json
{
  "type": "sdp_offer",
  "from_peer": "pub-abc123",
  "sdp": "v=0..."
}
```

(SFU offering a specific publisher's stream to a subscriber)

```json
{
  "type": "sdp_answer",
  "from_peer": null,
  "sdp": "v=0..."
}
```

(SFU answering the publisher's own offer; `from_peer: null` = "this is the SFU itself", i.e. the publisher's direct counterparty)

```json
{
  "type": "ice_candidate",
  "from_peer": "pub-abc123",
  "candidate": "candidate:..."
}
```

```json
{
  "type": "connection_quality",
  "from_peer": "pub-abc123",
  "quality": "poor",
  "layer": "mid"
}
```

(sent to a subscriber when that pairing's `quality` or `layer` changes, and in reply to every `set_preferred_layer`. `layer` is what's actually forwarded (`high` | `mid` | `low`), which can be lower than the pick if the publisher isn't sending that layer. `quality` is `good` while at the pick, otherwise how bad the loss was that caused the last step down (`poor` | `bad`). The FE shows `layer` as the selected quality and tags it "auto" when it differs from the user's pick.)

```json
{ "type": "room_closed" }
```

(the admin closed the room. Sent to every publisher and subscriber in it; the SFU then closes their peer connections and the websocket, so there's nothing to send back.)

```json
{ "type": "error", "code": "peer_not_found", "reason": "publisher not found" }
```

(`code` is machine-readable and `reason` is human-readable text for logs or display. A client should treat a `code` it doesn't know as a generic error. The codes:

| `code` | Meaning |
| --- | --- |
| `invalid_message` | The frame wasn't valid JSON for any client message, or a field was wrong (e.g. a non-null `target_peer` on the publisher's own connection, or a malformed ICE candidate). |
| `not_joined` | Sent before `join_as_publisher` / `join_as_subscriber`. |
| `already_joined` | A second `join_as_*` on the same connection. |
| `peer_not_found` | `target_peer` isn't a publisher the sender can address (unknown, or not yet requested with `request_track`). |
| `wrong_role` | The message is for the other role, e.g. a publisher sending `request_track`. |
| `invalid_sdp` | The SDP in `sdp_offer` / `sdp_answer` was rejected. |
| `invalid_state` | The message doesn't fit the negotiation state, e.g. `ice_candidate` before `sdp_offer`, or an `sdp_answer` with no pending offer. |
| `internal` | The SFU failed to set up a peer connection or offer. |
)

---

## Settled decisions (from review)

1. **Room existence:** rooms are created with `POST /api/rooms` (one per server) and exist until the admin closes them or Go restarts. A never-created or closed room id is a 404 on every `/api/rooms/...` endpoint; a created room with nobody in it is a 200 with an empty `publishers` list.
2. **`peer_id` assignment:** the SFU assigns it, returned in the `joined` message.
3. **Auth:** creating a room needs the server passcode (`.env`). Admin actions (close the room, edit the stage, manage the blacklist) need the `admin_token` from `POST /api/rooms`, a JWT with the claims `admin: true` and `room_id`, sent as `X-Admin-Token`. It's reclaimable from the creator's IP. Joining, watching and registering as a streamer need no auth. Blacklist enforcement is in Go: `GET /api/rooms/:room_id` returns 403 `blacklisted` to a blocked IP.
4. **Renegotiation:** reuses `sdp_offer`/`sdp_answer` with `target_peer: null`, no new message type. Consistent with the publisher always being the offerer on that pairing (see negotiation roles above).
5. **`room` scoping:** implicit per-connection after the initial `join_as_*` — not repeated on subsequent messages.
6. **Negotiation politeness:** documented above — publisher is impolite, SFU is polite, on the publisher/SFU pairing; subscriber/SFU pairing is strictly offerer(SFU)/answerer(subscriber) with no glare case.
7. **Low-bandwidth subscribers:** the publisher simulcasts (`h`/`m`/`l`). The subscriber picks a layer per publisher with `set_preferred_layer`, and the SFU steps it down from `connection_stats` and reports the result with `connection_quality`. Upgrades only happen when the subscriber picks again. No transcoding, and no renegotiation on a layer switch (see "Connection quality & simulcast" above).
8. **Stage layout:** owned by Go, not the SFU. Go stores the template, placements and the streamer registry (`streamer_id` → name and current `peer_id`), and pushes changes over the SSE `events` stream. The SFU stays media-only and needs no changes for stages.
9. **Room existence is only enforced by Go.** The SFU doesn't know which rooms were created, so a client that talks to the SFU directly (skipping `GET /api/rooms/:room_id`) can still join a never-created room id there. Accepted: the 404 exists to steer the normal app flow, not as a security boundary.
10. **Closing a room** goes through the SFU (`POST /internal/rooms/:room_id/close`), which tells everyone with `room_closed` and disconnects them, so no media keeps flowing after the admin ends the room.
11. **Streamer liveness:** while at least one `events` stream is open for a room, Go polls `GET /internal/rooms/:room_id/publishers` every 2 s and sends `stage_updated` when a registered streamer goes live or offline. Watchers aren't held up by the delay: they get `publisher_joined`/`publisher_left` from the SFU directly.
12. **Blacklist identifier:** the admin blocks by `peer_id`, and Go resolves it to an IP through the SFU at the time of the call. `peer_id` is not reused across connections, so it's only valid while that peer is connected. Blocks are stored by IP and last until the room closes or the admin unblocks. Blocking also disconnects the named peer straight away (`POST /internal/rooms/:room_id/peers/:peer_id/disconnect`); other peers on the same IP are only stopped from joining later.

## Still open

- **Watchers in the admin view** — the admin can only blacklist `peer_id`s it can see, and `GET /api/rooms/:room_id/publishers` lists publishers only. A viewer (subscriber) has a `peer_id` but no endpoint shows it to the admin, so a viewer can't be blacklisted yet. Options: add subscriber `peer_id`s to an admin-only view, or accept publisher-only blocking for now.
- **Token expiry** — admin tokens don't expire. The `room_id` claim stops reuse across rooms, but a token from the current room stays valid until the room closes. Add an `exp` claim if that's too long.
- **Blacklist bypass via the SFU** — Go enforces the blacklist on `GET /api/rooms/:room_id`, but a client that opens the SFU websocket directly can still join (see decision 9). Closing this needs the SFU to check the blocked list, which means Go pushing it to the SFU.
- **Track/media shape** — this contract covers signaling only. Whether a publisher can have multiple tracks (camera + screen-share) and how a subscriber would request "all tracks" vs a specific one isn't modeled yet (`request_track` currently implies "give me this publisher's stream," singular). Fine to leave for later since it doesn't block getting a basic single-track pipeline working end to end. Video within a single track can now come in simulcast layers (`h`/`m`/`l`), but that's a quality choice inside one stream, not a separate track.
