use std::collections::HashMap;
use std::sync::{Arc, Mutex};

use common::signaling::{IceServer, RoomId};
use tokio::sync::mpsc;

use crate::domain::error::DomainError;
use crate::domain::peer::PeerInfo;
use crate::domain::ports::MediaGateway;
use crate::service::room::{Room, RoomCommand, RoomSnapshot};

/// Looks up (or, on join, lazily spawns) the actor task for a room.
///
/// Read-only lookups (used by the internal REST API) never create a room
/// — probing an unused room id by hitting the REST API should not leak a
/// task, since a room is only meaningful once someone actually joins it.
pub struct RoomService {
    ice_servers: Vec<IceServer>,
    media: Arc<dyn MediaGateway>,
    max_participants: Option<usize>,
    rooms: Mutex<HashMap<RoomId, mpsc::Sender<RoomCommand>>>,
}

impl RoomService {
    pub fn new(
        ice_servers: Vec<IceServer>,
        media: Arc<dyn MediaGateway>,
        max_participants: Option<usize>,
    ) -> Self {
        Self {
            ice_servers,
            media,
            max_participants,
            rooms: Mutex::new(HashMap::new()),
        }
    }

    /// Read-only lookup; does not create the room if absent.
    pub fn get(&self, room_id: &str) -> Option<mpsc::Sender<RoomCommand>> {
        self.rooms.lock().unwrap().get(room_id).cloned()
    }

    /// What the SFU knows about a connected peer. `PeerNotFound` if the room
    /// or the peer doesn't exist.
    pub async fn peer_info(&self, room_id: &str, peer_id: &str) -> Result<PeerInfo, DomainError> {
        let not_found = || peer_not_found(room_id, peer_id);
        let room = self.get(room_id).ok_or_else(not_found)?;
        let (reply, rx) = tokio::sync::oneshot::channel();
        let peer_id_owned = peer_id.to_string();
        let command = RoomCommand::PeerInfo {
            peer_id: peer_id_owned,
            reply,
        };
        room.send(command).await.map_err(|_| not_found())?;
        rx.await.ok().flatten().ok_or_else(not_found)
    }

    /// Removes a peer as if it had left and closes its connection.
    /// `PeerNotFound` if the room or the peer doesn't exist.
    pub async fn disconnect_peer(&self, room_id: &str, peer_id: &str) -> Result<(), DomainError> {
        let not_found = || peer_not_found(room_id, peer_id);
        let room = self.get(room_id).ok_or_else(not_found)?;
        let (reply, rx) = tokio::sync::oneshot::channel();
        let command = RoomCommand::Kick {
            peer_id: peer_id.to_string(),
            reply,
        };
        room.send(command).await.map_err(|_| not_found())?;
        match rx.await {
            Ok(true) => Ok(()),
            _ => Err(not_found()),
        }
    }

    /// A point-in-time view of a room. A room nobody has joined is empty;
    /// this never creates one.
    pub async fn snapshot(&self, room_id: &str) -> RoomSnapshot {
        let Some(room) = self.get(room_id) else {
            return RoomSnapshot::default();
        };
        let (reply, rx) = tokio::sync::oneshot::channel();
        if room.send(RoomCommand::Snapshot { reply }).await.is_err() {
            return RoomSnapshot::default();
        }
        rx.await.unwrap_or_default()
    }

    /// Closes a room: its actor tells every peer `room_closed`, disconnects
    /// them and stops. A room id nobody has joined is a no-op. The next
    /// join with the same id starts a fresh room.
    pub async fn close(&self, room_id: &str) {
        let Some(room) = self.rooms.lock().unwrap().remove(room_id) else {
            return;
        };
        let (reply, done) = tokio::sync::oneshot::channel();
        if room.send(RoomCommand::Close { reply }).await.is_ok() {
            let _ = done.await;
        }
    }

    /// Closes every room, e.g. on shutdown, so each peer hears `room_closed`
    /// and its connection is closed rather than dropped.
    pub async fn close_all(&self) {
        let room_ids: Vec<RoomId> = self.rooms.lock().unwrap().keys().cloned().collect();
        for room_id in room_ids {
            self.close(&room_id).await;
        }
    }

    /// Lookup used by a joining connection; spawns a new `Room` actor the
    /// first time a given room id is seen.
    pub fn get_or_create(&self, room_id: &RoomId) -> mpsc::Sender<RoomCommand> {
        let mut rooms = self.rooms.lock().unwrap();
        rooms
            .entry(room_id.clone())
            .or_insert_with(|| {
                Room::spawn(
                    self.ice_servers.clone(),
                    self.media.clone(),
                    self.max_participants,
                )
            })
            .clone()
    }
}

fn peer_not_found(room_id: &str, peer_id: &str) -> DomainError {
    DomainError::PeerNotFound(format!("peer {peer_id} is not connected to room {room_id}"))
}
