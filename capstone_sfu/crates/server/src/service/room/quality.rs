use common::signaling::{PeerId, ServerMessage, SimulcastLayer};

use super::actor::{send_error, send_to, Room};
use super::state::Pairing;
use crate::domain::error::DomainError;
use crate::domain::quality::StatsReport;

impl Room {
    pub(super) fn handle_connection_stats(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        report: StatsReport,
    ) {
        let Some(pairing) = self.pairing_for(&peer_id, &target_peer, "connection_stats") else {
            return;
        };
        if pairing.quality.update(&report) {
            tracing::info!(
                %peer_id,
                %target_peer,
                quality = ?pairing.quality.quality(),
                layer = ?pairing.quality.layer(),
                "subscriber downgraded"
            );
            pairing.target_layer.send_replace(pairing.quality.layer());
        }
        self.report_quality(&peer_id, &target_peer, false);
    }

    /// The subscriber picked a layer for one pairing. Applied at once, even
    /// above a layer it was downgraded from: that's the manual retry.
    pub(super) fn handle_set_preferred_layer(
        &mut self,
        peer_id: PeerId,
        target_peer: PeerId,
        layer: SimulcastLayer,
    ) {
        let Some(pairing) = self.pairing_for(&peer_id, &target_peer, "set_preferred_layer") else {
            return;
        };
        pairing.quality.set_preferred(layer);
        pairing.target_layer.send_replace(layer);
        // Always answer, so the picker shows what's really forwarded even
        // when the publisher doesn't send the picked layer.
        self.report_quality(&peer_id, &target_peer, true);
    }

    /// The pairing a subscriber's quality message is about, or an `error`
    /// back to the sender if there isn't one.
    pub(super) fn pairing_for(
        &mut self,
        peer_id: &PeerId,
        target_peer: &PeerId,
        message: &str,
    ) -> Option<&mut Pairing> {
        if !self.subscribers.contains_key(peer_id) {
            self.error_to(
                peer_id,
                DomainError::WrongRole(format!("only a subscriber may send {message}")),
            );
            return None;
        }
        let entry = self.subscribers.get_mut(peer_id)?;
        if !entry.pairings.contains_key(target_peer) {
            send_error(
                &entry.handle,
                DomainError::PeerNotFound(format!(
                    "{message} target_peer must name a requested publisher"
                )),
            );
            return None;
        }
        entry.pairings.get_mut(target_peer)
    }

    /// Sends `connection_quality` for a pairing if it changed since the last
    /// one, or unconditionally with `force`. The layer reported is the one
    /// actually forwarded, which also depends on which layers the publisher
    /// is sending right now, so it's re-checked every time.
    pub(super) fn report_quality(
        &mut self,
        subscriber_id: &PeerId,
        publisher_id: &PeerId,
        force: bool,
    ) {
        let Some(entry) = self.subscribers.get_mut(subscriber_id) else {
            return;
        };
        let Some(pairing) = entry.pairings.get_mut(publisher_id) else {
            return;
        };
        let target = pairing.quality.layer();
        let layer = self
            .publishers
            .get(publisher_id)
            .and_then(|publisher| {
                publisher
                    .tracks
                    .iter()
                    .filter(|track| track.is_video())
                    .find_map(|track| track.effective_layer(target))
            })
            .unwrap_or(target);

        let current = (pairing.quality.quality(), layer);
        if force || current != pairing.reported {
            pairing.reported = current;
            send_to(
                &entry.handle,
                ServerMessage::ConnectionQuality {
                    from_peer: publisher_id.clone(),
                    quality: current.0,
                    layer: current.1,
                },
            );
        }
    }
}
