//! The room actor. `actor` owns the run loop and peer lifecycle,
//! `negotiation` the SDP/ICE/track flow, and `quality` the per-pairing
//! connection quality and layer selection; `state` holds the data they share.

mod actor;
mod negotiation;
mod quality;
mod state;

pub use actor::Room;

use std::sync::Arc;

use common::signaling::{PeerId, SimulcastLayer};
use tokio::sync::oneshot;

use crate::domain::error::DomainError;
use crate::domain::peer::{ConnectionHandle, PeerInfo, PeerRole};
use crate::domain::ports::MediaTrack;
use crate::domain::quality::StatsReport;

/// Commands sent into a `Room` actor from any connection currently joined
/// to it (or, for `Snapshot`, from the internal REST handlers).
pub enum RoomCommand {
    /// Replies with the new peer's id, or `Err` when the room is full.
    Join {
        role: PeerRole,
        handle: ConnectionHandle,
        reply: oneshot::Sender<Result<PeerId, DomainError>>,
    },
    SdpOffer {
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        sdp: String,
    },
    SdpAnswer {
        peer_id: PeerId,
        target_peer: PeerId,
        sdp: String,
    },
    IceCandidate {
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        candidate: String,
    },
    RequestTrack {
        peer_id: PeerId,
        target_peer: PeerId,
    },
    ConnectionStats {
        peer_id: PeerId,
        target_peer: PeerId,
        report: StatsReport,
    },
    SetPreferredLayer {
        peer_id: PeerId,
        target_peer: PeerId,
        layer: SimulcastLayer,
    },
    Disconnect {
        peer_id: PeerId,
    },
    /// Looks up a connected peer.
    PeerInfo {
        peer_id: PeerId,
        reply: oneshot::Sender<Option<PeerInfo>>,
    },
    /// Removes a peer as if it had left, then closes its connection.
    /// Replies `false` if no such peer is connected.
    Kick {
        peer_id: PeerId,
        reply: oneshot::Sender<bool>,
    },
    Snapshot {
        reply: oneshot::Sender<RoomSnapshot>,
    },
    /// The admin closed the room: tell everyone, disconnect them, stop.
    Close {
        reply: oneshot::Sender<()>,
    },
    /// Internal: a candidate one of the SFU's own `PeerConnection`s gathered,
    /// to relay to `peer_id`. `origin_publisher` names the subscriber pairing.
    LocalIceCandidate {
        peer_id: PeerId,
        origin_publisher: Option<PeerId>,
        candidate: String,
    },
    /// Internal: a publisher's `PeerConnection` started receiving a track.
    TrackPublished {
        publisher_id: PeerId,
        track: Arc<dyn MediaTrack>,
    },
    /// Internal: a subscriber pairing just connected and needs a keyframe.
    RequestKeyframe {
        publisher_id: PeerId,
    },
}

#[derive(Debug, Default, Clone)]
pub struct RoomSnapshot {
    pub publisher_ids: Vec<PeerId>,
    pub subscriber_count: usize,
    /// Subscriber↔publisher pairings by current quality, so one subscriber
    /// watching two publishers counts twice.
    pub subscriber_quality: QualityCounts,
}

#[derive(Debug, Default, Clone, Copy)]
pub struct QualityCounts {
    pub good: usize,
    pub poor: usize,
    pub bad: usize,
}

#[cfg(test)]
mod tests;
