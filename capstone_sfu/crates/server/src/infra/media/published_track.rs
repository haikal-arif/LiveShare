use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use common::signaling::SimulcastLayer;
use rtc::rtcp::payload_feedbacks::picture_loss_indication::PictureLossIndication;
use rtc::rtp;
use rtc::rtp_transceiver::rtp_sender::{RTCRtpCodec, RtpCodecKind};
use tokio::sync::broadcast;
use webrtc::media_stream::track_remote::{TrackRemote, TrackRemoteEvent};

/// A layer that hasn't carried media for this long is treated as gone, so
/// a publisher dropping a layer (e.g. its own bandwidth got tight) doesn't
/// freeze the subscribers forwarded that layer.
const LAYER_IDLE_TIMEOUT: Duration = Duration::from_secs(1);

#[derive(Clone)]
pub(super) struct LayerPacket {
    pub(super) layer: SimulcastLayer,
    pub(super) packet: rtp::Packet,
}

#[derive(Default)]
struct LayerState {
    ssrcs: HashMap<SimulcastLayer, u32>,
    last_seen: HashMap<SimulcastLayer, Instant>,
}

/// One track a publisher is sending into the SFU. A track without
/// simulcast has a single `High` layer.
pub struct PublishedTrack {
    pub kind: RtpCodecKind,
    pub codec: RTCRtpCodec,
    remote: Arc<dyn TrackRemote>,
    layers: Mutex<LayerState>,
    pub(super) packets: broadcast::Sender<LayerPacket>,
}

impl PublishedTrack {
    /// Starts the single reader task for a freshly received remote track.
    /// Returns `None` if the track has no SSRC/codec yet (nothing to forward).
    pub async fn start(remote: Arc<dyn TrackRemote>) -> Option<Arc<Self>> {
        let ssrcs = remote.ssrcs().await;
        let first_ssrc = *ssrcs.first()?;
        let codec = remote.codec(first_ssrc).await?;
        let kind = remote.kind().await;
        let mut rids = Vec::with_capacity(ssrcs.len());
        for ssrc in ssrcs {
            rids.push((ssrc, remote.rid(ssrc).await));
        }
        let (packets, _) = broadcast::channel(1024);

        let track = Arc::new(Self {
            kind,
            codec,
            remote,
            layers: Mutex::default(),
            packets,
        });
        // Layers already known now; later ones are announced by `OnOpen`.
        let mut ssrc_layers = HashMap::new();
        for (ssrc, rid) in rids {
            if let Some(layer) = rid.as_deref().and_then(SimulcastLayer::from_rid) {
                track.map_ssrc(&mut ssrc_layers, ssrc, layer);
            }
        }
        if ssrc_layers.is_empty() {
            // No rids: not simulcast, so this is the one and only layer.
            track.map_ssrc(&mut ssrc_layers, first_ssrc, SimulcastLayer::High);
        }

        let reader = track.clone();
        tokio::spawn(async move {
            // Only each layer's primary media stream is relayed: RTX/FEC use
            // a separate SSRC or payload type whose numbering is specific to
            // the publisher's leg and can't be forwarded as-is.
            let mut primary_pt = None;
            while let Some(event) = reader.remote.poll().await {
                match event {
                    // Fires once per simulcast layer as its first packet arrives.
                    TrackRemoteEvent::OnOpen(init) => {
                        let Some(rid) = init.rid else { continue };
                        match SimulcastLayer::from_rid(&rid) {
                            Some(layer) => reader.map_ssrc(&mut ssrc_layers, init.ssrc, layer),
                            None => tracing::warn!(
                                %rid,
                                "ignoring simulcast layer with unknown rid (expected h/m/l)"
                            ),
                        }
                    }
                    TrackRemoteEvent::OnRtpPacket(packet) => {
                        let Some(&layer) = ssrc_layers.get(&packet.header.ssrc) else {
                            continue;
                        };
                        let pt = *primary_pt.get_or_insert(packet.header.payload_type);
                        if packet.header.payload_type == pt {
                            reader
                                .layers
                                .lock()
                                .unwrap()
                                .last_seen
                                .insert(layer, Instant::now());
                            let _ = reader.packets.send(LayerPacket { layer, packet });
                        }
                    }
                    TrackRemoteEvent::OnEnded => break,
                    _ => {}
                }
            }
        });

        Some(track)
    }

    fn map_ssrc(
        &self,
        ssrc_layers: &mut HashMap<u32, SimulcastLayer>,
        ssrc: u32,
        layer: SimulcastLayer,
    ) {
        ssrc_layers.insert(ssrc, layer);
        let mut layers = self.layers.lock().unwrap();
        // The rid can arrive after the ssrc was first seen without one.
        layers.ssrcs.retain(|_, s| *s != ssrc);
        layers.ssrcs.insert(layer, ssrc);
    }

    /// The layer a subscriber targeting `target` should be forwarded: the
    /// best live layer not above `target`, else the lowest live one. With
    /// no live layer yet, known layers are considered instead. `None` only
    /// before any layer is known.
    pub fn effective_layer(&self, target: SimulcastLayer) -> Option<SimulcastLayer> {
        let layers = self.layers.lock().unwrap();
        let now = Instant::now();
        let live: Vec<SimulcastLayer> = layers
            .last_seen
            .iter()
            .filter(|(_, seen)| now.duration_since(**seen) < LAYER_IDLE_TIMEOUT)
            .map(|(layer, _)| *layer)
            .collect();
        let candidates = if live.is_empty() {
            layers.ssrcs.keys().copied().collect()
        } else {
            live
        };
        pick_layer(&candidates, target)
    }

    /// Asks the publisher for a fresh keyframe on every layer, so a newly
    /// attached subscriber can start decoding immediately.
    pub async fn request_keyframe(&self) {
        let ssrcs: Vec<u32> = self
            .layers
            .lock()
            .unwrap()
            .ssrcs
            .values()
            .copied()
            .collect();
        for ssrc in ssrcs {
            self.write_pli(ssrc).await;
        }
    }

    /// Asks for a keyframe on one layer, e.g. to switch a subscriber onto it.
    pub(super) async fn request_layer_keyframe(&self, layer: SimulcastLayer) {
        let ssrc = self.layers.lock().unwrap().ssrcs.get(&layer).copied();
        if let Some(ssrc) = ssrc {
            self.write_pli(ssrc).await;
        }
    }

    async fn write_pli(&self, media_ssrc: u32) {
        if self.kind != RtpCodecKind::Video {
            return;
        }
        let pli = PictureLossIndication {
            sender_ssrc: 0,
            media_ssrc,
        };
        let _ = self.remote.write_rtcp(vec![Box::new(pli)]).await;
    }
}

fn pick_layer(available: &[SimulcastLayer], target: SimulcastLayer) -> Option<SimulcastLayer> {
    available
        .iter()
        .copied()
        .filter(|layer| *layer <= target)
        .max()
        .or_else(|| available.iter().copied().min())
}

#[cfg(test)]
mod tests {
    use super::*;
    use SimulcastLayer::{High, Low, Mid};

    #[test]
    fn pick_layer_prefers_best_not_above_target() {
        assert_eq!(pick_layer(&[High, Mid, Low], Mid), Some(Mid));
        assert_eq!(pick_layer(&[High, Low], Mid), Some(Low));
        assert_eq!(pick_layer(&[High, Mid], Low), Some(Mid));
        assert_eq!(pick_layer(&[High], Low), Some(High));
        assert_eq!(pick_layer(&[], High), None);
    }

    #[test]
    fn rid_mapping() {
        assert_eq!(SimulcastLayer::from_rid("h"), Some(High));
        assert_eq!(SimulcastLayer::from_rid("l"), Some(Low));
        assert_eq!(SimulcastLayer::from_rid("q"), None, "only h/m/l are layers");
    }
}
