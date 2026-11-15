//! Wire types for the SFU's internal REST API (`contracts.md` section 2),
//! called by the Go webserver.

use serde::{Deserialize, Serialize};

use crate::signaling::PeerId;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HealthResponse {
    pub status: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PublisherDto {
    pub peer_id: PeerId,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PublishersResponse {
    pub room_id: String,
    pub publishers: Vec<PublisherDto>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SubscriberQualityDto {
    pub good: usize,
    pub poor: usize,
    pub bad: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StatsResponse {
    pub room_id: String,
    pub publisher_count: usize,
    pub subscriber_count: usize,
    pub subscriber_quality: SubscriberQualityDto,
}

/// `GET /internal/rooms/:room_id/peers/:peer_id`. `ip` is where the peer
/// opened its websocket from; `null` only if the SFU couldn't tell, which
/// doesn't happen behind the reverse proxy.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PeerResponse {
    pub room_id: String,
    pub peer_id: PeerId,
    pub ip: Option<String>,
}

/// Body of every non-2xx internal REST response, the same shape the Go
/// webserver uses.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorBody {
    pub error: ErrorDetail,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorDetail {
    pub code: String,
    pub message: String,
}
