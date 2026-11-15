//! Internal-only REST API (`contracts.md` section 2) — only reachable from
//! the Go webserver, never exposed past the reverse proxy in a real
//! deployment. Served on the same axum router as `/ws`; the "internal-only"
//! property is a network-topology concern, not a code-level one.

mod error;
mod internal;

pub use internal::router;
