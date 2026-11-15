# Weekly Report — Week [9]

> Copy this file per week (or append a new section) and fill it in before your consultation. Weekly reports and daily consultations run in parallel across all 7 stages of the timeline (Ideation → Scope → User Stories → Wireframes → Documentation → Implementation → Code Defense), so use this from Week 1 — you don't need code yet to have progress worth reporting.

| Field        | Value                                                                                        |
| ------------ | -------------------------------------------------------------------------------------------- |
| Project name | _Local live video sharing_                                                                    |
| Trainee      | _M Haikal A_                                                                                 |
| Week / dates | 9 / (2026-08-31 – 2026-09-15)                                                                  |
| Phase        | Scope / User Stories                                                                          |
| Checkpoint   | _API Contract & WS Signal, SFU BoilerPlate WIP_                            |

## 1. This Week's Goal

Have a working signaling WebSocket server (handles room creation, sharer/watcher connections, and the basic messaging needed to set up peer connections).

## 2. Progress & Blockers

One row per deliverable. Required before consultation (Section 6 of the policy) — whenever status isn't Done, don't just name the problem in the blocker cell, name what you tried, and flag whether you need help on it.

| Deliverable            | Planned this week          | Actual status     | Blocker / What I tried                                                                                  | Evidence                    |
| ---------------------- | --------------------------- | ----------------- | -------------------------------------------------------------------------------------------------------- | --------------------------- |
| Api Contract | Draft the signaling + REST API contract covering browser↔Go, Go↔SFU, and browser↔SFU | Done | N/A — worked through it with Claude across several review passes (room existence semantics, who assigns peer_id, auth deferral, renegotiation reuse, polite/impolite negotiation roles for glare handling) | `api-contract-draft.md` |
| SFU Boilerplate | Rust workspace skeleton (webserver + sfu modules + mock publisher/subscriber) | WIP, partially superseded | Built an initial single-binary skeleton (axum webserver + sfu module wired by `mpsc`). Mid-week the architecture changed to a Go control-plane + Rust SFU media-plane split (multi-publisher/multi-subscriber discovery needed a real backend, not just an in-process channel), so that skeleton no longer matches the current contract. Also could not run `cargo check` in my current environment (no Rust toolchain) to verify the old skeleton compiled. Next step is re-scaffolding the SFU as its own websocket + internal REST server per the finalized contract. Not blocked on help, just re-sequencing. | `video-share-skeleton.zip` (superseded), `api-contract-draft.md` (current direction) |
### Other blockers (not tied to one deliverable)

None currently.

## 3. Decisions & Trade-offs

- **Split control plane from media plane**: signaling/discovery moves to a Go webserver, actual media SFU stays in Rust. Trade-off: adds a network hop and a second language to maintain, but matches how real SFU deployments (LiveKit, etc.) are structured and leaves room to run multiple SFU instances later without changing the public API.
- **Go↔SFU communication is synchronous REST for now** (Go calls SFU's internal API on demand) rather than the SFU pushing events to Go. Trade-off: simpler to build and always accurate, at the cost of one extra request per discovery call and a hard dependency on the SFU being reachable when Go needs it. Revisit if this becomes a bottleneck.
- **Multi-publisher, multi-subscriber, one role per peer**: a subscriber may hold multiple concurrent peer connections (one per publisher it watches); a publisher has exactly one, direct to the SFU. This means every subscriber-side signaling message carries a `target_peer`, while publisher-side messages don't need one.
- **Negotiation roles fixed by peer type**: publisher is always the offerer ("impolite" in perfect-negotiation terms) on its connection to the SFU, including for renegotiation (e.g. adding a track later reuses the same offer/answer messages — no new message type needed). Subscriber connections are strictly SFU-offers/subscriber-answers, so there's no glare case to handle there at all.
- **Rooms are always valid**: an empty room (no publishers yet) returns 200 with an empty list rather than 404. Keeps room lookup simple — no separate "room provisioning" step.
- **Auth deferred**: no auth for now; noted a blacklist as a likely future extension point (exact enforcement location — Go vs. SFU — intentionally left open until it's actually needed).

## 4. AI Usage This Week

- Used Claude to review the product overview doc, spot that Room Management had user stories and functional requirements but was missing from the Scope table, and draft the missing scope rows and admin-specific edge cases.
- Used Claude to help think through open design questions (admin identity, room capacity, blocklist persistence, reconnection handling) and write the resulting decisions into the doc.
- Used Claude to scaffold an initial Rust workspace skeleton for the webserver/SFU (later superseded), then to work through the signaling protocol for multi-publisher/multi-subscriber rooms, evaluate the Go-backend-plus-Rust-SFU split for feasibility, and draft/iterate the resulting API contract (`api-contract-draft.md`) across REST (browser↔Go, Go↔SFU) and WebSocket (browser↔SFU) boundaries.

## 5. Plan for Next Week

- Have a working basic WebApp UI.

## 6. Consultation Notes (fill in after the session)

- Feedback received:
- Actions I'm taking based on it: