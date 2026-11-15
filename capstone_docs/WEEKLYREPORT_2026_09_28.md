# Weekly Report — Week [10]

> Copy this file per week (or append a new section) and fill it in before your consultation. Weekly reports and daily consultations run in parallel across all 7 stages of the timeline (Ideation → Scope → User Stories → Wireframes → Documentation → Implementation → Code Defense), so use this from Week 1 — you don't need code yet to have progress worth reporting.

| Field        | Value                                                                                        |
| ------------ | -------------------------------------------------------------------------------------------- |
| Project name | _Local live video sharing_                                                                    |
| Trainee      | _M Haikal A_                                                                                 |
| Week / dates | 10 / (2026-09-15 – 2026-09-28)                                                                 |
| Phase        | Implementation                                                                                |
| Checkpoint   | _Working MVP: SFU with real WebRTC, Go discovery API, SvelteKit web app_                      |

## 1. This Week's Goal

Get a working end-to-end MVP: re-scaffold the SFU as its own service per the finalized API contract, stand up the Go control-plane webserver, build the first version of the web app, and get real media flowing from publisher → SFU → subscriber.

## 2. Progress & Blockers

| Deliverable            | Planned this week          | Actual status     | Blocker / What I tried                                                                                  | Evidence                    |
| ---------------------- | --------------------------- | ----------------- | -------------------------------------------------------------------------------------------------------- | --------------------------- |
| SFU re-scaffold + signaling | Rewrite the SFU as a standalone websocket + internal REST server per `API-Contract.md` | Done | Replaced the old single-binary webserver+SFU skeleton with one `server` crate: room/peer registry, signaling websocket (join, `publisher_joined`/`publisher_left` broadcasts, `request_track`, SDP/ICE relay), and the internal REST API for Go (room state, health). WebRTC was behind a stub (`webrtc_stub.rs`, fake SDP) so the signaling state machine could be finished and tested first. | `capstone-sfu` commit `aa32a23` "feat: mvp"; `crates/server/tests/signaling.rs` (7 integration tests) |
| Real WebRTC media plane (replacing the stub) | Swap the stub for real `webrtc-rs` negotiation and RTP forwarding | Done (not yet committed) | Replaced `webrtc_stub.rs` with `webrtc_engine.rs` on `webrtc-rs` 0.21: one receive-only `PeerConnection` per publisher, one send-only per (subscriber, publisher) pairing, RTP fanned out per track over a `broadcast` channel, and a relay task per subscriber track that re-stamps SSRC/payload type. Handles PLI/FIR keyframe requests so new subscribers get a picture quickly. Found and fixed a contract gap along the way: what happens when a subscriber requests a publisher that has no media yet (see Decisions). | `capstone-sfu/crates/server/src/webrtc_engine.rs`; `crates/server/tests/media.rs` (media flows, deferred offer, fan-out to multiple subscribers and cleanup on publisher leave, malformed SDP rejected) |
| Go control-plane webserver | Public discovery REST API backed by the SFU's internal API | Done (not yet committed) | Gin server with `GET /api/health`, `GET /api/rooms/:room_id`, `GET /api/rooms/:room_id/publishers`, CORS, and an SFU HTTP client. Returns the SFU websocket URL and ICE servers to clients. SFU address and ICE config come from env vars. | `capstone-be/internal/` (handler, sfuclient, router, config); `internal/handler/room_test.go` |
| Web app (FlashLive) | Basic SvelteKit UI for create / stream / watch | Done | SvelteKit 2 / Svelte 5 app: splash → home, Create Room (generates an `XXX-XXX` room code and a QR code), join as streamer (camera preview) or watcher, QR scan screen, and a room view with `VideoTile`s. Talks to Go for discovery and to the SFU for signaling/WebRTC. | `capstone-fe` commit `d7bb67e`; unit tests (`roomcode`, `qr`, `api`, `signaling`, `webrtc` specs) and Playwright flows (`flashlive-flows.e2e.ts`), not yet committed |
| Local dev tooling | One command to run the whole stack | Done | `start-dev.sh` starts SFU (`cargo run`), Go backend (`go run`), and frontend (`npm run dev`) together with prefixed logs, and kills each process group on Ctrl+C so no orphaned `cargo`/`go` children are left behind. | `capstone/start-dev.sh` |

### Other blockers (not tied to one deliverable)

- A lot of this week's work is still uncommitted: the whole Go backend is untracked, the SFU's real WebRTC engine and media tests are local changes, and the frontend test files are untracked. Committing and pushing these is the first thing to do next week.
- The `mock-publisher` / `mock-subscriber` CLI crates were removed in the SFU rewrite. Their role is now covered by the Rust integration tests (`tests/media.rs` drives real `webrtc-rs` client peers). `technical-architecture.md` still describes them and needs updating.

## 3. Decisions & Trade-offs

- **Build signaling on a stub, then swap in real WebRTC**: finished and tested the room/peer state machine and message routing against a fake SDP stub first, with the WebRTC calls kept in one file so swapping in `webrtc-rs` only touched that file and its call sites. Trade-off: two passes over the SFU, but signaling bugs and media bugs never had to be debugged at the same time.
- **Subscriber offers are deferred until the publisher has a live track**: if a subscriber sends `request_track` before the publisher is sending media, the SFU accepts the request and sends `sdp_offer` once a track arrives, instead of returning an error or an empty offer. Trade-off: the subscriber may wait with no feedback, but clients don't need retry logic. An unknown publisher still returns an error.
- **SFU re-offers when a publisher's tracks change**: the same `sdp_offer` (same `from_peer`) is reused for renegotiation on the subscriber pairing, so subscribers must answer every offer, not just the first. Keeps the message set small and matches the publisher-side renegotiation rule. `API-Contract.md` is updated to say this.
- **Relay RTP, don't transcode**: the SFU forwards publisher RTP packets as-is and only rewrites SSRC/payload type per subscriber. Trade-off: very low CPU cost, but every subscriber gets the publisher's single quality level (no simulcast/adaptive bitrate yet).
- **Replaced the mock CLIs with integration tests**: instead of keeping separate `mock-publisher`/`mock-subscriber` binaries in sync with the contract, the SFU tests spin up real WebRTC client peers in-process. Trade-off: no standalone tool for manual testing, but the pipeline is checked automatically on every `cargo test`.
- **Room codes are `XXX-XXX` without look-alike characters** (`0/O/1/I`), and QR codes carry a join URL, so typed and scanned codes resolve to the same room.

## 4. AI Usage This Week

- Used Claude to draft `technical-architecture.md` from the finalized API contract.
- Used Claude to help implement the SFU's real WebRTC engine on `webrtc-rs` and to work through the subscriber negotiation edge cases (no live track yet, track changes mid-session), then update `API-Contract.md` to match.
- Used Claude to help write this weekly report from the repos' commit history and working-tree changes.

## 5. Plan for Next Week

- Commit and push the uncommitted work across `capstone-sfu`, `capstone-be`, and `capstone-fe`.
- Test the MVP on real devices on the local network (multiple phones streaming and watching at once), including reconnection when a peer drops.
- Update `technical-architecture.md` to match what was built (mock CLIs removed, real WebRTC engine, Go endpoints).
- Start on room management / admin features from the product overview (e.g. blocklist), and polish the web app UI.

## 6. Consultation Notes (fill in after the session)

- Feedback received:
- Actions I'm taking based on it: