# Weekly Report — Week [19]

| Field        | Value                                                                                        |
| ------------ | -------------------------------------------------------------------------------------------- |
| Project name | _Local live video sharing_                                                                    |
| Trainee      | _M Haikal A_                                                                                 |
| Week / dates | 19 / (2026-11-05 – 2026-11-17)                                                                 |
| Phase        | Implementation                                                                                |
| Checkpoint   | _The app can run on the public internet: one EC2 server created with Terraform, a TURN relay (coturn) for networks that block direct media, a trusted certificate, and a participant limit. The four repos are now one monorepo. Docs and README updated to match_ |

## 1. This Week's Goal

Make the app reachable from outside the local network for the demo: deploy the whole stack on a cloud server, add a TURN relay so phones on mobile data can send and receive video, and make the deployment repeatable. Alongside that, settle the repository layout question left open last report.

## 2. Progress & Blockers

One row per deliverable. Required before consultation (Section 6 of the policy) — whenever status isn't Done, don't just name the problem in the blocker cell, name what you tried, and flag whether you need help on it.

| Deliverable            | Planned this week          | Actual status     | Blocker / What I tried                                                                                  | Evidence                    |
| ---------------------- | --------------------------- | ----------------- | -------------------------------------------------------------------------------------------------------- | --------------------------- |
| Repository layout | Pick the canonical line for `capstone-be` and fix the parent repo | Done, by a different route | Instead of choosing between the two backend histories and repairing submodule pointers, all components were imported into one monorepo. The per-component histories are not carried over; the old GitLab repos still hold them. | Commit `fee5232` (2026-11-15), remote `capstone-monorepo` on GitHub |
| TURN relay (coturn) | Not planned last week; added for the public deployment | Done | `turn` service in the compose file (`coturn/coturn:4.6.2`, `turn` profile, host networking). The Go webserver and the SFU read `TURN_URLS`, `TURN_USERNAME`, `TURN_CREDENTIAL` and add one TURN entry to `ice_servers`; the frontend passes the credential to `RTCPeerConnection`. Turned on and off with `make docker-setup TURN=1\|0`. One problem on the way: the coturn image's entrypoint swallows a leading `-n`, so the flag was dropped. | `docker-compose.yml`; `capstone_be/cmd/webserver/main.go` (`turnServers`); `capstone_sfu/crates/server/src/config.rs` (`turn_server`); `capstone_fe/src/lib/webrtc.ts`; `capstone_docs/turn-relay.md` |
| Participant limit | Not planned last week | Done | `MAX_PARTICIPANTS` in the SFU (`APP_MAX_PARTICIPANTS` in `.env`), sharers and watchers counted together, `0` = no limit. A join past the limit gets `error` with code `invalid_state` and "room is full". It has no error code of its own, so the frontend can only recognize it by the text. | `capstone_sfu/crates/server/src/service/room/actor.rs` (`handle_join`); `contracts.md` section 3 |
| EC2 server with Terraform | Not planned last week | Done | `terraform/ec2-deploy/main.tf` creates the security group, one `c7i-flex.large` Ubuntu 24.04 instance and an Elastic IP. Applied on 2026-11-16 in `ap-southeast-2`; the state shows the instance running with the Elastic IP attached. The region is Sydney because the account's organization policy denied EC2 in `ap-southeast-1`. A further plan was saved on 2026-11-17, and the state file was not modified after it, so I can't tell from the files whether it was applied. | Terraform state (serial 7, local, not committed); `terraform/ec2-deploy/main.tf` |
| Stack setup for a public server | Not planned last week | Done | `scripts/docker-env.sh` now updates `.env` from options (`EC2=1` reads the public IP from instance metadata, `DOMAIN=`, `HTTPS_PORT=`, `HTTP_PORT=`, `TURN=`) and keeps existing secrets. `make tls` gets a Let's Encrypt certificate with certbot and installs it for the gateway and coturn. `gen-cert.sh` uses mkcert locally and falls back to self-signed. | `Makefile` (`docker-setup`, `tls`); `scripts/docker-env.sh`; `gateway/gen-cert.sh` |
| App running on the EC2 server | Not planned last week | **Not recorded** | The deployment steps were followed from the monorepo (rsync to the instance, `make docker-setup`, `make tls`, `make docker-up`), but nothing in the repo records the result: whether the site loads on the DuckDNS name with a trusted certificate, and whether a phone on mobile data can share and watch through the relay. To be filled in before the consultation. | — |
| Blocklist and rate limits at the gateway | Close the "blacklist bypass via the SFU" gap | Done | The Go webserver mirrors blocked IPs into a file of nginx `deny` lines on a volume shared with the gateway; a watcher reloads nginx when it changes, without dropping connections. A blocked IP is now refused on every path, including `/ws`. The gateway also limits request rates (20 r/s per address; 10 r/min for room creation and admin reclaim). The gap remains only for local `make dev`, which has no gateway. | `capstone_be/internal/repository/gateway_blocklist.go`; `gateway/docker-entrypoint.d/40-blocklist-watch.sh`; `gateway/templates/default.conf.template` |
| Full stream view (`capstone_fe`) | Not planned last week | Done | A watcher can open one stream full screen, with the quality picker, a mini stage map and previous/next. | `capstone_fe/src/lib/components/FullStream.svelte`; `capstone_fe/src/routes/room/[id]/+page.svelte` |
| Documentation | Keep docs in line with the code | Done | Added `ec2-deployment.md` (Terraform and stack procedure) and `turn-relay.md`. Updated the technical architecture (gateway, TURN, deployment section), product overview (features 16–21), API contract (participant limit), `context.md`, the docs index, and a new project `README.md`. `capstone_docs/API-Contract.md` and `capstone_sfu/API-Contract.md` are synced with `contracts.md`; `capstone_fe/API-Contract.md` is still an old copy. | `capstone_docs/`; `README.md`; `contracts.md` |
| Tests | Keep all suites green | Done (unit and integration; Playwright not run) | Run on 2026-11-17. Backend: `go vet ./...` and `go test ./...` pass. SFU: `cargo test --workspace` passes, 63 tests (42 unit, 21 integration). Frontend: `vitest --run` 15 files, 66 tests pass; `svelte-check` 0 errors, 0 warnings across 418 files. Playwright e2e and the backend's `e2e` tag were not run. | Test output 2026-11-17 |
| CI | Check the GitLab pipelines for the repos with CI | Not done; changed by the monorepo | The `.gitlab-ci.yml` files are now in `capstone_be/` and `capstone_sfu/` subfolders of a GitHub repo, where nothing runs them. There is no CI for the monorepo yet. Options: a GitHub Actions workflow that runs the three suites, or mirror the monorepo to GitLab with a root pipeline. I haven't chosen. | `capstone_be/.gitlab-ci.yml`; `capstone_sfu/.gitlab-ci.yml` |
| SFU bind address (`SFU_ADDR` vs `BIND_ADDR`) | Make internal endpoints bind to localhost by default | Done (fixed 2026-11-17; not yet run end to end) | `make setup` now writes `BIND_ADDR=127.0.0.1:9000`, and the Makefile loads `capstone_sfu/.env` and exports it, since the SFU doesn't read a `.env` itself. Checked with a dry run of the Makefile, not by starting the SFU. An existing `capstone_sfu/.env` keeps the old `SFU_ADDR` key until it's deleted and regenerated. The SFU's own default is still `0.0.0.0` when it's started outside `make`. | `Makefile` (`$(SFU_DIR)/.env`, `-include`) |
| Frontend `.env` name | Not planned; found while writing the README | Done | `make setup` wrote `PUBLIC_API_URL`, but the frontend reads `PUBLIC_API_BASE_URL`. The Makefile now writes the right name. | `Makefile` (`$(FE_DIR)/.env`); `capstone_fe/src/lib/api.ts` |
| TURN certificate after renewal | Not planned; found while writing the TURN guide | Done (not tested on the server) | `make tls` replaced the certificate files but restarted only the gateway. It now also restarts the TURN relay when TURN is on. | `Makefile` (`tls`) |
| SFU `.env` ignored | Add `.env` to the SFU `.gitignore` | Done | Covered by the monorepo's root `.gitignore`, which ignores `.env` in every folder. | `git check-ignore capstone_sfu/.env` |
| Real-device LAN test | Several phones streaming and watching, plus reconnect | Not recorded | No result is written down for this period. Same as the EC2 row: to be filled in. | — |
| Test coverage for `repository` and `middleware` | Raise and re-measure | Not started | Not prioritized behind the deployment work. | — |

### Other blockers (not tied to one deliverable)

- **Two working copies of the monorepo.** The bootcamp copy and the personal copy point at the same GitHub remote and the same commit, but each has different uncommitted changes. The Terraform state exists only in the personal copy. The uncommitted work should be committed from one place so the two don't drift.
- **Terraform state is a local file.** If it's lost, Terraform no longer knows about the instance, security group and Elastic IP, and they'd have to be removed by hand. It's git-ignored on purpose (it holds account details), so it needs a backup somewhere else, or a remote backend.
- **TURN relay capacity is unmeasured.** The relay range has 49 ports, one per relayed connection, and the SFU is given the TURN entry too. I haven't tested how many participants that supports.
- **Reports for weeks 12–16 and 18 are missing from the repo.** This report covers 2026-11-05 to 2026-11-17 in one go.

## 3. Decisions & Trade-offs

- **One EC2 instance instead of the free-tier split (Vercel + Oracle VM)**: the SFU needs inbound UDP, and serving the frontend from the same gateway keeps everything on one origin with no CORS setup. Trade-off: it costs money while it runs, and it's a single point of failure with in-memory state.
- **Terraform for the server, `make` for the app**: the firewall rules (eight inbound rules, two UDP ranges) are the part most easily got wrong by hand, and `terraform destroy` removes everything that costs money. Trade-off: local state has to be looked after, and the app install is still manual steps over SSH.
- **TURN as an optional compose profile with one shared credential**: a LAN doesn't need a relay, so it stays off there. One static username and password is simple. Trade-off: anyone who joins a room can read the credential and use the relay; per-join credentials are left for later.
- **coturn on host networking**: the relay needs the real public interface and a range of UDP ports. Trade-off: it isn't isolated on the compose network like the other services.
- **Blocklist enforced in nginx rather than in the SFU**: one `deny` list covers REST, SSE and the websocket, and the SFU stays media-only. Trade-off: enforcement depends on the gateway being in front, so it doesn't exist in local dev.
- **"Room is full" reuses `invalid_state`**: no contract change was needed for clients. Trade-off: the frontend can't show a specific message without matching on text.
- **Monorepo with a fresh history**: one clone builds the whole stack, which the compose file always assumed. Trade-off: the per-component commit history is not in the new repo.

## 4. AI Usage This Week

- Used Claude to compare the docs against the code and the Terraform state, and to update the documentation listed above, including the EC2 and TURN guides and the project README.
- Used Claude to run the backend, SFU and frontend test suites and to draft this report from the repository files and that test output. Rows marked "Not recorded" are the ones that couldn't be confirmed from files.
- _(Add how AI was used for the TURN, Terraform and deployment work itself.)_

## 5. Plan for Next Week

- Record the deployment test: site on the DuckDNS name with a trusted certificate, a phone on mobile data sharing and watching, and whether the relay is used.
- Commit the uncommitted work from one working copy, and back up the Terraform state.
- Add CI for the monorepo (run the Go, Rust and frontend suites on push).
- Start the stack with `make dev` after regenerating the `.env` files, to confirm the SFU binds to localhost and the frontend reaches the backend.
- Give "room is full" its own error code and show a clear message in the frontend.
- Run the Playwright suite, and raise `repository` and `middleware` test coverage.
- Destroy the EC2 deployment when the demo is over.

## 6. Consultation Notes (fill in after the session)

- Feedback received:
- Actions I'm taking based on it:
