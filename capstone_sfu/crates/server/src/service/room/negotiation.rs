use std::sync::Arc;

use common::signaling::{ConnectionQuality, PeerId, ServerMessage, SimulcastLayer};
use tokio::sync::watch;

use super::actor::{send_error, send_to, Room, RoomEventSink};
use super::state::Pairing;
use crate::domain::error::DomainError;
use crate::domain::ports::{MediaError, MediaTrack};
use crate::domain::quality::QualityTracker;

impl Room {
    pub(super) async fn handle_sdp_offer(
        &mut self,
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        sdp: String,
    ) {
        let Some(entry) = self.publishers.get_mut(&peer_id) else {
            self.error_to(
                &peer_id,
                DomainError::WrongRole("only a publisher may send an unsolicited sdp_offer".into()),
            );
            return;
        };
        if target_peer.is_some() {
            send_error(
                &entry.handle,
                DomainError::InvalidMessage(
                    "target_peer must be null on the publisher/SFU pairing".into(),
                ),
            );
            return;
        }

        let pc = match &entry.pc {
            Some(pc) => pc.clone(),
            None => {
                let events = Arc::new(RoomEventSink {
                    room: self.self_tx.clone(),
                    peer_id: peer_id.clone(),
                    origin_publisher: None,
                });
                match self.media.new_publisher_session(events).await {
                    Ok(pc) => entry.pc.insert(pc).clone(),
                    Err(err) => {
                        tracing::warn!(%peer_id, "failed to create publisher peer connection: {err}");
                        send_error(
                            &entry.handle,
                            DomainError::Internal("failed to create peer connection".into()),
                        );
                        return;
                    }
                }
            }
        };

        match pc.accept_offer(sdp).await {
            Ok(answer) => send_to(
                &entry.handle,
                ServerMessage::SdpAnswer {
                    from_peer: None,
                    sdp: answer,
                },
            ),
            Err(err) => {
                tracing::debug!(%peer_id, "rejecting publisher offer: {err}");
                send_error(
                    &entry.handle,
                    DomainError::InvalidSdp(format!("invalid sdp_offer: {err}")),
                );
            }
        }
    }

    pub(super) async fn handle_request_track(&mut self, peer_id: PeerId, target_peer: PeerId) {
        if !self.publishers.contains_key(&target_peer) {
            self.error_to(
                &peer_id,
                DomainError::PeerNotFound("publisher not found".into()),
            );
            return;
        }
        let Some(entry) = self.subscribers.get_mut(&peer_id) else {
            self.error_to(
                &peer_id,
                DomainError::WrongRole("only a subscriber may request a track".into()),
            );
            return;
        };

        if !entry.pairings.contains_key(&target_peer) {
            let events = Arc::new(RoomEventSink {
                room: self.self_tx.clone(),
                peer_id: peer_id.clone(),
                origin_publisher: Some(target_peer.clone()),
            });
            let pc = match self.media.new_subscriber_session(events).await {
                Ok(pc) => pc,
                Err(err) => {
                    tracing::warn!(%peer_id, "failed to create subscriber peer connection: {err}");
                    send_error(
                        &entry.handle,
                        DomainError::Internal("failed to create peer connection".into()),
                    );
                    return;
                }
            };
            entry.pairings.insert(
                target_peer.clone(),
                Pairing {
                    pc,
                    attached: 0,
                    awaiting_answer: false,
                    renegotiate: false,
                    quality: QualityTracker::default(),
                    target_layer: watch::Sender::new(SimulcastLayer::High),
                    reported: (ConnectionQuality::Good, SimulcastLayer::High),
                },
            );
        }

        self.attach_new_tracks(&peer_id, &target_peer).await;
        self.offer(&peer_id, &target_peer).await;
    }

    pub(super) async fn handle_sdp_answer(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        sdp: String,
    ) {
        let Some(entry) = self.subscribers.get_mut(&peer_id) else {
            self.error_to(
                &peer_id,
                DomainError::WrongRole("only a subscriber may send an sdp_answer".into()),
            );
            return;
        };
        let Some(pairing) = entry.pairings.get_mut(&target_peer) else {
            send_error(
                &entry.handle,
                DomainError::InvalidState("no pending offer for that target_peer".into()),
            );
            return;
        };

        let result = pairing.pc.apply_answer(sdp).await;
        pairing.awaiting_answer = false;
        if let Err(err) = result {
            tracing::debug!(%peer_id, %target_peer, "rejecting subscriber answer: {err}");
            send_error(
                &entry.handle,
                DomainError::InvalidSdp(format!("invalid sdp_answer: {err}")),
            );
            return;
        }

        let renegotiate = std::mem::take(&mut pairing.renegotiate);
        // Newly attached tracks only start flowing now, so they need a keyframe.
        self.request_keyframe(&target_peer).await;
        if renegotiate {
            self.offer(&peer_id, &target_peer).await;
        }
    }

    pub(super) async fn handle_ice_candidate(
        &mut self,
        peer_id: PeerId,
        target_peer: Option<PeerId>,
        candidate: String,
    ) {
        let pc = if let Some(entry) = self.publishers.get(&peer_id) {
            if target_peer.is_some() {
                send_error(
                    &entry.handle,
                    DomainError::InvalidMessage(
                        "target_peer must be null on the publisher/SFU pairing".into(),
                    ),
                );
                return;
            }
            entry.pc.clone()
        } else if let Some(entry) = self.subscribers.get(&peer_id) {
            match target_peer.and_then(|target| entry.pairings.get(&target)) {
                Some(pairing) => Some(pairing.pc.clone()),
                None => {
                    send_error(
                        &entry.handle,
                        DomainError::PeerNotFound(
                            "ice_candidate target_peer must name a requested publisher".into(),
                        ),
                    );
                    return;
                }
            }
        } else {
            self.error_to(&peer_id, DomainError::NotJoined("not joined".into()));
            return;
        };

        let Some(pc) = pc else {
            self.error_to(
                &peer_id,
                DomainError::InvalidState("ice_candidate before sdp_offer".into()),
            );
            return;
        };
        match pc.add_ice_candidate(&candidate).await {
            Ok(()) => {}
            Err(MediaError::InvalidCandidate) => self.error_to(
                &peer_id,
                DomainError::InvalidMessage("invalid ice candidate".into()),
            ),
            Err(err) => tracing::debug!(%peer_id, "failed to add remote ice candidate: {err}"),
        }
    }

    pub(super) fn handle_local_ice_candidate(
        &mut self,
        peer_id: PeerId,
        origin_publisher: Option<PeerId>,
        candidate: String,
    ) {
        let msg = ServerMessage::IceCandidate {
            from_peer: origin_publisher.clone(),
            candidate,
        };
        match origin_publisher {
            None => {
                if let Some(entry) = self.publishers.get(&peer_id) {
                    send_to(&entry.handle, msg);
                }
            }
            Some(publisher_id) => {
                let paired = self
                    .subscribers
                    .get(&peer_id)
                    .filter(|entry| entry.pairings.contains_key(&publisher_id));
                if let Some(entry) = paired {
                    send_to(&entry.handle, msg);
                }
            }
        }
    }

    pub(super) async fn handle_track_published(
        &mut self,
        publisher_id: PeerId,
        track: Arc<dyn MediaTrack>,
    ) {
        let Some(entry) = self.publishers.get_mut(&publisher_id) else {
            return;
        };
        tracing::info!(%publisher_id, codec = %track.mime_type(), "publisher track available");
        entry.tracks.push(track);

        let paired: Vec<PeerId> = self
            .subscribers
            .iter()
            .filter(|(_, sub)| sub.pairings.contains_key(&publisher_id))
            .map(|(id, _)| id.clone())
            .collect();
        for subscriber_id in paired {
            self.attach_new_tracks(&subscriber_id, &publisher_id).await;
            self.offer(&subscriber_id, &publisher_id).await;
        }
    }

    /// Adds any of the publisher's tracks not yet on this pairing's
    /// `PeerConnection`. Does not renegotiate by itself.
    pub(super) async fn attach_new_tracks(
        &mut self,
        subscriber_id: &PeerId,
        publisher_id: &PeerId,
    ) {
        let Some(publisher) = self.publishers.get(publisher_id) else {
            return;
        };
        let Some(pairing) = self
            .subscribers
            .get_mut(subscriber_id)
            .and_then(|sub| sub.pairings.get_mut(publisher_id))
        else {
            return;
        };
        for track in &publisher.tracks[pairing.attached..] {
            let target = pairing.target_layer.subscribe();
            if let Err(err) = pairing.pc.attach_track(track, publisher_id, target).await {
                tracing::warn!(%subscriber_id, %publisher_id, "failed to attach track: {err}");
            }
        }
        pairing.attached = publisher.tracks.len();
    }

    /// Sends a fresh SFU offer for one pairing, or defers it if the previous
    /// offer is still waiting on its answer. Until the publisher has at
    /// least one track there is nothing to offer (an offer with no m-lines
    /// carries no ICE credentials), so `handle_track_published` sends the
    /// first offer instead.
    pub(super) async fn offer(&mut self, subscriber_id: &PeerId, publisher_id: &PeerId) {
        let Some(sub) = self.subscribers.get_mut(subscriber_id) else {
            return;
        };
        let Some(pairing) = sub.pairings.get_mut(publisher_id) else {
            return;
        };
        if pairing.attached == 0 {
            return;
        }
        if pairing.awaiting_answer {
            pairing.renegotiate = true;
            return;
        }

        let sdp = match pairing.pc.create_offer().await {
            Ok(sdp) => sdp,
            Err(err) => {
                tracing::warn!(%subscriber_id, %publisher_id, "failed to create offer: {err}");
                send_error(
                    &sub.handle,
                    DomainError::Internal("failed to create offer".into()),
                );
                return;
            }
        };
        pairing.awaiting_answer = true;
        send_to(
            &sub.handle,
            ServerMessage::SdpOffer {
                from_peer: Some(publisher_id.clone()),
                sdp,
            },
        );
    }

    pub(super) async fn request_keyframe(&self, publisher_id: &PeerId) {
        if let Some(entry) = self.publishers.get(publisher_id) {
            for track in &entry.tracks {
                track.request_keyframe().await;
            }
        }
    }
}
