//! Ports the room logic depends on. The media engine (webrtc-rs today)
//! implements them in `infra::media`, so nothing above this layer touches
//! WebRTC types and rooms can be tested against fakes.

use std::any::Any;
use std::sync::Arc;

use async_trait::async_trait;
use common::signaling::SimulcastLayer;
use tokio::sync::watch;

#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
pub enum MediaError {
    /// The remote SDP (offer or answer) was rejected. The message is the
    /// engine's own reason.
    #[error("{0}")]
    InvalidSdp(String),
    /// The remote ICE candidate wasn't a JSON-encoded candidate init.
    #[error("invalid ice candidate")]
    InvalidCandidate,
    #[error("{0}")]
    Engine(String),
}

/// Something a media session reports back to the room that owns it.
pub enum MediaEvent {
    /// A candidate the SFU's side gathered, JSON-encoded, to relay to the peer.
    LocalIceCandidate { candidate: String },
    /// A publisher session started receiving a track.
    TrackPublished { track: Arc<dyn MediaTrack> },
    /// The session's connection to the peer is up.
    Connected,
}

/// Receives `MediaEvent`s. Engines call it inline from their driver loop, so
/// `emit` must never block or wait on the room.
pub trait MediaEventSink: Send + Sync {
    fn emit(&self, event: MediaEvent);
}

/// One track a publisher sends into the SFU. Opaque to the room: only the
/// gateway that produced it knows how to attach it to a subscriber session.
#[async_trait]
pub trait MediaTrack: Send + Sync {
    fn mime_type(&self) -> String;
    fn is_video(&self) -> bool;
    /// The layer a subscriber targeting `target` is actually forwarded.
    fn effective_layer(&self, target: SimulcastLayer) -> Option<SimulcastLayer>;
    /// Asks the publisher for a fresh keyframe on every layer.
    async fn request_keyframe(&self);
    /// Lets the engine recover its concrete track type when attaching.
    fn into_any(self: Arc<Self>) -> Arc<dyn Any + Send + Sync>;
}

/// One negotiated connection between the SFU and a peer.
#[async_trait]
pub trait MediaSession: Send + Sync {
    /// Publisher side: applies the peer's offer and returns the SFU's answer.
    async fn accept_offer(&self, sdp: String) -> Result<String, MediaError>;
    /// Subscriber side: returns a fresh SFU offer.
    async fn create_offer(&self) -> Result<String, MediaError>;
    /// Subscriber side: applies the peer's answer to the last offer.
    async fn apply_answer(&self, sdp: String) -> Result<(), MediaError>;
    /// Adds a JSON-encoded remote ICE candidate. An empty candidate is an
    /// end-of-candidates marker and does nothing.
    async fn add_ice_candidate(&self, candidate_json: &str) -> Result<(), MediaError>;
    /// Starts forwarding `track` to this session, following `layer`. The
    /// caller must renegotiate afterwards.
    async fn attach_track(
        &self,
        track: &Arc<dyn MediaTrack>,
        stream_id: &str,
        layer: watch::Receiver<SimulcastLayer>,
    ) -> Result<(), MediaError>;
    /// Stops forwarding and closes the connection.
    async fn close(&self);
}

#[async_trait]
pub trait MediaGateway: Send + Sync {
    /// A receive-only session for a publisher.
    async fn new_publisher_session(
        &self,
        events: Arc<dyn MediaEventSink>,
    ) -> Result<Arc<dyn MediaSession>, MediaError>;
    /// A send-only session carrying one publisher's tracks to a subscriber.
    async fn new_subscriber_session(
        &self,
        events: Arc<dyn MediaEventSink>,
    ) -> Result<Arc<dyn MediaSession>, MediaError>;
}
