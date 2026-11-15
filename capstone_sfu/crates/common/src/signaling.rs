use serde::{Deserialize, Serialize};

/// Identifies an individual connected peer (publisher or subscriber),
/// assigned by the SFU at join time.
pub type PeerId = String;
/// Identifies a room/session that a publisher/subscriber joins.
pub type RoomId = String;

/// Mirrors the WebRTC `RTCIceServer` shape handed to clients.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct IceServer {
    pub urls: Vec<String>,
    /// Only for TURN: the long-term username and password.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub username: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub credential: Option<String>,
}

/// Messages sent FROM a client (browser or mock app) TO the SFU over the
/// public signaling WebSocket.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ClientMessage {
    /// Announce intent to publish a stream into a room. Exactly one peer
    /// connection follows: this connection, with the SFU itself.
    JoinAsPublisher { room: RoomId },
    /// Announce intent to watch stream(s) in a room. May end up with
    /// multiple concurrent peer connections, one per watched publisher.
    JoinAsSubscriber { room: RoomId },
    /// On the publisher/SFU pairing (`target_peer: null`), the publisher
    /// is always the offerer, including for renegotiation. Never sent by
    /// a subscriber.
    SdpOffer {
        #[serde(default)]
        target_peer: Option<PeerId>,
        sdp: String,
    },
    /// Only ever a subscriber answering the SFU's offer for a specific
    /// publisher's stream.
    SdpAnswer { target_peer: PeerId, sdp: String },
    /// `target_peer` is `None` on the publisher pairing, `Some(publisher)`
    /// on a subscriber pairing.
    IceCandidate {
        #[serde(default)]
        target_peer: Option<PeerId>,
        candidate: String,
    },
    /// Subscriber asking to start watching a given publisher; triggers the
    /// SFU to create an offer for that pair.
    RequestTrack { target_peer: PeerId },
    /// Subscriber's periodic `getStats()` report for one watched publisher;
    /// the SFU uses it to pick that pairing's simulcast layer. Only the
    /// loss figure is required, since browsers don't all expose the rest.
    ConnectionStats {
        target_peer: PeerId,
        #[serde(default)]
        available_incoming_bitrate_bps: Option<u64>,
        packet_loss_pct: f64,
        #[serde(default)]
        round_trip_time_ms: Option<f64>,
        #[serde(default)]
        jitter_ms: Option<f64>,
    },
    /// Subscriber picking the simulcast layer it wants from one publisher.
    /// Takes effect at once, even above a layer the SFU downgraded it
    /// from; the SFU still steps down from it on sustained loss.
    SetPreferredLayer {
        target_peer: PeerId,
        layer: SimulcastLayer,
    },
    /// Clean, explicit disconnect.
    Leave {},
}

/// How well a subscriber↔publisher pairing's connection is keeping up.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ConnectionQuality {
    Good,
    Poor,
    Bad,
}

/// A publisher simulcast layer, sent with rids `h` / `m` / `l`. Ordered
/// lowest to highest quality.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SimulcastLayer {
    Low,
    Mid,
    High,
}

impl SimulcastLayer {
    pub fn from_rid(rid: &str) -> Option<Self> {
        match rid {
            "h" => Some(Self::High),
            "m" => Some(Self::Mid),
            "l" => Some(Self::Low),
            _ => None,
        }
    }

    pub fn rid(self) -> &'static str {
        match self {
            Self::High => "h",
            Self::Mid => "m",
            Self::Low => "l",
        }
    }
}

/// Machine-readable reason accompanying every `error` message. Clients
/// should treat codes they don't know as a generic error.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ErrorCode {
    InvalidMessage,
    NotJoined,
    AlreadyJoined,
    PeerNotFound,
    WrongRole,
    InvalidSdp,
    InvalidState,
    Internal,
}

/// Messages sent FROM the SFU TO a client over the public signaling
/// WebSocket.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ServerMessage {
    /// Sent right after join; `publishers` is the room snapshot at join
    /// time (excluding the joining peer itself, if it's a publisher).
    Joined {
        peer_id: PeerId,
        ice_servers: Vec<IceServer>,
        publishers: Vec<PeerId>,
    },
    PublisherJoined {
        peer_id: PeerId,
    },
    PublisherLeft {
        peer_id: PeerId,
    },
    /// `from_peer: None` = "this is the SFU itself" (the publisher's own
    /// counterparty). `Some(publisher)` when offering a publisher's
    /// stream to a subscriber.
    SdpOffer {
        from_peer: Option<PeerId>,
        sdp: String,
    },
    SdpAnswer {
        from_peer: Option<PeerId>,
        sdp: String,
    },
    IceCandidate {
        from_peer: Option<PeerId>,
        candidate: String,
    },
    /// Sent to a subscriber when a pairing's `quality` or forwarded `layer`
    /// changes, and in reply to every `set_preferred_layer`.
    ConnectionQuality {
        from_peer: PeerId,
        quality: ConnectionQuality,
        layer: SimulcastLayer,
    },
    /// The admin closed the room. The SFU disconnects everyone right after.
    RoomClosed,
    Error {
        code: ErrorCode,
        reason: String,
    },
}
