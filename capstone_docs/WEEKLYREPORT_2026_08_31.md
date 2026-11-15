# Weekly Report — Week [2]

> Copy this file per week (or append a new section) and fill it in before your consultation. Weekly reports and daily consultations run in parallel across all 7 stages of the timeline (Ideation → Scope → User Stories → Wireframes → Documentation → Implementation → Code Defense), so use this from Week 1 — you don't need code yet to have progress worth reporting.

| Field        | Value                                                                                        |
| ------------ | -------------------------------------------------------------------------------------------- |
| Project name | _Local live video sharing_                                                                    |
| Trainee      | _M Haikal A_                                                                                 |
| Week / dates | 7 / (2026-08-25 – 2026-08-31)                                                                  |
| Phase        | Scope / User Stories                                                                          |
| Checkpoint   | _Room management scope locked, user stories & edge cases expanded_                            |

## 1. This Week's Goal

Filling in the gaps left from last week scoping — specifically the Room Management (admin) feature, which had user stories and functional requirements drafted but wasn't reflected in the Scope table, and had thin edge case coverage.

## 2. Progress & Blockers

One row per deliverable. Required before consultation (Section 6 of the policy) — whenever status isn't Done, don't just name the problem in the blocker cell, name what you tried, and flag whether you need help on it.

| Deliverable            | Planned this week          | Actual status     | Blocker / What I tried                                                                                  | Evidence                    |
| ---------------------- | --------------------------- | ----------------- | -------------------------------------------------------------------------------------------------------- | --------------------------- |
| Scope table (Room Management) | Add Room Creation/QR, Layout Management, and Sharer Moderation as scoped features | Done | —                                                                                                         | link to product-overview.md |
| US-3 acceptance criteria | Add rearrange stream ordering + kick/IP-block sharer criteria | Done | —                                                                                                         | link to product-overview.md |
| Admin role definition | Define who becomes admin and how | Done | Decided: whoever creates the room is auto-assigned admin, no login needed, since app is fully local-network | link to product-overview.md |
| Room model | Decide single vs. multi-room per server | Done | Decided: one room per network/server instance for MVP | link to product-overview.md |
| Admin reconnection flow | Decide how a disconnected admin reclaims control | Done | Decided: same-IP auto-reclaim first, secret key (generated at room creation) as fallback | link to product-overview.md |
| Room capacity | Define MVP capacity limits | Done | Decided: 2 sharers/publishers + 5 watchers for MVP | link to product-overview.md |
| Admin edge cases (4.3) | Replace placeholder edge cases (copy-pasted from Watcher section) with admin-specific ones | Done | Added 9 cases: mid-rearrange disconnects, kicking an already-gone sharer, NAT-shared IP blocks, self-block prevention, empty room state, session/IP reconnect, concurrent admin edits, capacity overflow, server/data lifetime | link to product-overview.md |
| Signaling server (WebSocket) | — | Not started | Next deliverable — see Section 5 | — |

### Other blockers (not tied to one deliverable)

None currently.

## 3. Decisions & Trade-offs

- **Admin = room creator, no login.** Simplest for a local, account-free flow; trade-off is no strong identity, so admin recovery relies on network-level signals (IP) rather than credentials.
- **One room per server instance (MVP).** Keeps server logic simple; multi-room support deferred.
- **Admin reclaim: IP first, secret key fallback.** Same-IP reconnect covers the common case (admin's device just dropped Wi-Fi briefly); the secret key covers the case where the admin's IP changed, at the cost of needing physical/session access to retrieve the key.
- **MVP capacity fixed at 2 sharers / 5 watchers.** No load-testing data yet — this is a starting assumption to unblock design, not a measured limit.
- **Blocklist and room state are ephemeral**, tied to server process lifetime (an event or up to a day) rather than persisted storage — matches the fully-local, no-account nature of the app but means blocks don't carry over between events.

## 4. AI Usage This Week

- Used Claude to review the product overview doc, spot that Room Management had user stories and functional requirements but was missing from the Scope table, and draft the missing scope rows and admin-specific edge cases.
- Used Claude to help think through open design questions (admin identity, room capacity, blocklist persistence, reconnection handling) and write the resulting decisions into the doc.

## 5. Plan for Next Week

- Have a working signaling WebSocket server (handles room creation, sharer/watcher connections, and the basic messaging needed to set up peer connections).

## 6. Consultation Notes (fill in after the session)

- Feedback received:
- Actions I'm taking based on it: