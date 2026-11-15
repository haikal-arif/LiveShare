# Weekly Report — Week [17]

| Field        | Value                                                                                        |
| ------------ | -------------------------------------------------------------------------------------------- |
| Project name | _Local live video sharing_                                                                    |
| Trainee      | _M Haikal A_                                                                                 |
| Week / dates | 17 / (2026-10-23 – 2026-11-04)                                                                 |
| Phase        | Implementation                                                                                |
| Checkpoint   | _Week 11 work is now committed and pushed in all four repos; backend CI restored; stack config split into its own repo; frontend unit tests and type check green against the refactored backend_ |

## 1. This Week's Goal

Close out the Week 11 open items: commit and push everything that was still in the working tree, get every repo's CI to a working state, check the frontend against the refactored backend, and move the stack config out of the parent folder into its own repo.

## 2. Progress & Blockers

One row per deliverable. Required before consultation (Section 6 of the policy) — whenever status isn't Done, don't just name the problem in the blocker cell, name what you tried, and flag whether you need help on it.

| Deliverable            | Planned this week          | Actual status     | Blocker / What I tried                                                                                  | Evidence                    |
| ---------------------- | --------------------------- | ----------------- | -------------------------------------------------------------------------------------------------------- | --------------------------- |
| Commit and push Week 11 work | Push all four service repos, which had uncommitted changes and no upstream | Done | `capstone_docs` (`technical-arch`), `capstone_fe` and `capstone_sfu` (`feature/stage-layout`) pushed. `capstone_be` had no `origin` configured in this copy and its remote `main` shared no history with local `main`. I pushed the work to `feature/stage-layout` first, then rebuilt `main` on top of the remote's initial commits with one refactor commit, so the remote history is preserved. Three pushes hit a transient GitLab 500 (pre-receive hook); all three succeeded on retry. | `capstone_docs` `60ae07a`; `capstone_fe` `71c2015`; `capstone_sfu` `b82a66a`; `capstone_be` `1ce4c78` on `main` |
| Backend CI | Every submodule's `.gitlab-ci.yml` runs and passes | Done | `capstone_be` had lost its `.gitlab-ci.yml` in the refactor commit. Restored the SAST config from the remote and added a `go-checks` job (`go vet ./...`, `go test ./...`, image `golang:1.26`). Locally vet and tests pass; I can't see the GitLab pipeline result from here, and I haven't confirmed the `golang:1.26` image tag exists. `capstone_sfu`'s CI (fmt, clippy `-D warnings`, `cargo test --workspace`, forbidden-import grep) passes locally on every step. `capstone_fe` and `capstone_docs` have no CI file. | `capstone_be` `d7a1ae2`; `capstone_sfu/.gitlab-ci.yml` |
| Stack config repo | Move compose, Makefile, gateway config and helper scripts into their own repo | Done | New private project `capstone-stack` under the capstone group. Contains `docker-compose.yml`, `Makefile`, `start-dev.sh`, `firewall.sh` and `gateway/` (Dockerfile, `gen-cert.sh`, nginx template, snippets). TLS key and `.env` are excluded and ignored. Open issue: the compose file builds from `./capstone_be|fe|sfu`, so a fresh clone won't build without sibling checkouts. | `capstone-stack` repo, commit `b47260b` |
| Frontend update (`capstone_fe`) | Re-run unit and type checks against the refactored backend, then commit | Done (Playwright not re-run) | The WIP from last week is committed as `71c2015`. `vitest --run`: 15 files, 66 tests, all pass. `svelte-check`: 0 errors, 0 warnings across 418 files. Playwright e2e was not run this week. | `capstone_fe` `71c2015`; test output 2026-11-04 |
| Simulcast in the SFU | Commit the simulcast engine and session work left in the SFU working tree | Done | Committed with the SFU's `Dockerfile`. `cargo fmt --check`, `cargo clippy --workspace -D warnings` and `cargo test --workspace` all pass locally. | `capstone_sfu` `b82a66a` |
| Stale `context.md` | Rewrite before the code defense | Partly done | Updated on 2026-10-22 and now describes the current router, SSE and liveness poller. It still lists the `SFU_ADDR` bind issue as a known gap (see below). I haven't re-checked every section against the code. | `context.md` |
| SFU bind address (`SFU_ADDR` vs `BIND_ADDR`) | Make internal endpoints bind to localhost by default | Not started | The Makefile still writes `SFU_ADDR=127.0.0.1:9000` into `capstone_sfu/.env`, but `config.rs:27` reads only `BIND_ADDR` (default `0.0.0.0:9000`). Fix is to read `SFU_ADDR` or write `BIND_ADDR`; I haven't chosen which. | `capstone_sfu/crates/server/src/config.rs:27`; `Makefile:55` |
| SFU `.env` ignored | Add `.env` to the SFU `.gitignore` | Not started | `git check-ignore` reports `.env` as not ignored. The file is still untracked and I've kept it out of commits. | `capstone_sfu/.gitignore` |
| Real-device LAN test | Several phones streaming and watching, plus reconnect | Not started | Carried over again. No new blocker; it was not prioritized behind the repo cleanup. | — |

### Other blockers (not tied to one deliverable)

- **Repo history mismatch in `capstone_be`.** `main` now sits on top of the remote's `46468d9`…`312c42d` history. The 12 development commits are kept on a local branch, and the remote branch `feature/stage-layout` still points at them. Someone needs to choose which line is canonical, and the parent `capstone` submodule pointer for `capstone-be` (`312c42d`) is now behind.
- **Refactor removed files that existed on the remote.** `.gitlab-ci.yml` (now restored), `README.md`, `cmd/server/main.go`, `internal/router/`, `internal/sfuclient/` and `internal/config/` are gone from `main`. I haven't confirmed whether removing the README and the code files is intended; the CI file was unintentional and is fixed.
- **Missing GitLab repos for `gateway/` and `loadtest/`.** Neither is a git repo. `loadtest/` is excluded from `capstone-stack` on purpose.
- **Parent repo not set up.** `capstone_v2/` is not a git repo, and the submodule layout in `../capstone` is not mirrored here.
- **Reports for weeks 12–16 are missing from the repo.** This report covers 2026-10-23 to 2026-11-04 in one go.

## 3. Decisions & Trade-offs

- **Rebuilt `capstone-be`'s `main` on the remote history instead of force-pushing**: the remote's four commits (including the CORS change and the SAST CI file) are kept as ancestors, and the development tree lands as one commit. Trade-off: the 12 development commits are not on `main`, so `git log main` shows one refactor commit. Accepted to avoid rewriting shared history.
- **Pushed to `feature/stage-layout` for the first attempt, not `main`**: this kept the first push non-destructive while the history question was open. Trade-off: the branch name no longer matches the content on `main`.
- **Stack config in its own repo, certs and `.env` out**: keeps the repo shareable and the secrets local. Trade-off: the repo can't build standalone; it depends on sibling service checkouts.
- **Restored the backend SAST config and added a Go test job rather than copying the SFU's pipeline**: the backend has no Rust checks to run and already had a SAST file on the remote. Trade-off: the `golang:1.26` image tag is unverified.

## 4. AI Usage This Week

- Used Claude to inspect the four repos, compare their remote and local histories, and draft the refactor commit that keeps the remote history. The tree was checked to be identical to local `main` before pushing.
- Used Claude to restore the backend CI config and add the Go job, and to run the SFU's CI commands and the frontend's unit and type checks locally.
- Used Claude to build the `capstone-stack` repo and to draft this report from commit history and the test output above.

## 5. Plan for Next Week

- Pick the canonical line for `capstone-be` (keep the remote history with the refactor commit, or merge the 12 development commits in), then update the parent submodule pointer.
- Fix the SFU bind address (`SFU_ADDR` vs `BIND_ADDR`) so internal endpoints bind to localhost by default, and add `.env` to the SFU's `.gitignore`.
- Run the Playwright suite against the refactored backend and check the GitLab pipelines for all three repos with CI.
- Test on real devices on the local network (multiple phones streaming and watching, reconnection, admin blocking a live peer).
- Raise `repository` and `middleware` test coverage, and re-measure it.

## 6. Consultation Notes (fill in after the session)

- Feedback received:
- Actions I'm taking based on it:
