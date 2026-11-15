pub mod config;
pub mod domain;
pub mod infra;
pub mod service;
pub mod transport;

use std::sync::Arc;

use axum::Router;
use config::Config;
use infra::media::MediaContext;
use service::registry::RoomService;
use transport::state::AppState;

/// Wires config, the media adapter, the room service and the transport
/// into the one router served by the binary. Also returns the room service
/// so the caller can close rooms on shutdown.
pub fn build(config: Config) -> (Router, Arc<RoomService>) {
    let media = Arc::new(
        MediaContext::new(&config.ice_servers, config.udp_ports).with_public_ips(config.public_ips),
    );
    let rooms = Arc::new(RoomService::new(
        config.ice_servers,
        media,
        config.max_participants,
    ));
    let router = transport::router(AppState {
        rooms: rooms.clone(),
    });
    (router, rooms)
}

/// `build` without the room service. Split out of `main` so integration
/// tests can bind it to an ephemeral port.
pub fn app(config: Config) -> Router {
    build(config).0
}
