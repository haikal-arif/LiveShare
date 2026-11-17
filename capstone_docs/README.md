# Capstone Documentation

Documentation for FlashLive ("Local POV Live Stream"). The project README is one level up, in [`../README.md`](../README.md).

---

## 1. Product Overview

- [product-overview.md](./product-overview.md): problem, scope and feature status, user stories, requirements, limitations.

## 2. Application Wireframe

- [Figma wireframe](https://www.figma.com/design/98MtNCX0c3F1Ucf0aPPFBn/Untitled?node-id=3-65&t=1xzyDQbJRnp9ipzI-1)
- [dataflow.html](./dataflow.html): data flow diagram (open in a browser).

## 3. Weekly Report

| Report | Period |
| --- | --- |
| [WEEKLYREPORT_2026_11_17.md](./WEEKLYREPORT_2026_11_17.md) | 2026-11-05 – 2026-11-17 |
| [WEEKLYREPORT_2026_11_04.md](./WEEKLYREPORT_2026_11_04.md) | 2026-10-23 – 2026-11-04 |
| [WEEKLYREPORT_2026_10_22.md](./WEEKLYREPORT_2026_10_22.md) | 2026-09-29 – 2026-10-22 |
| [WEEKLYREPORT_2026_09_28.md](./WEEKLYREPORT_2026_09_28.md) | 2026-09-15 – 2026-09-28 |
| [WEEKLYREPORT_2026_09_15.md](./WEEKLYREPORT_2026_09_15.md) | 2026-08-31 – 2026-09-15 |
| [WEEKLYREPORT_2026_08_31.md](./WEEKLYREPORT_2026_08_31.md) | 2026-08-25 – 2026-08-31 |
| [WEEKLYREPORT_2026_08_26.md](./WEEKLYREPORT_2026_08_26.md) | to 2026-08-26 |
| [WEEKLYREPORT_2026_08_04.md](./WEEKLYREPORT_2026_08_04.md) | 2026-07-24 – 2026-08-04 |
| [WEEKLYREPORT_2026_07_24.md](./WEEKLYREPORT_2026_07_24.md) | 2026-07-17 – 2026-07-24 |

## 4. Backend Documentation

- [technical-architecture.md](./technical-architecture.md): components, boundaries, lifecycle, design decisions. Covers the Go webserver, the Rust SFU, the gateway and the TURN relay.
- [API-Contract.md](./API-Contract.md): every endpoint and signaling message. A copy of `../contracts.md`, which is the source of truth.

## 5. Frontend Documentation

- [`../capstone_fe/README.md`](../capstone_fe/README.md): developing, building and testing the SvelteKit app.
- [desktop-display-plan.md](./desktop-display-plan.md): plan for desktop sharing as a separate stream (not built).

## 6. Integration Documentation

- [ec2-deployment.md](./ec2-deployment.md): deploying on AWS EC2, with Terraform for the server and `make` for the stack.
- [turn-relay.md](./turn-relay.md): the TURN relay (coturn): what it's for, how to turn it on, ports, checks.
- [free-deployment-plan.md](./free-deployment-plan.md): the earlier free-tier plan (Vercel + Oracle VM). Superseded by the EC2 deployment.
