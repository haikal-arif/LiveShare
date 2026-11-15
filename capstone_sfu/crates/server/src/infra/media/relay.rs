use std::sync::Arc;
use std::time::{Duration, Instant};

use common::signaling::SimulcastLayer;
use rtc::media_stream::MediaStreamTrack;
use rtc::rtp_transceiver::rtp_sender::{
    RTCRtpCodec, RTCRtpCodingParameters, RTCRtpEncodingParameters,
};
use tokio::sync::{broadcast, watch};
use webrtc::media_stream::track_local::static_rtp::TrackLocalStaticRTP;
use webrtc::media_stream::track_local::{TrackLocal, TrackLocalEvent};
use webrtc::rtp_transceiver::RtpSender;

use super::engine::SharedPeerConnection;
use super::keyframe::{is_keyframe, is_keyframe_request};
use super::layer_switcher::LayerSwitcher;
use super::published_track::{LayerPacket, PublishedTrack};

/// How often a relay re-checks which layer it should forward, and how long
/// a pending layer switch waits before asking for another keyframe.
const LAYER_CHECK_INTERVAL: Duration = Duration::from_millis(250);

/// Adds `track` to a subscriber pairing's `PeerConnection` and spawns the
/// task relaying its packets, following the pairing's `target` layer. The
/// caller must renegotiate afterwards.
pub async fn attach_track(
    pc: &SharedPeerConnection,
    track: &Arc<PublishedTrack>,
    stream_id: &str,
    target: watch::Receiver<SimulcastLayer>,
) -> webrtc::error::Result<tokio::task::JoinHandle<()>> {
    let ssrc = rand_u32();
    let local = Arc::new(TrackLocalStaticRTP::new(MediaStreamTrack::new(
        stream_id.to_string(),
        format!("{stream_id}-{}", ssrc),
        stream_id.to_string(),
        track.kind,
        vec![RTCRtpEncodingParameters {
            rtp_coding_parameters: RTCRtpCodingParameters {
                ssrc: Some(ssrc),
                ..Default::default()
            },
            codec: track.codec.clone(),
            ..Default::default()
        }],
    )));
    let sender = pc.add_track(local.clone() as Arc<dyn TrackLocal>).await?;

    let rx = track.packets.subscribe();
    let track = track.clone();
    Ok(tokio::spawn(async move {
        tokio::join!(
            relay(rx, local.clone(), sender, ssrc, track.clone(), target),
            forward_keyframe_requests(local, track),
        );
    }))
}

/// Relays the subscriber's own PLI/FIR for this track to the publisher, so a
/// subscriber that missed a keyframe can recover.
async fn forward_keyframe_requests(local: Arc<TrackLocalStaticRTP>, track: Arc<PublishedTrack>) {
    loop {
        match local.poll().await {
            Some(TrackLocalEvent::OnRtcpPacket(packets)) => {
                if packets.iter().any(|p| is_keyframe_request(p.as_ref())) {
                    track.request_keyframe().await;
                }
            }
            Some(_) => {}
            // Not bound to a negotiated sender yet (or between renegotiations).
            None => tokio::time::sleep(Duration::from_millis(250)).await,
        }
    }
}

async fn relay(
    mut rx: broadcast::Receiver<LayerPacket>,
    local: Arc<TrackLocalStaticRTP>,
    sender: Arc<dyn RtpSender>,
    ssrc: u32,
    track: Arc<PublishedTrack>,
    mut target: watch::Receiver<SimulcastLayer>,
) {
    let mut switcher = LayerSwitcher::new(track.codec.clock_rate);
    let mut payload_type = None;
    let mut check = tokio::time::interval(LAYER_CHECK_INTERVAL);
    let mut target_open = true;

    let retarget = |switcher: &mut LayerSwitcher, target: SimulcastLayer| {
        track
            .effective_layer(target)
            .filter(|layer| switcher.set_target(*layer, Instant::now()))
    };
    let mut want = retarget(&mut switcher, *target.borrow());

    loop {
        tokio::select! {
            received = rx.recv() => {
                let LayerPacket { layer, mut packet } = match received {
                    Ok(packet) => packet,
                    Err(broadcast::error::RecvError::Lagged(_)) => continue,
                    Err(broadcast::error::RecvError::Closed) => break,
                };
                // The payload type is only known once this leg has been negotiated.
                if payload_type.is_none() {
                    payload_type = negotiated_payload_type(&sender, &track.codec).await;
                }
                let Some(pt) = payload_type else { continue };

                let keyframe = is_keyframe(&track.codec.mime_type, &packet.payload);
                if !switcher.admit(layer, &mut packet.header, keyframe, Instant::now()) {
                    continue;
                }
                packet.header.ssrc = ssrc;
                packet.header.payload_type = pt;
                // Extension ids are negotiated per leg; the publisher's don't apply here.
                packet.header.extension = false;
                packet.header.extensions.clear();
                packet.header.extensions_padding = 0;
                // Errors here just mean "not bound yet" or "closing": drop and move on.
                let _ = local.write_rtp(packet).await;
            }
            changed = target.changed(), if target_open => {
                target_open = changed.is_ok();
                want = retarget(&mut switcher, *target.borrow());
            }
            _ = check.tick() => {
                // Layers come and go on the publisher side too.
                want = retarget(&mut switcher, *target.borrow());
                if want.is_none() && switcher.keyframe_overdue(Instant::now()) {
                    want = switcher.target;
                }
            }
        }
        if let Some(layer) = want.take() {
            track.request_layer_keyframe(layer).await;
        }
    }
}

async fn negotiated_payload_type(sender: &Arc<dyn RtpSender>, codec: &RTCRtpCodec) -> Option<u8> {
    let params = sender.get_parameters().await.ok()?;
    let codecs = &params.rtp_parameters.codecs;
    let same_codec = |c: &&rtc::rtp_transceiver::rtp_sender::RTCRtpCodecParameters| {
        c.rtp_codec.mime_type.eq_ignore_ascii_case(&codec.mime_type)
            && c.rtp_codec.clock_rate == codec.clock_rate
    };
    codecs
        .iter()
        .filter(same_codec)
        .find(|c| c.rtp_codec.sdp_fmtp_line == codec.sdp_fmtp_line)
        .or_else(|| codecs.iter().find(same_codec))
        .map(|c| c.payload_type)
}

fn rand_u32() -> u32 {
    uuid::Uuid::new_v4().as_u128() as u32
}
