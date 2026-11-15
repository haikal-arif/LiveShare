//! Everything that speaks HTTP or WebSocket. Translates wire messages into
//! service calls and back; holds no room logic of its own.

pub mod http;
pub mod state;
pub mod ws;

use axum::Router;

use state::AppState;

/// The single router serving both the public `/ws` signaling endpoint and
/// the internal-only `/internal/*` REST API.
pub fn router(state: AppState) -> Router {
    ws::router().merge(http::router()).with_state(state)
}
