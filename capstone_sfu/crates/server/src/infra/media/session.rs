use std::any::Any;
use std::sync::{Arc, Mutex};

use async_trait::async_trait;
use common::signaling::SimulcastLayer;
use tokio::sync::watch;
use tokio::task::JoinHandle;
use webrtc::media_stream::track_remote::TrackRemote;
use webrtc::peer_connection::{
    PeerConnectionEventHandler, RTCIceCandidateInit, RTCPeerConnectionIceEvent,
    RTCPeerConnectionState, RTCSessionDescription,
};

use super::engine::{MediaContext, SharedPeerConnection};
use super::published_track::PublishedTrack;
use super::relay::attach_track;
use crate::domain::ports::{
    MediaError, MediaEvent, MediaEventSink, MediaGateway, MediaSession, MediaTrack,
};

#[async_trait]
impl MediaGateway for MediaContext {
    async fn new_publisher_session(
        &self,
        events: Arc<dyn MediaEventSink>,
    ) -> Result<Arc<dyn MediaSession>, MediaError> {
        let handler = PublisherPcHandler {
            events,
            public_ips: self.public_ips(),
        };
        let pc = self
            .new_peer_connection(Arc::new(handler))
            .await
            .map_err(|err| MediaError::Engine(err.to_string()))?;
        Ok(Arc::new(WebrtcSession::new(pc)))
    }

    async fn new_subscriber_session(
        &self,
        events: Arc<dyn MediaEventSink>,
    ) -> Result<Arc<dyn MediaSession>, MediaError> {
        let handler = SubscriberPcHandler {
            events,
            public_ips: self.public_ips(),
        };
        let pc = self
            .new_peer_connection(Arc::new(handler))
            .await
            .map_err(|err| MediaError::Engine(err.to_string()))?;
        Ok(Arc::new(WebrtcSession::new(pc)))
    }
}

#[async_trait]
impl MediaTrack for PublishedTrack {
    fn mime_type(&self) -> String {
        self.codec.mime_type.clone()
    }

    fn is_video(&self) -> bool {
        self.kind == rtc::rtp_transceiver::rtp_sender::RtpCodecKind::Video
    }

    fn effective_layer(&self, target: SimulcastLayer) -> Option<SimulcastLayer> {
        PublishedTrack::effective_layer(self, target)
    }

    async fn request_keyframe(&self) {
        PublishedTrack::request_keyframe(self).await
    }

    fn into_any(self: Arc<Self>) -> Arc<dyn Any + Send + Sync> {
        self
    }
}

/// A `MediaSession` over one webrtc-rs `PeerConnection`, plus the relay
/// tasks feeding it.
struct WebrtcSession {
    pc: SharedPeerConnection,
    relays: Mutex<Vec<JoinHandle<()>>>,
}

impl WebrtcSession {
    fn new(pc: SharedPeerConnection) -> Self {
        Self {
            pc,
            relays: Mutex::new(Vec::new()),
        }
    }
}

#[async_trait]
impl MediaSession for WebrtcSession {
    async fn accept_offer(&self, sdp: String) -> Result<String, MediaError> {
        let answer = async {
            self.pc
                .set_remote_description(RTCSessionDescription::offer(sdp)?)
                .await?;
            let answer = self.pc.create_answer(None).await?;
            self.pc.set_local_description(answer.clone()).await?;
            Ok::<_, webrtc::error::Error>(
                self.pc
                    .local_description()
                    .await
                    .map_or(answer.sdp, |d| d.sdp),
            )
        };
        answer
            .await
            .map_err(|err| MediaError::InvalidSdp(err.to_string()))
    }

    async fn create_offer(&self) -> Result<String, MediaError> {
        let offer = async {
            let offer = self.pc.create_offer(None).await?;
            self.pc.set_local_description(offer.clone()).await?;
            Ok::<_, webrtc::error::Error>(
                self.pc
                    .local_description()
                    .await
                    .map_or(offer.sdp, |d| d.sdp),
            )
        };
        offer
            .await
            .map_err(|err| MediaError::Engine(err.to_string()))
    }

    async fn apply_answer(&self, sdp: String) -> Result<(), MediaError> {
        let result = match RTCSessionDescription::answer(sdp) {
            Ok(answer) => self.pc.set_remote_description(answer).await,
            Err(err) => Err(err),
        };
        result.map_err(|err| MediaError::InvalidSdp(err.to_string()))
    }

    async fn add_ice_candidate(&self, candidate_json: &str) -> Result<(), MediaError> {
        let init = serde_json::from_str::<RTCIceCandidateInit>(candidate_json)
            .map_err(|_| MediaError::InvalidCandidate)?;
        // An empty candidate string is an end-of-candidates marker.
        if init.candidate.is_empty() {
            return Ok(());
        }
        self.pc
            .add_ice_candidate(init)
            .await
            .map_err(|err| MediaError::Engine(err.to_string()))
    }

    async fn attach_track(
        &self,
        track: &Arc<dyn MediaTrack>,
        stream_id: &str,
        layer: watch::Receiver<SimulcastLayer>,
    ) -> Result<(), MediaError> {
        let track = track
            .clone()
            .into_any()
            .downcast::<PublishedTrack>()
            .map_err(|_| MediaError::Engine("track was not produced by this engine".into()))?;
        let relay = attach_track(&self.pc, &track, stream_id, layer)
            .await
            .map_err(|err| MediaError::Engine(err.to_string()))?;
        self.relays.lock().unwrap().push(relay);
        Ok(())
    }

    async fn close(&self) {
        for relay in self.relays.lock().unwrap().drain(..) {
            relay.abort();
        }
        let _ = self.pc.close().await;
    }
}

/// The candidates to send a client for one local ICE event. Normally just
/// the event's own; with `public_ips` set, a host candidate (whose address
/// is only meaningful inside the SFU's network) is replaced by one per
/// public address, and every other kind goes through untouched.
fn encode_candidates(event: &RTCPeerConnectionIceEvent, public_ips: &[String]) -> Vec<String> {
    let Ok(init) = event.candidate.to_json() else {
        return Vec::new();
    };
    let inits = match rewrite_host_candidate(&init, public_ips) {
        Some(rewritten) => rewritten,
        None => vec![init],
    };
    inits
        .iter()
        .filter_map(|init| serde_json::to_string(init).ok())
        .collect()
}

/// `candidate:<foundation> <component> <transport> <priority> <address>
/// <port> typ host ...` with the address swapped for each of `public_ips`.
/// `None` when there is nothing to rewrite (no public IPs, or not a host
/// candidate).
fn rewrite_host_candidate(
    init: &RTCIceCandidateInit,
    public_ips: &[String],
) -> Option<Vec<RTCIceCandidateInit>> {
    if public_ips.is_empty() {
        return None;
    }
    let fields: Vec<&str> = init.candidate.split_whitespace().collect();
    if fields.len() < 8 || fields[6] != "typ" || fields[7] != "host" {
        return None;
    }
    Some(
        public_ips
            .iter()
            .map(|ip| {
                let mut fields = fields.clone();
                fields[4] = ip;
                RTCIceCandidateInit {
                    candidate: fields.join(" "),
                    ..init.clone()
                }
            })
            .collect(),
    )
}

struct PublisherPcHandler {
    events: Arc<dyn MediaEventSink>,
    public_ips: Arc<[String]>,
}

#[async_trait]
impl PeerConnectionEventHandler for PublisherPcHandler {
    async fn on_ice_candidate(&self, event: RTCPeerConnectionIceEvent) {
        for candidate in encode_candidates(&event, &self.public_ips) {
            self.events
                .emit(MediaEvent::LocalIceCandidate { candidate });
        }
    }

    async fn on_track(&self, track: Arc<dyn TrackRemote>) {
        let events = self.events.clone();
        tokio::spawn(async move {
            if let Some(track) = PublishedTrack::start(track).await {
                events.emit(MediaEvent::TrackPublished { track });
            }
        });
    }
}

struct SubscriberPcHandler {
    events: Arc<dyn MediaEventSink>,
    public_ips: Arc<[String]>,
}

#[async_trait]
impl PeerConnectionEventHandler for SubscriberPcHandler {
    async fn on_ice_candidate(&self, event: RTCPeerConnectionIceEvent) {
        for candidate in encode_candidates(&event, &self.public_ips) {
            self.events
                .emit(MediaEvent::LocalIceCandidate { candidate });
        }
    }

    async fn on_connection_state_change(&self, state: RTCPeerConnectionState) {
        tracing::debug!("subscriber pairing state: {state}");
        if state == RTCPeerConnectionState::Connected {
            self.events.emit(MediaEvent::Connected);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn init(candidate: &str) -> RTCIceCandidateInit {
        RTCIceCandidateInit {
            candidate: candidate.to_string(),
            sdp_mid: Some("0".to_string()),
            ..Default::default()
        }
    }

    #[test]
    fn host_candidate_gets_each_public_ip_and_keeps_the_rest() {
        let host = init("candidate:1 1 udp 2130706431 172.28.0.2 50000 typ host");
        let ips = vec!["192.168.1.5".to_string(), "10.0.0.9".to_string()];

        let out = rewrite_host_candidate(&host, &ips).unwrap();

        assert_eq!(out.len(), 2);
        assert_eq!(
            out[0].candidate,
            "candidate:1 1 udp 2130706431 192.168.1.5 50000 typ host"
        );
        assert_eq!(
            out[1].candidate,
            "candidate:1 1 udp 2130706431 10.0.0.9 50000 typ host"
        );
        assert_eq!(out[0].sdp_mid, host.sdp_mid);
    }

    #[test]
    fn other_candidates_and_empty_config_are_left_alone() {
        let host = init("candidate:1 1 udp 2130706431 172.28.0.2 50000 typ host");
        let srflx = init(
            "candidate:2 1 udp 1694498815 1.2.3.4 50000 typ srflx raddr 172.28.0.2 rport 50000",
        );
        let ips = vec!["192.168.1.5".to_string()];

        assert!(rewrite_host_candidate(&host, &[]).is_none());
        assert!(rewrite_host_candidate(&srflx, &ips).is_none());
        assert!(rewrite_host_candidate(&init(""), &ips).is_none());
    }
}
