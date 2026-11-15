# Product Overview

> This is the product-level companion to `backend.md` (covers API + database) and `frontend.md`. Update the status and change log as your project evolves.

## Document Info

| Field | Value |
| --- | --- |
| Project name | Local POV Live Stream |
| Status | In Progress (8 of 9 original features Done; feature 5 in progress; features 10–15 Done) |
| Owner | Muhammad Haikal AlFarizi (kal) |
| Target live date | Capstone Week |
| Summary | A local web app that allows event participants to stream live video feeds over a shared network so attendees in the back can watch different points of view. |

## Change Log

| Date | Change |
| --- | --- |
| 2026-08-17 | Converted original ideation doc to Product Overview format |
| 2026-08-31 | Added US-3 acceptance criteria for stream rearranging and sharer kick/IP block; expanded Admin edge case handling; added Room Management features to Scope table; clarified admin role is auto-assigned to room creator (no login); stated one room per network/server instance; added admin reclaim flow (same IP, then secret key fallback), MVP capacity (2 sharers / 5 watchers), and blocklist/room data lifetime tied to server uptime |
| 2026-09-30 | Defined Automatic Quality Adjuster approach: sharers send simulcast layers (high/mid/low), the server picks a layer per watcher from reported connection stats (no transcoding); added low-bandwidth edge cases; replaced the adaptive-bitrate future improvement with a manual quality override |
| 2026-09-30 | Watchers pick a quality per stream (High/Mid/Low); the server lowers it automatically when the connection can't keep up and marks it "auto"; watchers retry a higher quality by picking it again (no automatic step up) |
| 2026-11-03 | Status sync against the codebase: features 1–4, 6–9 marked Done; feature 5 (network discovery) In Progress; added features 10–15 that are implemented but were missing from the scope (real-time events, streamer live status, room close, manual code join, connection stats, Docker Compose deployment) |
| 2026-10-14 | Aligned with the API contract: room creation needs the server passcode; the admin credential is a token (the "secret key"); layout is a stage template plus streamer placements, and an offline streamer's placement stays (greyed out); blocking a sharer's IP also disconnects them (no separate kick); removed the 2-sharer/5-watcher cap and the block-own-IP guard |

## 1. Background & Problem

| # | Problem | Solution |
| --- | --- | --- |
| 1 | Event attendees (e.g., at concerts or conferences) sitting in the back have poor visibility, while hiring dedicated camera crews is budget-prohibitive. | A web app allowing attendees and crew at the front to stream high-bitrate, low-latency video over a local server to everyone on the network. |

## 2. Scope

| # | Feature | Description | Priority | Status |
| --- | --- | --- | --- | --- |
| 1 | Camera Video Feed Streaming | Capture video via browser camera and send the live feed to the local server. | P0 | Done |
| 2 | Local Video Streaming Server | Server logic to receive camera feeds and distribute them across the local network. | P0 | Done |
| 3 | Video Watcher Dashboard | Web interface listing available streams for users to watch. | P0 | Done |
| 4 | Stream Selection (Video Picker) | Allows watchers to select a specific stream so only that feed is transferred. | P1 | Done |
| 5 | Auto Connect & Friendly UI | Seamless network discovery and responsive user interface. | P1 | In Progress |
| 6 | Automatic Quality Adjuster | Sharers send the feed in multiple quality layers (simulcast: high/mid/low); the server tracks each watcher's connection quality and forwards the layer that fits their bandwidth, so one weak watcher doesn't degrade the stream for others. | P1 | Done |
| 7 | Room Creation & QR Access | Admin creates a room (gated by the server passcode) and generates a QR code that sharers and watchers scan to join. | P0 | Done |
| 8 | Room Layout Management | Admin picks a venue template (concert, conference, theater, blank) and places each streamer on it; watchers see streams arranged that way. | P1 | Done |
| 9 | Sharer Moderation (IP Block) | Admin blocks a sharer's IP: the sharer is disconnected immediately and can't rejoin; the blocklist can also be reversed. | P1 | Done |
| 10 | Real-time Room Events | Pushes stage changes, streamer live/offline updates, and room end to every connected client over a per-room event stream. | P1 | Done |
| 11 | Streamer Registration & Live Status | Streamers register under a room and are marked live or offline from the SFU's connection state; offline streamers keep their placement. | P1 | Done |
| 12 | Room Close (End of Event) | Admin closes the room, which ends it for all participants and releases the SFU room. | P1 | Done |
| 13 | Manual Room Code Join | Participants can enter a room code by hand as an alternative to scanning the QR code. | P1 | Done |
| 14 | Connection Stats Reporting | Clients report packet counters every 2 seconds; the SFU exposes per-room stats that feed the quality adjuster. | P1 | Done |
| 15 | Local Deployment (Docker Compose + HTTPS Gateway) | The frontend, Go backend, Rust SFU, and an HTTPS gateway are packaged to start together with Docker Compose on the local network. | P1 | Done |

## 3. User Stories

### US-1 — Local Camera Streaming (Sharer)

**As an** event participant or crew member, **I want to** stream my phone's camera feed to the local server **so that** people in the back can see what is happening from my point of view.

**Acceptance Criteria**

* Given camera permissions are granted, when the user connects to the server and starts streaming, then the video feed begins broadcasting to the local server.
* Given a user stops the stream or leaves the page, then the feed is safely disconnected and removed from the active streams list.

### US-2 — Stream Viewer Dashboard (Watcher)

**As a** concert/conference attendee, **I want to** view a list of available live streams on the local network **so that** I can pick a feed with better visibility of the speaker or stage.

**Acceptance Criteria**

* Given the user opens the web app on the local network, when streams are active, then all available video feeds are listed on the dashboard.
* Given a watcher selects a specific stream, then the app displays that video feed in real time.

### US-3 — Room Management (Admin)

**As a** concert/conference planner, **I want to** create a video sharing room that I can manage **so that** I can arrange stream layout so watcher can have better experience.

**Acceptance Criteria**

* Given the user opens the web app there will be option to create a room.
* Given the user enters the server's passcode (set by whoever runs the server), when they create a room, then that user is automatically assigned the admin role for that room — no separate login or account is required, since the app runs fully within the local network. A missing or wrong passcode means no room is created.
* Given a room already exists on the server instance, when another user tries to create a new room, then the app prevents it and instead directs them to join the existing room (as sharer, watcher, or via the QR code) — only one room is supported per network/server instance.
* Given the admin's device disconnects, when the admin reconnects from the same IP address, then admin privileges are automatically reclaimed for that room.
* Given the admin reconnects from a different IP (or the reclaim otherwise fails), when they enter the room's secret key (the admin token), then admin privileges are reclaimed; the secret key is issued when the room is created and only accessible to whoever has physical access to the admin's original device/session.
* Given the option is selected, the app will generate QR code that can be scanned by both sharer and watcher.
* Given the room is created, there will be option to configure room layout.
* Given the admin is configuring the layout, when they pick a venue template (concert, conference, theater or blank) and drag streamers to spots on it, then saving stores exactly that arrangement and it is reflected for all watchers in real time. With no template set, watchers see the plain grid.
* Given a streamer is registered but not placed on the template, then watchers see that stream in a "More streams" row.
* Given a sharer is actively streaming, when the admin blocks that sharer, then the sharer is immediately disconnected, their feed is removed from the active streams list, and their IP address is added to the room's blocklist.
* Given an IP is on the blocklist, then any attempt to join the room from that IP is rejected.
* Given an IP is on the blocklist, when the admin chooses to unblock it, then the IP is removed from the blocklist and can join again as a sharer.

## 4. Functional Requirements

### 4.1 — Live Camera Feed Broadcast (Sharer)

**Summary**
Captures live video from the browser media devices and streams it over the local Wi-Fi connection to the central local server.

**Flow**

1. Sharer connects to the event Wi-Fi network.
2. Sharer scans the QR code to open the web app.
3. Sharer grants browser camera permissions.
4. Sharer connects to the server and begins streaming their point of view.

**Edge Cases Handling**

| Edge Case | Expected Behaviour |
| --- | --- |
| User denies camera permissions | Show an error message explaining that camera access is required to stream. |
| Network disconnects mid-stream | Display a reconnection prompt and stop transmission until reconnected. |
| Sharer's browser/device can't send simulcast layers | Stream continues with a single high-quality layer; watchers on low bandwidth can't be downgraded for that feed. |

### 4.2 — Stream Viewer (Watcher)

**Summary**
Allows users on the local network to discover active video feeds and watch a selected stream.

**Flow**

1. Watcher connects to the event Wi-Fi network.
2. Watcher scans the QR code to open the app dashboard.
3. Watcher selects a stream from the available streams list.
4. Watcher views the live stream directly in the browser.

**Edge Cases Handling**

| Edge Case | Expected Behaviour |
| --- | --- |
| The selected stream goes offline | Display a "Stream Ended" notice and prompt the user to select another feed. |
| No streams are currently active | Display an empty state message ("No active feeds right now"). |
| Watcher's connection is weak (high packet loss / low bandwidth) | Server switches that watcher to a lower quality layer without interrupting playback. The quality picker on that stream changes to the lower level, tagged "auto". It stays there until the watcher picks a higher quality again to retry; if the connection still can't keep up, it's lowered again. |

### 4.3 — Room Management (Admin)

**Summary**
Allows event owner to create and manage their video sharing room.

**Flow**

1. Admin opens the app, enters the server passcode and creates a room; whoever creates the room is automatically the admin for that room (no login/account needed, since the whole app is local-network-only).
2. Admin will see the default watcher dashboard
3. Admin picks a template and places streamers on it
4. Admin can block a sharer: the sharer is disconnected and their IP goes on the blocklist

**Edge Cases Handling**

| Edge Case | Expected Behaviour |
| --- | --- |
| A streamer on the stage goes offline (or the admin saves a layout while one is offline) | The streamer stays registered and keeps their spot; watchers see the pin greyed out until they return or the room closes. |
| Admin blocks a sharer who has already disconnected on their own | Nothing is blocked; show a message that the sharer is no longer connected rather than a generic error. |
| Admin blocks an IP that is shared by multiple devices (e.g., behind the same NAT/router) | Warn the admin before confirming that blocking this IP will also stop other legitimate devices on the same network from joining. Only the named sharer is disconnected right away. |
| Room is created but no sharers or watchers have connected yet | Show an empty state on the admin dashboard ("Waiting for participants") along with the QR code for easy re-sharing. |
| Admin's session disconnects or the admin refreshes the page | Room configuration (layout, blocklist) persists on the server and is restored when the admin reconnects from the same IP; if the IP changed, the admin is prompted to enter the room's secret key to reclaim admin privileges. |
| Two admins (or an admin on two devices) try to change the layout at the same time | Last write wins; apply the most recent layout change and sync it to all connected clients. |
| Server/room shuts down at the end of the event | The blocklist and room state exist only for the life of the server process (an event or up to a day); nothing persists once the server is stopped. |

## 5. Non-Functional Requirements

| # | Category | Detail |
| --- | --- | --- |
| 1 | Performance | Low latency transmission over the local Wi-Fi network with high bitrate support. |
| 2 | Usability | One-touch connection flow via QR code scanning without requiring account creation. |
| 3 | Network Constraints | Operate entirely within a local network without requiring public internet access. |
| 4 | Room Model | One room supported per network/server instance for the MVP; the server does not manage multiple concurrent rooms. |
| 5 | Data Lifetime | Room state and the IP blocklist live only as long as the server process is running (an event or up to a day); nothing persists after the server stops. |

## 6. Known Limitations & Future Improvements

* **Out of Scope (MVP):** Accessing streams over the public internet (restricted purely to local network connections).
* **Known Limitation:** The IP blocklist is enforced by the web server when a client asks to join. A client that connects straight to the media server can still get in until the blocklist is also checked there. Only sharers can be blocked for now; watchers have no ID the admin can see.
* **Future Improvement:** Automatically step a downgraded watcher back up once the connection recovers, which needs the server to probe for spare bandwidth rather than rely on the watcher's current receive rate.

## 7. WireFrame 
[wireframe](https://www.figma.com/design/98MtNCX0c3F1Ucf0aPPFBn/Untitled?node-id=3-65&t=1xzyDQbJRnp9ipzI-1)