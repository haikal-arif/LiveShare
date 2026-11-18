# FlashLive — Local POV Live Stream

A web app for events. People near the front share their phone camera, and people at the back watch those streams, arranged on a map of the venue. It runs on a laptop on the venue's Wi-Fi, or on a cloud server when participants aren't on the same network.

## Overview & Assumptions

1. There are three roles. The **admin** creates the room, **sharers** publish their camera, and **watchers** view the streams. Nobody logs in.
2. One server hosts one room at a time. Creating a room needs the server's passcode, and whoever creates it is the admin.
3. Sharers and watchers join by scanning the room's QR code or typing the room code. Joining needs no passcode.
4. The admin can place sharers on a stage template, block a sharer by IP, and end the room.
5. All state is in memory. Restarting the backend ends the room and clears the blocklist.
6. Browsers only allow the camera on HTTPS, so the app is always served over HTTPS (except `localhost` in development).
7. Video goes over UDP between each browser and the media server. Networks that block this need the TURN relay, which is optional.

## What this Project Contains

1. A Go backend (`capstone_be/`) using gin. It owns rooms, admin auth, the streamer list, the stage layout, the blocklist, and live updates over SSE.
2. A Rust media server, the SFU (`capstone_sfu/`). It handles WebRTC signaling over a websocket and forwards video, with three quality layers per stream.
3. A SvelteKit frontend (`capstone_fe/`), built as a static single-page app.
4. An nginx gateway (`gateway/`). It is the one public entry point: TLS, routing, rate limits and the IP blocklist.
5. A Docker Compose stack (`docker-compose.yml`, `Makefile`, `scripts/`) that runs all of the above, plus an optional TURN relay (coturn).
6. Terraform for an AWS EC2 server (`terraform/ec2-deploy/`).
7. A load test that publishes streams from headless Chromium (`loadtest/`).
8. The API contract (`contracts.md`). This is the source of truth for every endpoint, signaling message and error code.
9. Documentation (`capstone_docs/`): product overview, technical architecture, deployment and TURN guides, weekly reports.

## How to Setup and Run The Project

The Docker stack is the normal way to run it. You need Docker with Compose v2, `make` and `openssl`. [mkcert](https://github.com/FiloSottile/mkcert) is optional: with it (run `mkcert -install` once) the browser on this machine trusts the certificate; without it the certificate is self-signed and each device shows a warning to accept once.

### 1. Configure the stack

```bash
make docker-setup
```

This writes `.env` with this machine's LAN IP, a random JWT secret and default ports, and creates the gateway's certificate. Running it again keeps the values already there.

Open `.env` and change the room passcode:

```
APP_ROOM_PASSCODE=<choose-a-passcode>
```

`APP_MAX_PARTICIPANTS` limits how many people a room accepts (sharers and watchers together). `0` means no limit. The file is git-ignored.

If the machine is on two networks at once (for example Wi-Fi plus USB tethering to a phone), add the second address so devices on both can join: `make docker-setup EXTRA_IPS=<other-ip>` (comma-separated for more; `EXTRA_IPS=none` removes them). The certificate must also cover that address; `gateway/gen-cert.sh` includes every IP the machine has when it runs.

### 2. Start the stack

```bash
make docker-up
```

The first build takes several minutes. When it finishes it prints the address, `https://<your-LAN-IP>:8443`. Open it on this machine or on any phone on the same network.

To also open the host firewall (`ufw`) for the network and stop everything on Ctrl-C, use `make run-all` instead. `make docker-logs` follows the logs and `make docker-down` stops the stack.

### 3. Run without Docker (development)

You need Go 1.26.5 or newer (the version in `capstone_be/go.mod`), a recent Rust toolchain, and Node.js 24.

```bash
make setup
make dev
```

`make setup` creates a `.env` in `capstone_be/`, `capstone_sfu/` and `capstone_fe/` and installs the frontend packages. It never overwrites an existing `.env`; delete one to have it written again.

`make dev` runs all three. Open `http://localhost:5173`. The backend listens on 8080. The SFU listens on 9000 on this machine only, so its internal API isn't reachable from the network. The default passcode is `changeme`; change `ROOM_PASSCODE` in `capstone_be/.env`.

The backend and the SFU don't read `.env` files themselves. The Makefile loads them, so start the services with `make`, or export the variables yourself.

### 4. Run the tests

```bash
make test
```

This runs the Go tests, the Rust tests and the frontend unit tests. `make help` lists every target.

### 5. Deploy on a cloud server

To make the app reachable from the internet, it runs on one AWS EC2 instance with a DuckDNS name, a Let's Encrypt certificate and the TURN relay. Terraform creates the server:

```bash
cd terraform/ec2-deploy
terraform init
terraform plan -var "ssh_cidr=<your-public-IP>/32" -out plan
terraform apply plan
```

Then, on the server:

```bash
make docker-setup EC2=1 DOMAIN=<name>.duckdns.org HTTPS_PORT=443 HTTP_PORT=80 TURN=1
make tls DOMAIN=<name>.duckdns.org EMAIL=<your-email>
make docker-up
```

The full procedure, including copying the project to the server and tearing it down, is in [`capstone_docs/ec2-deployment.md`](capstone_docs/ec2-deployment.md). The TURN relay is explained in [`capstone_docs/turn-relay.md`](capstone_docs/turn-relay.md).

## Using the App

1. **Admin:** open the app, choose **Create Room** and enter the passcode. The app shows a room code and a QR code to share.
2. **Sharer:** scan the QR code, or enter the room code by hand, and choose to share. Allow camera access, check the preview, and start streaming. On a desktop browser a sharer can switch between the camera and a screen.
3. **Watcher:** scan the QR code or enter the room code and choose to watch. The streams appear on the stage map, or in a plain grid when the admin hasn't set a stage.
4. Tap a stream to open it full screen. Pick **High**, **Mid** or **Low** quality. If the connection can't keep up, the server lowers the quality and marks it "auto"; pick a higher quality again to retry.
5. **Admin:** open the stage editor, pick a template (concert, conference, theater or blank) and drag sharers onto it. Watchers see the change at once.
6. **Admin:** block a sharer to disconnect them and stop their IP from joining again. End the room when the event is over.

If the admin's page is closed, opening the app again from the same IP gives the admin role back. From another IP, the admin token from the original device is needed.

## API

There are three interfaces. Full details are in `contracts.md`.

**Browser to backend (REST, under `/api`)**

| Method | Path                                     | Purpose                                                                     |
| ------ | ---------------------------------------- | --------------------------------------------------------------------------- |
| POST   | `/rooms`                                 | Create the room (passcode in the body); returns the room id and admin token |
| GET    | `/rooms/:room_id`                        | Everything needed to join: sharers, SFU address, ICE servers                |
| GET    | `/rooms/:room_id/publishers`             | Just the list of sharers                                                    |
| DELETE | `/rooms/:room_id`                        | End the room (admin)                                                        |
| POST   | `/rooms/:room_id/admin/reclaim`          | Get the admin token again from the creator's IP                             |
| PUT    | `/rooms/:room_id/streamers/:streamer_id` | Register or update a sharer                                                 |
| GET    | `/rooms/:room_id/stage`                  | Stage template, placements and sharers                                      |
| PUT    | `/rooms/:room_id/stage`                  | Replace the stage layout (admin)                                            |
| GET    | `/rooms/:room_id/events`                 | SSE stream: `stage_updated`, `room_closed`                                  |
| POST   | `/blacklist`                             | Block a sharer's IP and disconnect them (admin)                             |
| GET    | `/blacklist`                             | List blocked IPs (admin)                                                    |
| DELETE | `/blacklist/:ip`                         | Unblock an IP (admin)                                                       |
| GET    | `/health`                                | Liveness                                                                    |

Admin endpoints need the token in the `X-Admin-Token` header. Errors use one shape, `{ "error": { "code", "message" } }`.

**Browser to SFU (websocket at `/ws`)**: JSON signaling messages such as `join_as_publisher`, `join_as_subscriber`, `sdp_offer`, `sdp_answer`, `ice_candidate`, `request_track` and `set_preferred_layer`.

**Backend to SFU (REST under `/internal`)**: the sharer list, peer IPs, disconnecting a peer and closing a room. The gateway returns 404 for `/internal`, so it is never public.

## Backend Layout

```
capstone_be/
  cmd/webserver/    entry point: reads the environment, wires everything, registers the routes
  internal/
    domain/         types and errors
    dto/            JSON request and response shapes
    usecase/        business rules (room, streamer, layout, blacklist, events, liveness, jwt); defines the interfaces it depends on
    repository/     in-memory stores; mirrors the blocklist to a file for the gateway
    httpclient/     client for the SFU's internal REST API
    events/         fans events out to the open SSE streams
    handler/        HTTP handlers
    middleware/     admin auth, blacklist check, client IP, CORS
    reqctx/         the caller-IP context key
  e2e/              black-box tests against the built binary and a fake SFU
```

The usecase layer does not import `repository` or `httpclient`. It declares the interfaces it needs, and `cmd/webserver/router.go` is the only place that connects them.

The SFU (`capstone_sfu/crates/server/src/`) is layered the same way: `domain/` (types, errors, media interfaces), `service/` (the room logic), `infra/media/` (everything that touches the WebRTC library) and `transport/` (websocket and REST).

## Design Notes

- **Control and media are separate services.** The Go backend never touches video, and the SFU knows nothing about stages or admins. The backend asks the SFU who is connected instead of tracking it.
- **Quality layers without transcoding.** Each sharer sends three versions of their video (simulcast). The SFU forwards one per watcher and steps it down when that watcher's connection is weak. It never steps back up by itself.
- **Stateless admin token.** The token is a JWT carrying the room id. The server stores no tokens, and a token from one room doesn't work in the next.
- **Stable sharer identity.** The SFU gives a new `peer_id` on every connection, so the stage is keyed by a `streamer_id` kept in the browser. A refresh doesn't lose the sharer's place.
- **One public origin.** Only the gateway is reachable over HTTP. It sets the client IP header itself, which the blocklist and admin reclaim rely on.
- **Blocking is enforced twice.** The backend refuses a blocked IP when it asks to join, and the gateway refuses it on every path, including the SFU's websocket.
- **TURN is optional.** Browsers get the relay's address and credential in the ICE server list. They use it only when the direct path fails.
- **Participant limit in the SFU.** It is the only part that knows how many peers are really connected.

## What was deferred

1. Accounts and login. The passcode and the admin token are the only credentials.
2. Persistence. Nothing survives a backend restart.
3. Blocking watchers. The admin can only block sharers, since watchers aren't listed.
4. Stepping a watcher's quality back up automatically.
5. A separate stream for screen sharing. Today it replaces the camera.
6. Per-user TURN credentials. Everyone shares one username and password.
7. CI for this repository. The test suites run locally with `make test`.
8. More than one room per server, and more than one SFU.

## Architecture

1. The browser loads the app and calls the REST API through the nginx gateway over HTTPS.
2. The browser opens a websocket to the SFU through the same gateway for WebRTC signaling.
3. Video travels over UDP directly between the browser and the SFU, or through the coturn relay when the direct path is blocked.
4. The backend calls the SFU's internal REST API over the private Docker network.
5. The backend pushes stage changes and the end of the room to browsers over SSE.
6. On AWS, all of this runs with Docker Compose on one EC2 instance created by Terraform.

Diagrams and the reasoning are in [`capstone_docs/technical-architecture.md`](capstone_docs/technical-architecture.md).

## Trade Offs

1. Two services (Go and Rust) take more setup than one binary, and the backend depends on the SFU being up. In return each stays simple, and the layout matches how larger SFU systems are built.
2. In-memory state means no database to run, which suits a one-day event. A restart loses the room.
3. An SFU that only forwards video uses little CPU, but it can't make a lower quality itself. Sharers have to upload three layers.
4. Blocking by IP needs no accounts. It also blocks everyone behind the same address, and a blocked person can come back from another network.
5. One EC2 instance is simple and cheap to remove, but it is a single point of failure and costs money while it runs.
6. A shared TURN credential is easy to set up. Anyone who joins a room can read it.
7. Terraform state is a local file. That needs no extra setup, but the file must not be lost.

## Next Steps

1. Add CI that runs the three test suites.
2. Give "room is full" its own error code and message.
3. Generate short-lived TURN credentials per join.
4. Let the admin see and block watchers.
5. Publish screen sharing as a second stream next to the camera.
6. Step a watcher's quality back up when the connection recovers.
7. Keep the Terraform state in a remote backend.
