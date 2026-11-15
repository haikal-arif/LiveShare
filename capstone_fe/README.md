# FlashLive (capstone-fe)

Frontend for **FlashLive** ("Local POV Live Stream") — a local-network live-streaming app: an admin creates a room and shares a QR code, sharers (streamers) publish camera video, watchers (subscribers) view it. Built with SvelteKit 2 / Svelte 5 (runes).

The frontend talks to two sibling services (not in this repo): a Go control-plane webserver and a Rust SFU, per the contract documented in `capstone-docs/API-Contract.md`.

## Developing

```sh
npm install
cp .env.example .env   # set PUBLIC_API_BASE_URL to your local Go webserver
npm run dev
```

## Building

```sh
npm run build
npm run preview   # serve the production build locally
```

## Testing

Three layers, run independently:

| Layer            | Command                      | What it covers                                                    |
| ---------------- | ---------------------------- | ----------------------------------------------------------------- |
| Type-check       | `npm run check`              | `svelte-check` across all `.svelte`/`.ts` files                   |
| Unit (Vitest)    | `npm run test:unit -- --run` | Pure logic in `src/lib/*.spec.ts` (omit `--run` to watch instead) |
| E2E (Playwright) | `npm run test:e2e`           | Page navigation/rendering against a real production build         |
| Everything       | `npm test`                   | Unit, then e2e                                                    |

### Unit tests (`src/lib/*.spec.ts`)

Each covers one module in isolation, with the browser/network boundary faked so tests are deterministic and don't need a running backend:

- **`roomcode.spec.ts`** — `generateRoomCode()` produces an `XXX-XXX` code with no visually-ambiguous characters (`0/O/1/I`); `normalizeRoomCode()` trims/uppercases user input consistently with scanned QR text.
- **`qr.spec.ts`** — `buildRoomJoinUrl()`/`extractRoomIdFromScan()` round-trip correctly, and gracefully fall back to raw text for non-URL or room-less scans.
- **`api.spec.ts`** — `getRoomDetails`/`getRoomPublishers` hit the right path per `API-Contract.md` (`GET /api/rooms/:room_id[/publishers]`), URL-encode the room id, and surface the contract's `{ error: { message } }` shape (or a sensible fallback) on non-200 responses. `fetch` is stubbed with `vi.stubGlobal`, so no server is contacted.
- **`signaling.spec.ts`** — `connectSignaling()`'s open/error resolution, per-type message dispatch, and unsubscribe behavior, against a hand-rolled fake `WebSocket` (`vi.stubGlobal('WebSocket', FakeWebSocket)`) so it never touches a real socket.
- **`webrtc.spec.ts`** — the actual negotiation rules from `API-Contract.md`: the publisher is always the offerer and always uses `target_peer: null`; a subscriber's per-publisher connection sends `request_track` immediately and only answers `sdp_offer`s addressed to it (`from_peer` filtering). Verified against a fake `RTCPeerConnection` (`vi.stubGlobal`), since jsdom/Node have no real WebRTC stack.

Run a single file directly with `npx vitest run src/lib/webrtc.spec.ts`.

**Intentionally not unit-tested:** `room.svelte.ts` (the session store) and the Svelte pages themselves. `room.svelte.ts` is a thin composition of the four modules above (already covered) plus real browser state (`MediaStream`, live sockets); testing it meaningfully would mean re-mocking all of the above behind module boundaries for little extra signal over the e2e tests below, which exercise it for real in a browser.

### E2E tests (`src/routes/flashlive-flows.e2e.ts`)

Run with Playwright against `npm run build && npm run preview` (see `playwright.config.ts`), in a real Chromium instance with no camera/mic permissions granted and no backend running. That last part is deliberate: several assertions pin the _honest failure_ behavior when `capstone-be`/`capstone-sfu` aren't reachable (e.g. the subscriber room shows `Failed to fetch` rather than a fake stream) — see [`flashlive-flows.e2e.ts`](src/routes/flashlive-flows.e2e.ts). Covers:

- Splash → home, and the three home actions are present.
- Landing via a shared-room link (`/home?room=...`) disables "Create Room".
- Create Room renders a room code and a QR canvas.
- Manual "Enter ID Room" join, for both the streamer path (→ camera-preview screen) and the watch path (→ straight into the room, no camera step — matches the wireframes).
- The scan screen requests camera access and reports failure when it's unavailable (the default, permission-less e2e environment), then the back button returns to the manual-entry screen.

**Intentionally not covered:** any scenario requiring a live camera/mic (headless CI has neither by default) or a live `capstone-be`/`capstone-sfu` pair — that's an integration concern for once those services exist end to end, not something this repo can fake convincingly without misrepresenting what actually works.

> To deploy your app, you may need to install an [adapter](https://svelte.dev/docs/kit/adapters) for your target environment.
