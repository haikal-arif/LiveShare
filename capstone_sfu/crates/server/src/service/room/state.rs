use std::collections::HashMap;
use std::sync::Arc;

use common::signaling::{ConnectionQuality, PeerId, SimulcastLayer};
use tokio::sync::watch;

use crate::domain::peer::ConnectionHandle;
use crate::domain::ports::{MediaSession, MediaTrack};
use crate::domain::quality::QualityTracker;

pub(super) struct PublisherEntry {
    pub(super) handle: ConnectionHandle,
    /// Created lazily on the publisher's first `sdp_offer`.
    pub(super) pc: Option<Arc<dyn MediaSession>>,
    /// Append-only, in arrival order.
    pub(super) tracks: Vec<Arc<dyn MediaTrack>>,
}

/// The SFU-offers/subscriber-answers `PeerConnection` carrying one
/// publisher's tracks to one subscriber.
pub(super) struct Pairing {
    pub(super) pc: Arc<dyn MediaSession>,
    /// How many of the publisher's `tracks` have been added to `pc`.
    pub(super) attached: usize,
    /// An offer is out and its answer hasn't arrived yet.
    pub(super) awaiting_answer: bool,
    /// Tracks changed while `awaiting_answer`; offer again once it lands.
    pub(super) renegotiate: bool,
    pub(super) quality: QualityTracker,
    /// The simulcast layer this pairing's relays should forward; always
    /// `quality.layer()`.
    pub(super) target_layer: watch::Sender<SimulcastLayer>,
    /// Last `connection_quality` sent. Reports only send one when this
    /// changes; a `set_preferred_layer` always gets one back.
    pub(super) reported: (ConnectionQuality, SimulcastLayer),
}

impl Pairing {
    pub(super) fn close(self) {
        tokio::spawn(async move { self.pc.close().await });
    }
}

pub(super) struct SubscriberEntry {
    pub(super) handle: ConnectionHandle,
    pub(super) pairings: HashMap<PeerId, Pairing>,
}
