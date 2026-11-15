use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::routing::{get, post};
use axum::{Json, Router};
use common::internal::{
    HealthResponse, PeerResponse, PublisherDto, PublishersResponse, StatsResponse,
    SubscriberQualityDto,
};

use super::error::ApiError;
use crate::transport::state::AppState;

pub fn router() -> Router<AppState> {
    Router::new()
        .route("/internal/health", get(health))
        .route("/internal/rooms/:room_id/publishers", get(publishers))
        .route("/internal/rooms/:room_id/stats", get(stats))
        .route("/internal/rooms/:room_id/close", post(close))
        .route("/internal/rooms/:room_id/peers/:peer_id", get(peer))
        .route(
            "/internal/rooms/:room_id/peers/:peer_id/disconnect",
            post(disconnect_peer),
        )
}

async fn health() -> Json<HealthResponse> {
    Json(HealthResponse {
        status: "ok".to_string(),
    })
}

async fn publishers(
    State(state): State<AppState>,
    Path(room_id): Path<String>,
) -> Json<PublishersResponse> {
    let snap = state.rooms.snapshot(&room_id).await;

    Json(PublishersResponse {
        room_id,
        publishers: snap
            .publisher_ids
            .into_iter()
            .map(|peer_id| PublisherDto { peer_id })
            .collect(),
    })
}

async fn stats(State(state): State<AppState>, Path(room_id): Path<String>) -> Json<StatsResponse> {
    let snap = state.rooms.snapshot(&room_id).await;
    let quality = snap.subscriber_quality;

    Json(StatsResponse {
        room_id,
        publisher_count: snap.publisher_ids.len(),
        subscriber_count: snap.subscriber_count,
        subscriber_quality: SubscriberQualityDto {
            good: quality.good,
            poor: quality.poor,
            bad: quality.bad,
        },
    })
}

/// Always 204, even for a room nobody joined, so Go can retry safely.
async fn close(State(state): State<AppState>, Path(room_id): Path<String>) -> StatusCode {
    state.rooms.close(&room_id).await;
    StatusCode::NO_CONTENT
}

/// 404 `peer_not_found` if the room or the peer doesn't exist.
async fn peer(
    State(state): State<AppState>,
    Path((room_id, peer_id)): Path<(String, String)>,
) -> Result<Json<PeerResponse>, ApiError> {
    let info = state.rooms.peer_info(&room_id, &peer_id).await?;
    Ok(Json(PeerResponse {
        room_id,
        peer_id: info.id,
        ip: info.ip,
    }))
}

/// 404 `peer_not_found` if the room or the peer doesn't exist.
async fn disconnect_peer(
    State(state): State<AppState>,
    Path((room_id, peer_id)): Path<(String, String)>,
) -> Result<StatusCode, ApiError> {
    state.rooms.disconnect_peer(&room_id, &peer_id).await?;
    Ok(StatusCode::NO_CONTENT)
}
