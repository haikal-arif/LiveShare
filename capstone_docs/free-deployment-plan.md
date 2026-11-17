# Plan: free proof-of-concept deployment

Status: **superseded.** The app was deployed on one AWS EC2 instance instead, with the frontend served by the gateway rather than Vercel, and with a TURN relay. See [ec2-deployment.md](./ec2-deployment.md). This plan is kept for the hosting comparison in section 3. Originally: planning only, nothing deployed or changed. Goal: a working demo of the whole app on free tiers. The frontend goes on Vercel. The Go backend, the Rust SFU, and the gateway go on one free VM, because the SFU needs open UDP media ports, which no free PaaS offers.

## 1. Target architecture

```
Browser ──https──▶ Vercel (static SvelteKit build, capstone_fe)
   │
   ├──https/wss──▶ Free VM (public IP + domain)
                    ├─ gateway (nginx, TLS, /api → be, /ws → sfu)
                    ├─ be  (Go, REST + SSE)
                    └─ sfu (Rust, WebRTC signaling on 9000 behind gateway)
   │
   └──UDP 50000–50199──▶ Free VM (SFU media, direct from browsers)
```

Why this split:

- Vercel can only host the static frontend. Its serverless functions don't suit a long-running Go server with SSE, and it can't host the SFU's UDP media.
- The VM is the only free host here that accepts inbound UDP. See §3 for options.

## 2. What the code needs (from the repo)

| Item | Where | Change for PoC |
|---|---|---|
| Frontend API base URL | `capstone_fe/src/lib/api.ts` reads `PUBLIC_API_BASE_URL`, baked in at build time (`$env/static/public`) | Set it in Vercel to `https://<domain>` before building. |
| Frontend build | `capstone_fe/vite.config.ts` uses `adapter-static` with `fallback: 'index.html'` | None. Vercel builds `npm run build` and serves `build/`. Add a `vercel.json` only if client-side routes 404. |
| Backend CORS | `ALLOWED_ORIGINS` in `docker-compose.yml` (set to the gateway's LAN address) | Add the Vercel origin, e.g. `https://<app>.vercel.app`. Keep the existing entry if you still need LAN testing. |
| SFU advertised IP | `SFU_PUBLIC_IPS` in `docker-compose.yml` | Set to the VM's public IP. |
| SFU WebSocket URL | `SFU_WS_URL` in `docker-compose.yml` | `wss://<domain>/ws` (already the default in compose). |
| TLS certificate | `gateway/gen-cert.sh` makes a self-signed cert, which browsers reject from Vercel | Use a real certificate, e.g. Let's Encrypt via certbot on the VM. |
| ICE servers | `stun:stun.l.google.com:19302` | Keep for the PoC. Note that some networks need a TURN server, which is not in scope. |

## 3. Hosting options checked (free tier)

| Option | Fits | Notes |
|---|---|---|
| **Oracle Cloud Always Free ARM VM** (recommended) | Backend, SFU, gateway | Public IP, UDP via security list plus iptables. Up to 4 cores and 24 GB RAM. Risks: ARM "out of capacity" errors, and one report of broken outbound IPv4. Sources: [Oracle docs](https://docs.oracle.com/en-us/iaas/Content/FreeTier/resourceref.htm), [guide](https://dev.to/build996/oracle-cloud-always-free-get-a-4-core-24gb-arm-vps-for-free-4l2o), [limits](https://space-node.net/blog/oracle-cloud-always-free-limits-2026) |
| Render free web service | Backend only (not SFU) | Sleeps after 15 min idle, 30–60 s cold start, 512 MB RAM. No evidence it accepts UDP. Source: [Render 2026](https://temps.sh/blog/render-pricing-2026) |
| Fly.io | Not a free option | Short trial, then pay-as-you-go. Source: [Fly.io plan](https://costbench.com/software/cloud-infrastructure/fly-io/free-plan/) |
| Home PC or Raspberry Pi + port forwarding + DDNS | Backend, SFU, gateway | Free. Breaks behind CGNAT. Machine must stay on. Not verified in this session. |
| Google Cloud e2-micro / AWS 12-month free tier | Backend, SFU, gateway | Not verified. Check UDP and egress limits before use. |

## 4. Steps

### 4.1 VM (Oracle Always Free)
1. Create an Always Free ARM instance (Ubuntu, Ampere A1) in a public subnet with a public IPv4 address. If capacity is unavailable, try another availability domain or retry later.
2. In the VCN security list, open TCP 80 and 443 (certbot and HTTPS) and UDP 50000–50199 (SFU media). Open TCP 22 only from your own IP.
3. On the VM, open the same ports in iptables (or `ufw`), since Ubuntu images apply their own rules. Save the rules so they survive a reboot.
4. Install Docker and the compose plugin. Clone the repo.
5. Point a free domain (DuckDNS or similar) at the VM's public IP.

### 4.2 TLS
6. Get a certificate for the domain with certbot (`certbot certonly --standalone -d <domain>`). Mount it into the gateway in place of the self-signed cert, and update `gateway/` if its paths differ.

### 4.3 Backend stack
7. Run `make docker-setup` on the VM. Set `APP_HOST_IP` to the VM's public IP or domain, `APP_ROOM_PASSCODE`, and `APP_JWT_SECRET`.
8. In `docker-compose.yml`, add the Vercel origin to `ALLOWED_ORIGINS`. Set `SFU_PUBLIC_IPS` to the VM's public IP.
9. `make docker-up`. Check `docker compose logs be sfu gateway`.
10. Verify from outside: `curl https://<domain>/api/...` returns a response, and `wss://<domain>/ws` connects.

### 4.4 Frontend on Vercel
11. Import the repo (or the `capstone_fe` folder) into Vercel. Build command `npm run build`, output directory `build`.
12. Set `PUBLIC_API_BASE_URL=https://<domain>` in Vercel environment variables, then deploy.
13. Open the Vercel URL, create a room, and join from a second device on a different network to test media over UDP.

## 5. Risks and limits

- **Not production.** The VM is a single point of failure, the passcode and JWT secret are simple, and nothing is monitored.
- **Oracle reclaims idle instances.** Keep the VM busy, or check its usage before relying on it.
- **Without TURN,** some networks (strict NATs, corporate firewalls) will fail to connect media. This is expected for a PoC.
- **Free-tier terms change.** Re-check Oracle's, Vercel's, and Render's limits before the demo.

## 6. Open questions

- Which domain to use (DuckDNS, or an existing one)?
- Is a LAN-only fallback still needed alongside the public deployment?
- Who is the demo audience? Their network decides whether TURN matters.
