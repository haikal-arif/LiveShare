//! The public signaling WebSocket (`API-Contract.md` section 3): one
//! connection per client, role fixed for its lifetime by whichever
//! `join_as_*` message arrives first.

mod handler;
mod mapping;
mod session;

pub use handler::router;
