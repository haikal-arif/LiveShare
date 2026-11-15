# Plan: desktop display support

Status: **planning only, nothing built.** Starts from the wireframes in `capstone_fe/wireframes/` (flows 1–7). Open questions are at the end; the first one decides the rest.

## 1. Reading of the request

"Desktop display" is taken to mean **a streamer shares their desktop (a full screen or one window) as a stream of its own**, alongside their camera, and watchers can see and switch to it.

Two other readings are noted and handled as follow-ups:

- **Desktop _layout_** (the app laid out for a wide screen when a watcher opens it on a PC). Every wireframe is a phone frame, so this needs new designs before any code. See §5.
- **Screen share as it already works** (replace the camera with the screen on the same stream). This exists today in `toggleScreenShare()` (`capstone_fe/src/lib/room.svelte.ts`). The plan extends it rather than replacing it, since the camera-replacement behaviour is what the wireframes most closely match.

## 2. What exists today

| Area            | Current state                                                                                                                                    | Where                                                                        |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| Screen share    | `getDisplayMedia` result **replaces** the camera track on the publisher's single video sender. Watchers see one tile; there is no second stream. | `room.svelte.ts` `toggleScreenShare`                                         |
| Control         | A monitor icon button ("Share screen") in the publisher's bottom bar.                                                                            | `room/[id]/+page.svelte`                                                     |
| Track model     | One video track per publisher. `request_track` means "this publisher's stream", singular.                                                        | `contracts.md` §3, `API-Contract.md` open items, `technical-architecture.md` |
| Simulcast       | Publisher sends `h`/`m`/`l`; subscriber picks a layer. Layer switches never renegotiate.                                                         | `contracts.md` §3 "Connection quality & simulcast"                           |
| Stage           | Placements are per **streamer**. A streamer is one tile on the stage.                                                                            | `contracts.md` "Stage layout", `stage/stage.ts`                              |
| Browser support | `getDisplayMedia` works in desktop Chrome/Edge/Firefox/Safari. It is not available in mobile browsers.                                           | —                                                                            |

The existing contract already expects this change. `contracts.md` (open items) and `technical-architecture.md` both list "multi-track per publisher" as not modelled yet and say a second track would reuse `sdp_offer`/`sdp_answer` with no new message type.

## 3. Wireframe-driven changes

The wireframes do **not** show a screen-share control. The room bar in flows 1, 2 and 4 has camera, mic and leave, plus a greyed "Share link room". The shipped code adds a monitor button that no wireframe covers. So the first design task is to draw it.

| Flow / screen                                 | Change                                                                                                                                                                                                                                      | Decision needed                                                                           |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| **Flow 1, 2, 4 – Streaming room (publisher)** | Add a "Share desktop" control to the bottom bar. Show it only where `navigator.mediaDevices.getDisplayMedia` exists; hide it on phones instead of greying it, so it doesn't look broken.                                                    | Hide vs. grey-out on unsupported devices. Recommend **hide**.                             |
| **Flow 1, 2, 4 – Streaming room (publisher)** | While sharing, show a "Sharing desktop" state on the publisher's own tile and a Stop control. Stopping the browser's own share dialog ends it too (`ended` event).                                                                          | Copy and placement of the state.                                                          |
| **Flow 6 – Arrange streamers (admin)**        | A desktop share is a **second tile for the same streamer**, not a new streamer. It appears under "Not on stage yet" as "<name> · Desktop" when live. Admin can place it independently, so a slide screen can sit at the front of the stage. | Separate placement (recommended) vs. one placement that swaps between camera and desktop. |
| **Flow 6 – Streaming room (admin)**           | Admin's stage shows a monitor badge on a streamer's tile while their desktop is live.                                                                                                                                                       | —                                                                                         |
| **Flow 7 – Stage view (watcher)**             | The "More streams" row lists "<name> · Desktop" next to the camera entry. Red-dot "live" rule applies to each.                                                                                                                              | —                                                                                         |
| **Flow 7 – Full stream view (watcher)**       | Add a source switch (Camera / Desktop) next to the existing quality picker ("High ▾"). Quality choice stays when switching source.                                                                                                          | Switch as a segmented control or a menu.                                                  |
| **Flow 7 – Stream ended**                     | When a desktop share stops while watched, show "<name> stopped sharing" and fall back to the camera if it's still live. Reuses the existing "stopped streaming" message.                                                                    | Fall back to camera vs. show "Pick another spot". Recommend **fall back to camera**.      |
| **Flow 7 – Stage view, no stage set**         | Desktop tiles appear in the grid like any other stream.                                                                                                                                                                                     | —                                                                                         |

## 4. Protocol and backend

The minimum change is to let a publisher carry **two video tracks, camera and desktop**, and let subscribers ask for one by kind.

1. **Contract first** (repo convention). In `contracts.md` §3 add a `track` field to `request_track` (`"camera"` default, `"screen"`), and to `sdp_offer`/`sdp_answer` where they carry a publisher's track. Add it to "Settled decisions" once agreed. Recommend **one publisher peer connection with a second transceiver** over the alternatives below.
2. **Rejected alternative: a second publisher peer.** A "screen" pseudo-peer with its own `peer_id` is the smallest SFU change, but it breaks "one publisher = one streamer": the streamer liveness rule (Go checks `peer_id` in the publisher list), blacklist-by-peer (an admin would blacklist the wrong one) and the stage all assume one peer per streamer.
3. **SFU** (`capstone_sfu`): the room actor and `negotiation.rs` track one video per publisher. Extend that to a keyed set of tracks per publisher. Relay logic in `infra/media` must forward each kind to subscribers that asked for it. The layer switcher stays per track. Keep the `domain/` and `service/` layering that CI checks (no `webrtc::` under `domain/` or `service/`).
4. **Simulcast for screens:** send one layer (`h`) by default. Desktop content is detail-heavy, so set `contentHint = "detail"`. Confirm the quality table in `contracts.md` still makes sense for a static, high-resolution source before reusing it.
5. **Go** (`capstone_be`): `GET /api/rooms/:room_id/publishers` stays per peer. Add the track kinds per publisher to the stage payload so the frontend knows a desktop is live (`stage_updated`). Streamer `live` stays "camera peer is live", unless the product wants desktop-only streamers to count as live.
6. **Ports:** a second transceiver may need more ICE/UDP traffic on the same published range (`APP_UDP_PORTS`, 200 ports). Verify whether the SFU multiplexes ICE over one port per publisher before assuming the range is enough.

## 5. Desktop layout (follow-up, needs designs)

Every wireframe is a phone frame. A watcher on a PC currently gets the phone layout stretched to the window. Before building this, draw:

- Stage view on desktop (stage plus the stream grid, or stage full width with a side rail for "More streams").
- Full stream view on desktop (video at full window size, with the mini stage map and the source switch).
- The streaming room on desktop (publisher controls and the share control).

Don't implement this until the designs exist; a desktop layout invented in code will drift from the phone flows.

## 6. Frontend work items

| File                                                          | Change                                                                                                                                                                |
| ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `src/lib/room.svelte.ts`                                      | Replace camera-replacement in `toggleScreenShare` with a second transceiver on the publisher's connection; keep the `ended` handling. Track `screenSharing` per kind. |
| `src/lib/webrtc.ts`                                           | Publisher connection: add the screen transceiver and renegotiate using the existing perfect-negotiation path (publisher is impolite).                                 |
| `src/lib/signaling.ts`, `src/lib/contract.ts`                 | `request_track` with `track`; types for the stage's per-streamer track kinds.                                                                                         |
| `src/lib/stage/stage.ts`                                      | Desktop as a separate placeable entity under the same streamer (§3).                                                                                                  |
| `src/lib/components/VideoTile.svelte`, `FullStream.svelte`    | Source switch in the full view; monitor badge on tiles; "stopped sharing" state.                                                                                      |
| `src/routes/room/[id]/+page.svelte`, `.../stage/+page.svelte` | Hide the share control on unsupported devices; show "Sharing desktop" state; admin stage placement for desktop tiles.                                                 |

## 7. Testing

- **Unit** (`npm run test:unit`): the share state machine in `room.svelte.ts` (start, browser stop, fallback to camera, toggling while on the stage); stage placement for the desktop entity; capability check hides the control.
- **SFU** (`cargo test`): mirror `tests/media.rs` for a second track: a subscriber asking for `screen` gets only that track; a camera-only subscriber is unaffected; publisher leaves → both tracks stop.
- **Go** (`go test ./...` and `-tags e2e`): stage payload includes desktop state; blacklisting still works with a sharing publisher.
- **Integration (the Docker stack)**: extend `media-e2e` so the publisher starts `getDisplayMedia` and the watcher checks frames on the desktop source. Headless Chromium needs `--auto-select-desktop-capture-source` or an equivalent flag. **To verify**, it's not yet confirmed that this flag works in the Playwright build here.
- **Manual**: desktop Chrome and Firefox as publisher; a phone as watcher (desktop share visible, control hidden for the phone publisher).

## 8. Sequence

1. Decide the open questions below and draw the missing screens (share control, desktop layout) in Figma or as wireframes.
2. Contract change in `contracts.md` and `API-Contract.md` (`track` field, settled decisions).
3. SFU: keyed tracks per publisher, with Rust tests.
4. Go: stage payload carries desktop state; e2e tests.
5. Frontend publish side, then watch side, then stage/admin.
6. Desktop layout, once designed.
7. Integration test through the Docker stack.

## 9. Risks

- **Bandwidth**: a 1080p desktop stream on top of the camera roughly doubles uplink per publisher. Needs a bitrate cap and a sanity check on a home connection.
- **Mobile**: no desktop capture on phones, so a phone can only watch. Make that clear in the UI rather than failing silently.
- **Audio**: `getDisplayMedia` can include system audio, depending on the browser and OS. Not in scope unless the product wants it.
- **Browser differences**: Safari and Firefox differ on what the capture picker offers and on `ended` timing; test both.

## 10. Open questions

1. **Reading**: is "desktop display" the shared-screen stream (this plan), the desktop layout, or both? (Plan assumes the stream, with layout as a follow-up.)
2. **Placement**: should a desktop share be a separate stage placement (recommended) or replace the streamer's camera tile while live?
3. **Unsupported devices**: hide the share control on phones (recommended) or grey it out as the wireframe's disabled button does?
4. **Audio**: video only, or include system audio when the browser offers it?
5. **Fallback**: when a desktop share stops while watched, fall back to the camera (recommended) or leave the tile empty?
6. **Designs**: who draws the share control and the desktop layout? The current wireframes can't answer those.
