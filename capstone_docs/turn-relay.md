# TURN relay (coturn)

FlashLive sends media over UDP straight between each browser and the SFU. Some networks block that: mobile carriers behind carrier-grade NAT, venue or office Wi-Fi that only allows web traffic. On those networks the page loads and signaling works, but no video arrives.

A TURN server fixes this. The browser sends its media to the TURN server on a port the network does allow, and the TURN server passes it on to the SFU. FlashLive uses [coturn](https://github.com/coturn/coturn) for this, as an optional container in the Docker stack.

TURN is off by default. On a LAN where every device can reach the host directly it isn't needed. It's meant for the EC2 deployment, where watchers and sharers come from the public internet ([ec2-deployment.md](./ec2-deployment.md)).

## 1. How it fits in

```
browser ──https/wss :443──────────▶ gateway ─▶ fe / be / sfu      (pages, API, signaling)
browser ──udp 50000-50199─────────▶ sfu                           (media, direct: the normal path)
browser ──3478 udp/tcp, 5349 tls──▶ coturn ──udp──▶ sfu           (media, relayed: the fallback)
```

Nothing in the app decides when to relay. ICE in the browser tries the direct path and the relayed path, and uses the relay only when the direct one fails.

What each part does:

| Part | Role |
| --- | --- |
| `scripts/docker-env.sh` | `TURN=1` sets `APP_TURN_ENABLED=1`, builds `APP_TURN_URLS` from `APP_PUBLIC_HOST`, and sets `COMPOSE_PROFILES=turn`. It also creates `APP_TURN_USER` (`capstone`) and a random `APP_TURN_PASSWORD` the first time. |
| `docker-compose.yml`, service `turn` | `coturn/coturn:4.6.2`, in the `turn` profile, so it only starts when `COMPOSE_PROFILES=turn`. |
| Go webserver | Reads `TURN_URLS`, `TURN_USERNAME`, `TURN_CREDENTIAL` and adds one TURN entry to `ice_servers` in `GET /api/rooms/:room_id`. |
| Rust SFU | Reads the same three variables, adds the entry to `ice_servers` in the `joined` message, and uses the same list for its own peer connections. |
| Frontend | Passes `urls`, `username` and `credential` from `ice_servers` to `RTCPeerConnection` (`toRtcIceServers` in `src/lib/webrtc.ts`). |

With TURN off, `TURN_URLS` is empty, no TURN entry is added, and `ice_servers` holds only the STUN server.

## 2. Turning it on and off

```bash
make docker-setup TURN=1    # or TURN=0 to turn it off
make docker-up
```

`make docker-up` prints `turn: running` or `turn: off` at the end. The setting is kept in `.env`, so later `make docker-setup` runs without `TURN=` leave it as it is.

Check that it's running:

```bash
docker compose ps turn
docker compose logs --tail=50 turn
```

## 3. Ports

All of these must be open to clients (the security group on EC2, the host firewall on a LAN).

| Port | Protocol | Purpose |
| --- | --- | --- |
| 3478 | UDP and TCP | TURN |
| 5349 | TCP | TURN over TLS (`turns:`), for networks that only allow TLS |
| 49152–49200 | UDP | Relayed media, one port per relayed connection |

The relay range comes from `APP_TURN_RELAY_MIN` and `APP_TURN_RELAY_MAX`. If you change them, change the firewall rule too.

`firewall.sh` (used by `make run-all` on a LAN) opens only the gateway ports and the SFU's UDP range. It doesn't open the TURN ports.

## 4. Configuration

Everything is set in `.env` by `make docker-setup`. There is no `turnserver.conf`; the compose file passes the options as flags.

| `.env` key | Used for |
| --- | --- |
| `APP_TURN_ENABLED` | `1` or `0`. Set with `TURN=1` / `TURN=0`. |
| `APP_TURN_USER`, `APP_TURN_PASSWORD` | The one username and password every client uses (`--lt-cred-mech`, `--user`). |
| `APP_TURN_URLS` | The URLs handed to browsers. Rewritten on every `make docker-setup`: `turn:<host>:3478?transport=udp`, `turn:<host>:3478?transport=tcp`, `turns:<host>:5349?transport=tcp`. |
| `APP_PUBLIC_HOST` | The host in those URLs, and coturn's `--realm`. |
| `APP_HOST_IP` | `--external-ip`: the address coturn gives clients for relayed media. On EC2 this is the Elastic IP. |
| `APP_TURN_RELAY_MIN`, `APP_TURN_RELAY_MAX` | Relay port range (default 49152–49200). |
| `COMPOSE_PROFILES` | `turn` when enabled, empty otherwise. Derived; don't edit by hand. |

Two details in the compose service are deliberate:

- **Host networking.** coturn needs the machine's real network interface and a whole UDP range. With bridge networking every relay port would have to be published one by one.
- **No `-n` flag.** The image's entrypoint passes each argument through `echo`, which swallows `-n`. Without a config file coturn only logs a warning and uses the flags.

coturn uses the gateway's certificate (`gateway/certs/capstone.crt` and `.key`, mounted read-only) for `turns:`. `make tls` replaces those files and then restarts the gateway and, when TURN is on, coturn, so both load the new certificate. Relayed streams drop for a moment during that restart.

## 5. Checking that the relay works

1. Open the app on a phone on mobile data (Wi-Fi off) and join a room as a sharer. Watch from another device.
2. On a desktop Chrome, open `chrome://webrtc-internals` while watching. The selected candidate pair shows `relay` as the candidate type when the media is going through coturn, and `host` or `srflx` when it's direct.
3. `docker compose logs turn` shows allocations as clients connect.

If video works on Wi-Fi but not on mobile data, check the three port rules above first, then that `.env` has `COMPOSE_PROFILES=turn`.

## 6. Limits

- **One shared credential.** Every client gets the same username and password in the room response, so anyone who joins a room can read them and use the relay. That's acceptable for a demo. The stricter option is short-lived credentials generated per join (coturn's `--use-auth-secret`), which needs a change in the Go webserver.
- **Small relay range.** The default range has 49 ports, and each relayed connection takes one. The SFU is given the same TURN entry, so its own peer connections may take ports as well. This hasn't been measured. For a larger room, widen `APP_TURN_RELAY_MIN`/`APP_TURN_RELAY_MAX` and the firewall rule together.
- **Relayed media costs bandwidth.** Everything relayed goes in and out of the server twice (once through coturn, once through the SFU).
- **Not a privacy feature.** TURN only changes the route. Media is encrypted by WebRTC (DTLS-SRTP) either way.
