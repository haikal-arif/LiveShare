use common::signaling::{PeerId, ServerMessage};
use tokio::sync::mpsc;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum PeerRole {
    Publisher,
    Subscriber,
}

/// What a room pushes to a peer's connection.
#[derive(Debug, Clone)]
pub enum Outbound {
    Message(ServerMessage),
    /// Close the connection once everything queued before it is delivered.
    Close,
}

impl From<ServerMessage> for Outbound {
    fn from(msg: ServerMessage) -> Self {
        Outbound::Message(msg)
    }
}

/// Handle a `Room` actor holds for a connected peer: how to push
/// asynchronous messages out to that peer's own connection writer task.
#[derive(Clone)]
pub struct ConnectionHandle {
    pub outbound: mpsc::Sender<Outbound>,
    /// Where the peer opened its connection from, if known.
    pub remote_ip: Option<String>,
}

/// What the SFU knows about a connected peer.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PeerInfo {
    pub id: PeerId,
    pub role: PeerRole,
    pub ip: Option<String>,
}
