use std::sync::Arc;

use crate::service::registry::RoomService;

/// Shared by every handler.
#[derive(Clone)]
pub struct AppState {
    pub rooms: Arc<RoomService>,
}
