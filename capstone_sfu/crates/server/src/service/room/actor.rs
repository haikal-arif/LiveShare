use std::collections::HashMap;
use std::sync::Arc;

use async_trait::async_trait;
use common::signaling::{ConnectionQuality, IceServer, PeerId, ServerMessage, SimulcastLayer};
use tokio::sync::{mpsc, oneshot};

use super::state::{PublisherEntry, SubscriberEntry};
use super::{QualityCounts, RoomCommand, RoomSnapshot};
use crate::domain::error::DomainError;
use crate::domain::peer::{ConnectionHandle, Outbound, PeerInfo, PeerRole};
use crate::domain::ports::{MediaEvent, MediaEventSink, MediaGateway, MediaTrack};
use crate::domain::quality::StatsReport;

/// Owns all mutable state for a single room. Spawned as its own task by
/// `RoomService::get_or_create`; every mutation goes through the command
/// channel, so there's no cross-task locking to get wrong.
pub struct Room {
    pub(super) ice_servers: Vec<IceServer>,
    pub(super) media: Arc<dyn MediaGateway>,
    /// Weak so the room's own `PeerConnection` callbacks don't keep it alive.
    pub(super) self_tx: mpsc::WeakSender<RoomCommand>,
    pub(super) publishers: HashMap<PeerId, PublisherEntry>,
    pub(super) subscribers: HashMap<PeerId, SubscriberEntry>,
    /// `None` = no limit on peers in the room.
    pub(super) max_participants: Option<usize>,
}

impl Room {
    pub fn spawn(
        ice_servers: Vec<IceServer>,
        media: Arc<dyn MediaGateway>,
        max_participants: Option<usize>,
    ) -> mpsc::Sender<RoomCommand> {
        let (tx, rx) = mpsc::channel(256);
        let room = Room {
            ice_servers,
            media,
            self_tx: tx.downgrade(),
            publishers: HashMap::new(),
            subscribers: HashMap::new(),
            max_participants,
        };
        tokio::spawn(run(room, rx));
        tx
    }

    pub(super) fn handle_join(
        &mut self,
        role: PeerRole,
        handle: ConnectionHandle,
        reply: oneshot::Sender<Result<PeerId, DomainError>>,
    ) {
        if let Some(max) = self.max_participants {
            let current = self.publishers.len() + self.subscribers.len();
            if current >= max {
                let _ = reply.send(Err(DomainError::InvalidState(format!(
                    "room is full ({current}/{max} participants)"
                ))));
                return;
            }
        }

        let peer_id = crate::domain::ids::generate(role);
        let publishers_snapshot: Vec<PeerId> = self.publishers.keys().cloned().collect();

        send_to(
            &handle,
            ServerMessage::Joined {
                peer_id: peer_id.clone(),
                ice_servers: self.ice_servers.clone(),
                publishers: publishers_snapshot,
            },
        );

        match role {
            PeerRole::Publisher => {
                self.publishers.insert(
                    peer_id.clone(),
                    PublisherEntry {
                        handle,
                        pc: None,
                        tracks: Vec::new(),
                    },
                );
                self.broadcast_to_subscribers(ServerMessage::PublisherJoined {
                    peer_id: peer_id.clone(),
                });
            }
            PeerRole::Subscriber => {
                self.subscribers.insert(
                    peer_id.clone(),
                    SubscriberEntry {
                        handle,
                        pairings: HashMap::new(),
                    },
                );
            }
        }

        let _ = reply.send(Ok(peer_id));
    }

    pub(super) fn handle_disconnect(&mut self, peer_id: PeerId) {
        if let Some(entry) = self.publishers.remove(&peer_id) {
            if let Some(pc) = entry.pc {
                tokio::spawn(async move { pc.close().await });
            }
            for sub in self.subscribers.values_mut() {
                if let Some(pairing) = sub.pairings.remove(&peer_id) {
                    pairing.close();
                }
            }
            self.broadcast_to_subscribers(ServerMessage::PublisherLeft { peer_id });
            return;
        }
        if let Some(entry) = self.subscribers.remove(&peer_id) {
            for pairing in entry.pairings.into_values() {
                pairing.close();
            }
        }
    }

    /// Sends `room_closed` to every peer (their websocket closes after it)
    /// and tears down every peer connection in the room.
    pub(super) fn close(&mut self) {
        for (_, entry) in self.publishers.drain() {
            send_room_closed(entry.handle);
            if let Some(pc) = entry.pc {
                tokio::spawn(async move { pc.close().await });
            }
        }
        for (_, entry) in self.subscribers.drain() {
            send_room_closed(entry.handle);
            for pairing in entry.pairings.into_values() {
                pairing.close();
            }
        }
    }

    pub(super) fn peer_info(&self, peer_id: &PeerId) -> Option<PeerInfo> {
        let (role, handle) = match self.publishers.get(peer_id) {
            Some(entry) => (PeerRole::Publisher, &entry.handle),
            None => (PeerRole::Subscriber, &self.subscribers.get(peer_id)?.handle),
        };
        Some(PeerInfo {
            id: peer_id.clone(),
            role,
            ip: handle.remote_ip.clone(),
        })
    }

    /// Same removal as a peer leaving (a publisher still triggers
    /// `publisher_left`), then closes its connection. Nothing is sent to the
    /// peer first.
    pub(super) fn kick(&mut self, peer_id: PeerId) -> bool {
        let handle = self
            .publishers
            .get(&peer_id)
            .map(|entry| entry.handle.clone())
            .or_else(|| {
                self.subscribers
                    .get(&peer_id)
                    .map(|entry| entry.handle.clone())
            });
        let Some(handle) = handle else {
            return false;
        };
        self.handle_disconnect(peer_id);
        send_close(handle);
        true
    }

    pub(super) fn snapshot(&self) -> RoomSnapshot {
        let mut subscriber_quality = QualityCounts::default();
        for pairing in self
            .subscribers
            .values()
            .flat_map(|sub| sub.pairings.values())
        {
            match pairing.quality.quality() {
                ConnectionQuality::Good => subscriber_quality.good += 1,
                ConnectionQuality::Poor => subscriber_quality.poor += 1,
                ConnectionQuality::Bad => subscriber_quality.bad += 1,
            }
        }
        RoomSnapshot {
            publisher_ids: self.publishers.keys().cloned().collect(),
            subscriber_count: self.subscribers.len(),
            subscriber_quality,
        }
    }

    pub(super) fn broadcast_to_subscribers(&self, msg: ServerMessage) {
        for entry in self.subscribers.values() {
            send_to(&entry.handle, msg.clone());
        }
    }

    pub(super) fn error_to(&self, peer_id: &PeerId, error: DomainError) {
        let handle = self
            .publishers
            .get(peer_id)
            .map(|entry| &entry.handle)
            .or_else(|| self.subscribers.get(peer_id).map(|entry| &entry.handle));
        if let Some(handle) = handle {
            send_error(handle, error);
        }
    }
}

/// The operations `run` dispatches each command to. `Room` implements it by
/// delegating to its own methods; tests implement it with a spy that records
/// the calls instead of touching media.
#[async_trait]
pub(super) trait RoomOps: Send {
    fn handle_join(
        &mut self,
        role: PeerRole,
        handle: ConnectionHandle,
        reply: oneshot::Sender<Result<PeerId, DomainError>>,
    );
    async fn handle_sdp_offer(&mut self, peer_id: PeerId, target_peer: Option<PeerId>, sdp: String);
    async fn handle_sdp_answer(&mut self, peer_id: PeerId, target_peer: PeerId, sdp: String);
    async fn handle_ice_candidate(
        &mut self,
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        candidate: String,
    );
    async fn handle_request_track(&mut self, peer_id: PeerId, target_peer: PeerId);
    fn handle_connection_stats(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        report: StatsReport,
    );
    fn handle_set_preferred_layer(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        layer: SimulcastLayer,
    );
    fn handle_disconnect(&mut self, peer_id: PeerId);
    fn peer_info(&self, peer_id: &PeerId) -> Option<PeerInfo>;
    fn kick(&mut self, peer_id: PeerId) -> bool;
    fn snapshot(&self) -> RoomSnapshot;
    fn close(&mut self);
    fn handle_local_ice_candidate(
        &mut self,
        peer_id: PeerId,
        origin_publisher: Option<PeerId>,
        candidate: String,
    );
    async fn handle_track_published(&mut self, publisher_id: PeerId, track: Arc<dyn MediaTrack>);
    async fn request_keyframe(&self, publisher_id: &PeerId);
}

#[async_trait]
impl RoomOps for Room {
    fn handle_join(
        &mut self,
        role: PeerRole,
        handle: ConnectionHandle,
        reply: oneshot::Sender<Result<PeerId, DomainError>>,
    ) {
        Room::handle_join(self, role, handle, reply)
    }
    async fn handle_sdp_offer(
        &mut self,
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        sdp: String,
    ) {
        Room::handle_sdp_offer(self, peer_id, target_peer, sdp).await
    }
    async fn handle_sdp_answer(&mut self, peer_id: PeerId, target_peer: PeerId, sdp: String) {
        Room::handle_sdp_answer(self, peer_id, target_peer, sdp).await
    }
    async fn handle_ice_candidate(
        &mut self,
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        candidate: String,
    ) {
        Room::handle_ice_candidate(self, peer_id, target_peer, candidate).await
    }
    async fn handle_request_track(&mut self, peer_id: PeerId, target_peer: PeerId) {
        Room::handle_request_track(self, peer_id, target_peer).await
    }
    fn handle_connection_stats(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        report: StatsReport,
    ) {
        Room::handle_connection_stats(self, peer_id, target_peer, report)
    }
    fn handle_set_preferred_layer(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        layer: SimulcastLayer,
    ) {
        Room::handle_set_preferred_layer(self, peer_id, target_peer, layer)
    }
    fn handle_disconnect(&mut self, peer_id: PeerId) {
        Room::handle_disconnect(self, peer_id)
    }
    fn peer_info(&self, peer_id: &PeerId) -> Option<PeerInfo> {
        Room::peer_info(self, peer_id)
    }
    fn kick(&mut self, peer_id: PeerId) -> bool {
        Room::kick(self, peer_id)
    }
    fn snapshot(&self) -> RoomSnapshot {
        Room::snapshot(self)
    }
    fn close(&mut self) {
        Room::close(self)
    }
    fn handle_local_ice_candidate(
        &mut self,
        peer_id: PeerId,
        origin_publisher: Option<PeerId>,
        candidate: String,
    ) {
        Room::handle_local_ice_candidate(self, peer_id, origin_publisher, candidate)
    }
    async fn handle_track_published(&mut self, publisher_id: PeerId, track: Arc<dyn MediaTrack>) {
        Room::handle_track_published(self, publisher_id, track).await
    }
    async fn request_keyframe(&self, publisher_id: &PeerId) {
        Room::request_keyframe(self, publisher_id).await
    }
}

/// The room's run loop: one command at a time, each sent to the matching
/// `RoomOps` method, until the channel closes or the room is closed.
async fn run<R: RoomOps>(mut room: R, mut rx: mpsc::Receiver<RoomCommand>) {
    while let Some(cmd) = rx.recv().await {
        match cmd {
            RoomCommand::Join {
                role,
                handle,
                reply,
            } => room.handle_join(role, handle, reply),
            RoomCommand::SdpOffer {
                peer_id,
                target_peer,
                sdp,
            } => room.handle_sdp_offer(peer_id, target_peer, sdp).await,
            RoomCommand::SdpAnswer {
                peer_id,
                target_peer,
                sdp,
            } => room.handle_sdp_answer(peer_id, target_peer, sdp).await,
            RoomCommand::IceCandidate {
                peer_id,
                target_peer,
                candidate,
            } => {
                room.handle_ice_candidate(peer_id, target_peer, candidate)
                    .await
            }
            RoomCommand::RequestTrack {
                peer_id,
                target_peer,
            } => room.handle_request_track(peer_id, target_peer).await,
            RoomCommand::ConnectionStats {
                peer_id,
                target_peer,
                report,
            } => room.handle_connection_stats(peer_id, target_peer, report),
            RoomCommand::SetPreferredLayer {
                peer_id,
                target_peer,
                layer,
            } => room.handle_set_preferred_layer(peer_id, target_peer, layer),
            RoomCommand::Disconnect { peer_id } => room.handle_disconnect(peer_id),
            RoomCommand::PeerInfo { peer_id, reply } => {
                let _ = reply.send(room.peer_info(&peer_id));
            }
            RoomCommand::Kick { peer_id, reply } => {
                let _ = reply.send(room.kick(peer_id));
            }
            RoomCommand::Snapshot { reply } => {
                let _ = reply.send(room.snapshot());
            }
            RoomCommand::Close { reply } => {
                room.close();
                let _ = reply.send(());
                return;
            }
            RoomCommand::LocalIceCandidate {
                peer_id,
                origin_publisher,
                candidate,
            } => room.handle_local_ice_candidate(peer_id, origin_publisher, candidate),
            RoomCommand::TrackPublished {
                publisher_id,
                track,
            } => room.handle_track_published(publisher_id, track).await,
            RoomCommand::RequestKeyframe { publisher_id } => {
                room.request_keyframe(&publisher_id).await
            }
        }
    }
}

/// Forwards a media session's events into the room's command channel.
/// Engines call `emit` inline from their driver loop, so it must never wait
/// on the room actor (which may itself be waiting on that driver). Commands
/// are posted from a detached task instead.
pub(super) struct RoomEventSink {
    pub(super) room: mpsc::WeakSender<RoomCommand>,
    pub(super) peer_id: PeerId,
    /// `Some(publisher)` for a subscriber pairing, `None` for a publisher.
    pub(super) origin_publisher: Option<PeerId>,
}

impl MediaEventSink for RoomEventSink {
    fn emit(&self, event: MediaEvent) {
        let cmd = match event {
            MediaEvent::LocalIceCandidate { candidate } => RoomCommand::LocalIceCandidate {
                peer_id: self.peer_id.clone(),
                origin_publisher: self.origin_publisher.clone(),
                candidate,
            },
            MediaEvent::TrackPublished { track } => RoomCommand::TrackPublished {
                publisher_id: self.peer_id.clone(),
                track,
            },
            MediaEvent::Connected => match &self.origin_publisher {
                Some(publisher_id) => RoomCommand::RequestKeyframe {
                    publisher_id: publisher_id.clone(),
                },
                None => return,
            },
        };
        if let Some(tx) = self.room.upgrade() {
            tokio::spawn(async move {
                let _ = tx.send(cmd).await;
            });
        }
    }
}

pub(super) fn send_error(handle: &ConnectionHandle, error: DomainError) {
    send_to(handle, error.into());
}

/// Unlike `send_to`, waits (briefly) for room in a full outbound channel:
/// `room_closed` is what makes the peer's websocket close, so it mustn't
/// be dropped just because the peer is busy.
pub(super) fn send_room_closed(handle: ConnectionHandle) {
    tokio::spawn(async move {
        let send = async {
            handle
                .outbound
                .send(ServerMessage::RoomClosed.into())
                .await
                .ok()?;
            handle.outbound.send(Outbound::Close).await.ok()
        };
        let _ = tokio::time::timeout(std::time::Duration::from_secs(2), send).await;
    });
}

/// Asks a peer's connection to close, waiting (briefly) for room in a full
/// outbound channel so the close isn't lost to a busy peer.
fn send_close(handle: ConnectionHandle) {
    tokio::spawn(async move {
        let send = handle.outbound.send(Outbound::Close);
        let _ = tokio::time::timeout(std::time::Duration::from_secs(2), send).await;
    });
}

/// Best-effort push to a peer's outbound channel. A slow/gone client
/// should never stall the room actor, so this never awaits — a full or
/// closed channel just drops the message.
pub(super) fn send_to(handle: &ConnectionHandle, msg: ServerMessage) {
    if handle.outbound.try_send(msg.into()).is_err() {
        tracing::debug!("dropping server message: peer outbound channel full or closed");
    }
}

#[cfg(test)]
mod tests {
    use std::any::Any;
    use std::sync::Mutex;
    use std::time::Duration;

    use super::*;

    /// Records which `RoomOps` method `run` called, in order. Besides
    /// recording, the spy gives the test a way to sync on dispatch:
    /// `handle_track_published` fires `published`, and `handle_join` replies
    /// with a fixed peer id.
    #[derive(Default)]
    struct SpyRoom {
        calls: Arc<Mutex<Vec<&'static str>>>,
        published: Option<oneshot::Sender<()>>,
    }

    impl SpyRoom {
        fn record(&self, name: &'static str) {
            self.calls.lock().unwrap().push(name);
        }
    }

    #[async_trait]
    impl RoomOps for SpyRoom {
        fn handle_join(
            &mut self,
            _role: PeerRole,
            _handle: ConnectionHandle,
            reply: oneshot::Sender<Result<PeerId, DomainError>>,
        ) {
            self.record("handle_join");
            let _ = reply.send(Ok("spy-peer".to_string()));
        }
        async fn handle_sdp_offer(&mut self, _: PeerId, _: Option<PeerId>, _: String) {
            self.record("handle_sdp_offer");
        }
        async fn handle_sdp_answer(&mut self, _: PeerId, _: PeerId, _: String) {
            self.record("handle_sdp_answer");
        }
        async fn handle_ice_candidate(&mut self, _: PeerId, _: Option<PeerId>, _: String) {
            self.record("handle_ice_candidate");
        }
        async fn handle_request_track(&mut self, _: PeerId, _: PeerId) {
            self.record("handle_request_track");
        }
        fn handle_connection_stats(&mut self, _: PeerId, _: PeerId, _: StatsReport) {
            self.record("handle_connection_stats");
        }
        fn handle_set_preferred_layer(&mut self, _: PeerId, _: PeerId, _: SimulcastLayer) {
            self.record("handle_set_preferred_layer");
        }
        fn handle_disconnect(&mut self, _: PeerId) {
            self.record("handle_disconnect");
        }
        fn peer_info(&self, _: &PeerId) -> Option<PeerInfo> {
            self.record("peer_info");
            None
        }
        fn kick(&mut self, _: PeerId) -> bool {
            self.record("kick");
            false
        }
        fn snapshot(&self) -> RoomSnapshot {
            self.record("snapshot");
            RoomSnapshot::default()
        }
        fn close(&mut self) {
            self.record("close");
        }
        fn handle_local_ice_candidate(&mut self, _: PeerId, _: Option<PeerId>, _: String) {
            self.record("handle_local_ice_candidate");
        }
        async fn handle_track_published(&mut self, _: PeerId, _: Arc<dyn MediaTrack>) {
            self.record("handle_track_published");
            if let Some(tx) = self.published.take() {
                let _ = tx.send(());
            }
        }
        async fn request_keyframe(&self, _: &PeerId) {
            self.record("request_keyframe");
        }
    }

    struct SpyTrack;

    #[async_trait]
    impl MediaTrack for SpyTrack {
        fn mime_type(&self) -> String {
            "video/VP8".to_string()
        }
        fn is_video(&self) -> bool {
            true
        }
        fn effective_layer(&self, target: SimulcastLayer) -> Option<SimulcastLayer> {
            Some(target)
        }
        async fn request_keyframe(&self) {}
        fn into_any(self: Arc<Self>) -> Arc<dyn Any + Send + Sync> {
            self
        }
    }

    struct TestCase {
        name: &'static str,
        setup: fn() -> RoomCommand,
        expected: &'static str,
    }

    /// Each command reaches exactly its own handler, once.
    #[tokio::test]
    async fn each_command_dispatches_to_its_handler() {
        let cases = [
            TestCase {
                name: "join",
                setup: || RoomCommand::Join {
                    role: PeerRole::Publisher,
                    handle: ConnectionHandle {
                        outbound: mpsc::channel(1).0,
                        remote_ip: None,
                    },
                    reply: oneshot::channel().0,
                },
                expected: "handle_join",
            },
            TestCase {
                name: "sdp offer",
                setup: || RoomCommand::SdpOffer {
                    peer_id: "pub-1".into(),
                    target_peer: None,
                    sdp: "offer".into(),
                },
                expected: "handle_sdp_offer",
            },
            TestCase {
                name: "sdp answer",
                setup: || RoomCommand::SdpAnswer {
                    peer_id: "sub-1".into(),
                    target_peer: "pub-1".into(),
                    sdp: "answer".into(),
                },
                expected: "handle_sdp_answer",
            },
            TestCase {
                name: "ice candidate",
                setup: || RoomCommand::IceCandidate {
                    peer_id: "pub-1".into(),
                    target_peer: None,
                    candidate: "{}".into(),
                },
                expected: "handle_ice_candidate",
            },
            TestCase {
                name: "request track",
                setup: || RoomCommand::RequestTrack {
                    peer_id: "sub-1".into(),
                    target_peer: "pub-1".into(),
                },
                expected: "handle_request_track",
            },
            TestCase {
                name: "connection stats",
                setup: || RoomCommand::ConnectionStats {
                    peer_id: "sub-1".into(),
                    target_peer: "pub-1".into(),
                    report: StatsReport {
                        available_incoming_bitrate_bps: None,
                        packet_loss_pct: 0.0,
                    },
                },
                expected: "handle_connection_stats",
            },
            TestCase {
                name: "set preferred layer",
                setup: || RoomCommand::SetPreferredLayer {
                    peer_id: "sub-1".into(),
                    target_peer: "pub-1".into(),
                    layer: SimulcastLayer::Low,
                },
                expected: "handle_set_preferred_layer",
            },
            TestCase {
                name: "disconnect",
                setup: || RoomCommand::Disconnect {
                    peer_id: "sub-1".into(),
                },
                expected: "handle_disconnect",
            },
            TestCase {
                name: "peer info",
                setup: || RoomCommand::PeerInfo {
                    peer_id: "sub-1".into(),
                    reply: oneshot::channel().0,
                },
                expected: "peer_info",
            },
            TestCase {
                name: "kick",
                setup: || RoomCommand::Kick {
                    peer_id: "sub-1".into(),
                    reply: oneshot::channel().0,
                },
                expected: "kick",
            },
            TestCase {
                name: "snapshot",
                setup: || RoomCommand::Snapshot {
                    reply: oneshot::channel().0,
                },
                expected: "snapshot",
            },
            TestCase {
                name: "close",
                setup: || RoomCommand::Close {
                    reply: oneshot::channel().0,
                },
                expected: "close",
            },
            TestCase {
                name: "local ice candidate",
                setup: || RoomCommand::LocalIceCandidate {
                    peer_id: "pub-1".into(),
                    origin_publisher: None,
                    candidate: "{}".into(),
                },
                expected: "handle_local_ice_candidate",
            },
            TestCase {
                name: "track published",
                setup: || RoomCommand::TrackPublished {
                    publisher_id: "pub-1".into(),
                    track: Arc::new(SpyTrack),
                },
                expected: "handle_track_published",
            },
            TestCase {
                name: "request keyframe",
                setup: || RoomCommand::RequestKeyframe {
                    publisher_id: "pub-1".into(),
                },
                expected: "request_keyframe",
            },
        ];

        for case in cases {
            println!("case: {}", case.name);
            let spy = SpyRoom::default();
            let calls = spy.calls.clone();
            let (tx, rx) = mpsc::channel(8);
            tx.send((case.setup)()).await.unwrap();
            drop(tx);
            run(spy, rx).await;
            assert_eq!(
                calls.lock().unwrap().as_slice(),
                [case.expected],
                "{}",
                case.name
            );
        }
    }

    /// `Close` ends the loop, so a command queued after it is never dispatched.
    #[tokio::test]
    async fn close_stops_dispatching() {
        let spy = SpyRoom::default();
        let calls = spy.calls.clone();
        let (tx, rx) = mpsc::channel(8);
        tx.send(RoomCommand::Close {
            reply: oneshot::channel().0,
        })
        .await
        .unwrap();
        tx.send(RoomCommand::Snapshot {
            reply: oneshot::channel().0,
        })
        .await
        .unwrap();
        drop(tx);

        run(spy, rx).await;

        assert_eq!(calls.lock().unwrap().as_slice(), ["close"]);
    }

    /// The spy's own behavior: it signals when `handle_track_published` runs,
    /// so the test waits on that instead of sleeping.
    #[tokio::test]
    async fn spy_signals_track_published() {
        let (published_tx, published_rx) = oneshot::channel();
        let spy = SpyRoom {
            published: Some(published_tx),
            ..Default::default()
        };
        let (tx, rx) = mpsc::channel(8);
        tx.send(RoomCommand::TrackPublished {
            publisher_id: "pub-1".into(),
            track: Arc::new(SpyTrack),
        })
        .await
        .unwrap();

        let runner = tokio::spawn(run(spy, rx));
        tokio::time::timeout(Duration::from_millis(500), published_rx)
            .await
            .expect("handle_track_published was not dispatched")
            .unwrap();
        drop(tx);
        runner.await.unwrap();
    }
}
