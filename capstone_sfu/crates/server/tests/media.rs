//! End-to-end media tests: real `webrtc-rs` clients publish and subscribe
//! through the SFU over its public websocket, and RTP must actually arrive.

use std::sync::Arc;
use std::time::Duration;

use common::signaling::{ClientMessage, ConnectionQuality, PeerId, ServerMessage, SimulcastLayer};
use futures_util::{SinkExt, StreamExt};
use rtc::media_stream::MediaStreamTrack;
use rtc::rtp;
use rtc::rtp_transceiver::rtp_sender::{
    RTCRtpCodec, RTCRtpCodingParameters, RTCRtpEncodingParameters, RtpCodecKind,
};
use server::config::Config;
use tokio::net::TcpListener;
use tokio::sync::{mpsc, Mutex};
use tokio::time::timeout;
use tokio_tungstenite::tungstenite::Message as WsMessage;
use webrtc::media_stream::track_local::static_rtp::TrackLocalStaticRTP;
use webrtc::media_stream::track_local::TrackLocal;
use webrtc::media_stream::track_remote::{TrackRemote, TrackRemoteEvent};
use webrtc::peer_connection::{
    register_default_interceptors, MediaEngine, PeerConnection, PeerConnectionBuilder,
    PeerConnectionEventHandler, RTCIceCandidateInit, RTCPeerConnectionIceEvent,
    RTCSessionDescription, Registry,
};

const VIDEO_SSRC: u32 = 1111;
const AUDIO_SSRC: u32 = 2222;
const WAIT: Duration = Duration::from_secs(20);

async fn spawn_app() -> String {
    let app = server::app(Config {
        bind_addr: "127.0.0.1:0".to_string(),
        udp_ports: None,
        max_participants: None,
        public_ips: vec![],
        ice_servers: vec![],
    });
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(async move { axum::serve(listener, app).await.unwrap() });
    format!("ws://{addr}/ws")
}

/// A signaling connection: outbound messages go through `tx`, inbound ones
/// arrive on `rx`.
struct Signaling {
    tx: mpsc::UnboundedSender<ClientMessage>,
    rx: mpsc::UnboundedReceiver<ServerMessage>,
}

async fn connect(ws_url: &str) -> Signaling {
    let (ws, _) = tokio_tungstenite::connect_async(ws_url).await.unwrap();
    let (mut sink, mut stream) = ws.split();
    let (out_tx, mut out_rx) = mpsc::unbounded_channel::<ClientMessage>();
    let (in_tx, in_rx) = mpsc::unbounded_channel::<ServerMessage>();
    tokio::spawn(async move {
        while let Some(msg) = out_rx.recv().await {
            let text = serde_json::to_string(&msg).unwrap();
            if sink.send(WsMessage::Text(text)).await.is_err() {
                break;
            }
        }
        let _ = sink.close().await;
    });
    tokio::spawn(async move {
        while let Some(Ok(msg)) = stream.next().await {
            if let WsMessage::Text(text) = msg {
                let _ = in_tx.send(serde_json::from_str(&text).unwrap());
            }
        }
    });
    Signaling {
        tx: out_tx,
        rx: in_rx,
    }
}

async fn join(sig: &mut Signaling, msg: ClientMessage) -> (PeerId, Vec<PeerId>) {
    sig.tx.send(msg).unwrap();
    match timeout(WAIT, sig.rx.recv()).await.unwrap().unwrap() {
        ServerMessage::Joined {
            peer_id,
            publishers,
            ..
        } => (peer_id, publishers),
        other => panic!("expected Joined, got {other:?}"),
    }
}

/// Forwards locally gathered candidates to the SFU and records remote tracks.
struct ClientHandler {
    tx: mpsc::UnboundedSender<ClientMessage>,
    target_peer: Option<PeerId>,
    tracks: mpsc::UnboundedSender<Arc<dyn TrackRemote>>,
}

#[async_trait::async_trait]
impl PeerConnectionEventHandler for ClientHandler {
    async fn on_ice_candidate(&self, event: RTCPeerConnectionIceEvent) {
        let init = event.candidate.to_json().unwrap();
        let _ = self.tx.send(ClientMessage::IceCandidate {
            target_peer: self.target_peer.clone(),
            candidate: serde_json::to_string(&init).unwrap(),
        });
    }

    async fn on_track(&self, track: Arc<dyn TrackRemote>) {
        let _ = self.tracks.send(track);
    }
}

async fn client_pc(
    sig: &Signaling,
    target_peer: Option<PeerId>,
) -> (
    Arc<dyn PeerConnection>,
    mpsc::UnboundedReceiver<Arc<dyn TrackRemote>>,
) {
    let (tracks_tx, tracks_rx) = mpsc::unbounded_channel();
    let mut media_engine = MediaEngine::default();
    media_engine.register_default_codecs().unwrap();
    let registry = register_default_interceptors(Registry::new(), &mut media_engine).unwrap();
    let pc = PeerConnectionBuilder::new()
        .with_media_engine(media_engine)
        .with_interceptor_registry(registry)
        .with_handler(Arc::new(ClientHandler {
            tx: sig.tx.clone(),
            target_peer,
            tracks: tracks_tx,
        }))
        .with_udp_addrs(vec!["0.0.0.0:0"])
        .build()
        .await
        .unwrap();
    (Arc::new(pc), tracks_rx)
}

fn local_track(
    kind: RtpCodecKind,
    ssrc: u32,
    mime: &str,
    clock_rate: u32,
    channels: u16,
) -> Arc<TrackLocalStaticRTP> {
    Arc::new(TrackLocalStaticRTP::new(MediaStreamTrack::new(
        "publisher-stream".to_string(),
        format!("track-{ssrc}"),
        format!("track-{ssrc}"),
        kind,
        vec![RTCRtpEncodingParameters {
            rtp_coding_parameters: RTCRtpCodingParameters {
                ssrc: Some(ssrc),
                ..Default::default()
            },
            codec: RTCRtpCodec {
                mime_type: mime.to_string(),
                clock_rate,
                channels,
                ..Default::default()
            },
            ..Default::default()
        }],
    )))
}

struct Publisher {
    id: PeerId,
    tx: mpsc::UnboundedSender<ClientMessage>,
    sig: Arc<Mutex<Signaling>>,
    pc: Arc<dyn PeerConnection>,
    pump: tokio::task::JoinHandle<()>,
}

/// Joins as publisher; if `media` is true, negotiates audio+video with the
/// SFU and starts writing synthetic RTP.
async fn start_publisher(ws_url: &str, room: &str, media: bool) -> Publisher {
    let mut sig = connect(ws_url).await;
    let (id, _) = join(
        &mut sig,
        ClientMessage::JoinAsPublisher {
            room: room.to_string(),
        },
    )
    .await;
    let (pc, _) = client_pc(&sig, None).await;

    if !media {
        return Publisher {
            id,
            tx: sig.tx.clone(),
            sig: Arc::new(Mutex::new(sig)),
            pc,
            pump: tokio::spawn(async {}),
        };
    }

    let (video, audio) = negotiate_media(&pc, &mut sig).await;
    let tx = sig.tx.clone();
    let sig = Arc::new(Mutex::new(sig));
    spawn_candidate_pump(sig.clone(), pc.clone(), None);
    let pump = spawn_rtp_pump(video, audio);

    Publisher {
        id,
        tx,
        sig,
        pc,
        pump,
    }
}

/// Sends the publisher's offer before applying it locally: applying it
/// starts ICE gathering, and a candidate must never reach the SFU ahead of
/// the offer it belongs to.
async fn send_offer(pc: &Arc<dyn PeerConnection>, sig: &Signaling) {
    let offer = pc.create_offer(None).await.unwrap();
    sig.tx
        .send(ClientMessage::SdpOffer {
            target_peer: None,
            sdp: offer.sdp.clone(),
        })
        .unwrap();
    pc.set_local_description(offer).await.unwrap();
}

/// Adds audio+video to the publisher's `pc` and completes the offer/answer
/// round with the SFU, checking the answer is real SDP.
async fn negotiate_media(
    pc: &Arc<dyn PeerConnection>,
    sig: &mut Signaling,
) -> (Arc<TrackLocalStaticRTP>, Arc<TrackLocalStaticRTP>) {
    let video = local_track(RtpCodecKind::Video, VIDEO_SSRC, "video/VP8", 90000, 0);
    let audio = local_track(RtpCodecKind::Audio, AUDIO_SSRC, "audio/opus", 48000, 2);
    pc.add_track(video.clone() as Arc<dyn TrackLocal>)
        .await
        .unwrap();
    pc.add_track(audio.clone() as Arc<dyn TrackLocal>)
        .await
        .unwrap();

    send_offer(pc, sig).await;

    // The SFU always sends its answer before any of its own candidates.
    match timeout(WAIT, sig.rx.recv()).await.unwrap().unwrap() {
        ServerMessage::SdpAnswer { from_peer, sdp } => {
            assert_eq!(from_peer, None);
            assert!(
                sdp.contains("m=video") && sdp.contains("m=audio"),
                "real SDP answer expected, got:\n{sdp}"
            );
            pc.set_remote_description(RTCSessionDescription::answer(sdp).unwrap())
                .await
                .unwrap();
        }
        other => panic!("expected SdpAnswer, got {other:?}"),
    }
    (video, audio)
}

/// Writes synthetic RTP at ~50 packets/s per track. Payloads are tagged
/// `V`/`A` so the subscriber can check each arrived on the right track.
fn spawn_rtp_pump(
    video: Arc<TrackLocalStaticRTP>,
    audio: Arc<TrackLocalStaticRTP>,
) -> tokio::task::JoinHandle<()> {
    tokio::spawn(async move {
        let mut seq: u16 = 0;
        loop {
            seq = seq.wrapping_add(1);
            for (track, ssrc, pt, tag) in [
                (&video, VIDEO_SSRC, 96u8, b'V'),
                (&audio, AUDIO_SSRC, 111u8, b'A'),
            ] {
                let packet = rtp::Packet {
                    header: rtp::header::Header {
                        version: 2,
                        payload_type: pt,
                        sequence_number: seq,
                        timestamp: seq as u32 * 960,
                        ssrc,
                        marker: true,
                        ..Default::default()
                    },
                    payload: bytes::Bytes::from(vec![tag, (seq >> 8) as u8, seq as u8, 0xAB, 0xCD]),
                };
                let _ = track.write_rtp(packet).await;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    })
}

/// Applies SFU-sent ICE candidates for the given pairing to `pc`.
fn spawn_candidate_pump(
    sig: Arc<Mutex<Signaling>>,
    pc: Arc<dyn PeerConnection>,
    from: Option<PeerId>,
) {
    tokio::spawn(async move {
        loop {
            let msg = { sig.lock().await.rx.recv().await };
            match msg {
                Some(ServerMessage::IceCandidate {
                    from_peer,
                    candidate,
                }) if from_peer == from => {
                    let init: RTCIceCandidateInit = serde_json::from_str(&candidate).unwrap();
                    pc.add_ice_candidate(init).await.unwrap();
                }
                Some(_) => {}
                None => break,
            }
        }
    });
}

struct Subscriber {
    sig: mpsc::UnboundedSender<ClientMessage>,
    video_tag: mpsc::UnboundedReceiver<(RtpCodecKind, Vec<u8>)>,
    offers: mpsc::UnboundedReceiver<String>,
    events: mpsc::UnboundedReceiver<ServerMessage>,
}

/// Joins as subscriber, requests `publisher`'s track and behaves like the
/// browser client: answers every `sdp_offer` for that pairing (including
/// renegotiations) and reports every received RTP payload by kind.
async fn start_subscriber(
    ws_url: &str,
    room: &str,
    publisher: Option<&PeerId>,
) -> (PeerId, Vec<PeerId>, Subscriber) {
    let mut sig = connect(ws_url).await;
    let (id, publishers) = join(
        &mut sig,
        ClientMessage::JoinAsSubscriber {
            room: room.to_string(),
        },
    )
    .await;
    let target = publisher.cloned().unwrap_or_else(|| publishers[0].clone());
    let (pc, mut tracks) = client_pc(&sig, Some(target.clone())).await;

    let (payload_tx, payload_rx) = mpsc::unbounded_channel();
    tokio::spawn(async move {
        while let Some(track) = tracks.recv().await {
            let payload_tx = payload_tx.clone();
            tokio::spawn(async move {
                let kind = track.kind().await;
                while let Some(evt) = track.poll().await {
                    if let TrackRemoteEvent::OnRtpPacket(p) = evt {
                        let _ = payload_tx.send((kind, p.payload.to_vec()));
                    }
                }
            });
        }
    });

    let (offers_tx, offers_rx) = mpsc::unbounded_channel();
    let (events_tx, events_rx) = mpsc::unbounded_channel();
    let tx = sig.tx.clone();
    let mut rx = sig.rx;
    let pair_target = target.clone();
    tokio::spawn(async move {
        while let Some(msg) = rx.recv().await {
            match msg {
                ServerMessage::SdpOffer { from_peer, sdp }
                    if from_peer.as_ref() == Some(&pair_target) =>
                {
                    let _ = offers_tx.send(sdp.clone());
                    pc.set_remote_description(RTCSessionDescription::offer(sdp).unwrap())
                        .await
                        .unwrap();
                    let answer = pc.create_answer(None).await.unwrap();
                    pc.set_local_description(answer).await.unwrap();
                    let sdp = pc.local_description().await.unwrap().sdp;
                    let _ = tx.send(ClientMessage::SdpAnswer {
                        target_peer: pair_target.clone(),
                        sdp,
                    });
                }
                ServerMessage::IceCandidate {
                    from_peer,
                    candidate,
                } if from_peer.as_ref() == Some(&pair_target) => {
                    let init: RTCIceCandidateInit = serde_json::from_str(&candidate).unwrap();
                    pc.add_ice_candidate(init).await.unwrap();
                }
                other => {
                    let _ = events_tx.send(other);
                }
            }
        }
        let _ = pc.close().await;
    });

    sig.tx
        .send(ClientMessage::RequestTrack {
            target_peer: target,
        })
        .unwrap();

    (
        id,
        publishers,
        Subscriber {
            sig: sig.tx,
            video_tag: payload_rx,
            offers: offers_rx,
            events: events_rx,
        },
    )
}

/// Waits until both an audio and a video payload from the publisher arrive.
async fn expect_audio_and_video(sub: &mut Subscriber) {
    let (mut audio, mut video) = (false, false);
    timeout(WAIT, async {
        while !(audio && video) {
            let (kind, payload) = sub.video_tag.recv().await.expect("payload channel closed");
            assert_eq!(
                &payload[3..],
                &[0xAB, 0xCD],
                "payload must be relayed untouched"
            );
            match (kind, payload[0]) {
                (RtpCodecKind::Video, b'V') => video = true,
                (RtpCodecKind::Audio, b'A') => audio = true,
                other => panic!("payload on wrong track: {other:?}"),
            }
        }
    })
    .await
    .expect("timed out waiting for relayed audio+video RTP");
}

#[tokio::test(flavor = "multi_thread")]
async fn subscriber_receives_publisher_media_after_it_is_flowing() {
    let ws_url = spawn_app().await;
    let publisher = start_publisher(&ws_url, "media-1", true).await;
    tokio::time::sleep(Duration::from_millis(500)).await;

    let (_, publishers, mut sub) = start_subscriber(&ws_url, "media-1", None).await;
    assert_eq!(publishers, vec![publisher.id.clone()]);

    let first_offer = timeout(WAIT, sub.offers.recv()).await.unwrap().unwrap();
    assert!(first_offer.contains("m=video") && first_offer.contains("m=audio"));
    expect_audio_and_video(&mut sub).await;
}

#[tokio::test(flavor = "multi_thread")]
async fn subscriber_that_requests_before_publisher_media_is_offered_once_it_arrives() {
    let ws_url = spawn_app().await;
    // Publisher joins but hasn't negotiated any media yet.
    let mut publisher = start_publisher(&ws_url, "media-2", false).await;
    let (_, _, mut sub) = start_subscriber(&ws_url, "media-2", Some(&publisher.id)).await;
    // Nothing to offer yet: the SFU holds the offer back rather than sending
    // one with no m-lines.
    assert!(timeout(Duration::from_millis(500), sub.offers.recv())
        .await
        .is_err());

    // Now the same publisher connection negotiates real media.
    let (video, audio) = negotiate_media(&publisher.pc, &mut *publisher.sig.lock().await).await;
    spawn_candidate_pump(publisher.sig.clone(), publisher.pc.clone(), None);
    publisher.pump.abort();
    publisher.pump = spawn_rtp_pump(video, audio);

    // Audio and video arrive independently, so the pairing may be offered
    // one track first and renegotiated to add the other.
    let renegotiated = timeout(WAIT, async {
        loop {
            let offer = sub.offers.recv().await.unwrap();
            if offer.contains("m=video") && offer.contains("m=audio") {
                return offer;
            }
        }
    })
    .await
    .expect("SFU never offered the publisher's audio+video tracks");
    assert!(renegotiated.contains("a=msid:"));
    expect_audio_and_video(&mut sub).await;
}

#[tokio::test(flavor = "multi_thread")]
async fn media_fans_out_to_multiple_subscribers_and_stops_on_publisher_leave() {
    let ws_url = spawn_app().await;
    let publisher = start_publisher(&ws_url, "media-3", true).await;

    let (_, _, mut sub_a) = start_subscriber(&ws_url, "media-3", Some(&publisher.id)).await;
    let (_, _, mut sub_b) = start_subscriber(&ws_url, "media-3", Some(&publisher.id)).await;
    expect_audio_and_video(&mut sub_a).await;
    expect_audio_and_video(&mut sub_b).await;

    publisher.pump.abort();
    publisher.tx.send(ClientMessage::Leave {}).unwrap();
    let _ = publisher.pc.close().await;

    for sub in [&mut sub_a, &mut sub_b] {
        let left = timeout(WAIT, async {
            loop {
                if let ServerMessage::PublisherLeft { peer_id } = sub.events.recv().await.unwrap() {
                    return peer_id;
                }
            }
        })
        .await
        .unwrap();
        assert_eq!(left, publisher.id);
    }
    // Keep the subscriber sockets open until the end of the test.
    drop((sub_a.sig, sub_b.sig));
}

#[tokio::test(flavor = "multi_thread")]
async fn malformed_sdp_offer_is_rejected_with_error() {
    let ws_url = spawn_app().await;
    let mut sig = connect(&ws_url).await;
    join(
        &mut sig,
        ClientMessage::JoinAsPublisher {
            room: "media-4".to_string(),
        },
    )
    .await;
    sig.tx
        .send(ClientMessage::SdpOffer {
            target_peer: None,
            sdp: "not sdp".to_string(),
        })
        .unwrap();
    match timeout(WAIT, sig.rx.recv()).await.unwrap().unwrap() {
        ServerMessage::Error { reason, .. } => {
            assert!(reason.contains("invalid sdp_offer"), "{reason}")
        }
        other => panic!("expected Error, got {other:?}"),
    }
}

const SIMULCAST_LAYERS: [(&str, u32); 3] = [("h", 3331), ("m", 3332), ("l", 3333)];

/// The negotiated id of header extension `uri`, from an `a=extmap` line.
fn extmap_id(sdp: &str, uri: &str) -> u8 {
    sdp.lines()
        .filter_map(|line| line.strip_prefix("a=extmap:"))
        .find_map(|rest| {
            let (id, ext_uri) = rest.split_once(' ')?;
            (ext_uri.trim() == uri).then(|| id.split('/').next()?.parse().ok())?
        })
        .unwrap_or_else(|| panic!("{uri} not negotiated:\n{sdp}"))
}

/// Joins as publisher and sends one VP8 video track as three simulcast
/// layers. Every packet is a VP8 keyframe whose third byte is the layer's
/// rid, so the subscriber can tell which layer it's being forwarded.
async fn start_simulcast_publisher(ws_url: &str, room: &str) -> Publisher {
    let mut sig = connect(ws_url).await;
    let (id, _) = join(
        &mut sig,
        ClientMessage::JoinAsPublisher {
            room: room.to_string(),
        },
    )
    .await;
    let (pc, _) = client_pc(&sig, None).await;

    let video = Arc::new(TrackLocalStaticRTP::new(MediaStreamTrack::new(
        "publisher-stream".to_string(),
        "simulcast-video".to_string(),
        "simulcast-video".to_string(),
        RtpCodecKind::Video,
        SIMULCAST_LAYERS
            .iter()
            .map(|(rid, ssrc)| RTCRtpEncodingParameters {
                rtp_coding_parameters: RTCRtpCodingParameters {
                    rid: rid.to_string(),
                    ssrc: Some(*ssrc),
                    ..Default::default()
                },
                codec: RTCRtpCodec {
                    mime_type: "video/VP8".to_string(),
                    clock_rate: 90000,
                    ..Default::default()
                },
                ..Default::default()
            })
            .collect(),
    )));
    pc.add_track(video.clone() as Arc<dyn TrackLocal>)
        .await
        .unwrap();

    send_offer(&pc, &sig).await;
    let answer = match timeout(WAIT, sig.rx.recv()).await.unwrap().unwrap() {
        ServerMessage::SdpAnswer { sdp, .. } => sdp,
        other => panic!("expected SdpAnswer, got {other:?}"),
    };
    assert!(
        answer.contains("a=simulcast:recv"),
        "SFU must accept simulcast:\n{answer}"
    );
    // Like a browser, tag every packet with its mid and rid: that's how
    // the SFU tells simulcast layers apart.
    let mid_id = extmap_id(&answer, "urn:ietf:params:rtp-hdrext:sdes:mid");
    let rid_id = extmap_id(&answer, "urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id");
    let mid = pc
        .local_description()
        .await
        .unwrap()
        .sdp
        .lines()
        .find_map(|line| line.strip_prefix("a=mid:").map(str::to_string))
        .unwrap();
    pc.set_remote_description(RTCSessionDescription::answer(answer).unwrap())
        .await
        .unwrap();

    let tx = sig.tx.clone();
    let sig = Arc::new(Mutex::new(sig));
    spawn_candidate_pump(sig.clone(), pc.clone(), None);
    let pump = tokio::spawn(async move {
        let mut seq: u16 = 0;
        loop {
            seq = seq.wrapping_add(1);
            for (rid, ssrc) in SIMULCAST_LAYERS {
                let mut packet = rtp::Packet {
                    header: rtp::header::Header {
                        version: 2,
                        payload_type: 96,
                        sequence_number: seq,
                        timestamp: seq as u32 * 3000,
                        ssrc,
                        marker: true,
                        ..Default::default()
                    },
                    payload: bytes::Bytes::from(vec![0x10, 0x00, rid.as_bytes()[0]]),
                };
                let tags = [(mid_id, mid.clone()), (rid_id, rid.to_string())];
                for (id, value) in tags {
                    packet.header.set_extension(id, value.into()).unwrap();
                }
                let _ = video.write_rtp(packet).await;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
    });

    Publisher {
        id,
        tx,
        sig,
        pc,
        pump,
    }
}

/// Waits for a video payload from the given simulcast layer.
async fn expect_layer(sub: &mut Subscriber, rid: u8) {
    timeout(WAIT, async {
        loop {
            let (kind, payload) = sub.video_tag.recv().await.expect("payload channel closed");
            if kind == RtpCodecKind::Video && payload.get(2) == Some(&rid) {
                return;
            }
        }
    })
    .await
    .unwrap_or_else(|_| panic!("timed out waiting for layer {}", rid as char));
}

#[tokio::test(flavor = "multi_thread")]
async fn low_bandwidth_subscriber_is_switched_to_low_layer_and_can_retry_high() {
    let ws_url = spawn_app().await;
    let publisher = start_simulcast_publisher(&ws_url, "media-5").await;
    let (_, _, mut sub) = start_subscriber(&ws_url, "media-5", Some(&publisher.id)).await;

    expect_layer(&mut sub, b'h').await;

    for _ in 0..2 {
        sub.sig
            .send(ClientMessage::ConnectionStats {
                target_peer: publisher.id.clone(),
                available_incoming_bitrate_bps: Some(300_000),
                packet_loss_pct: 15.0,
                round_trip_time_ms: None,
                jitter_ms: None,
            })
            .unwrap();
    }
    let quality = timeout(WAIT, async {
        loop {
            if let ServerMessage::ConnectionQuality { quality, layer, .. } =
                sub.events.recv().await.unwrap()
            {
                return (quality, layer);
            }
        }
    })
    .await
    .expect("no connection_quality received");
    assert_eq!(quality, (ConnectionQuality::Bad, SimulcastLayer::Low));

    expect_layer(&mut sub, b'l').await;
    // Once switched, only the low layer keeps arriving.
    for _ in 0..20 {
        let (_, payload) = timeout(WAIT, sub.video_tag.recv()).await.unwrap().unwrap();
        assert_eq!(payload[2], b'l', "higher layer leaked after switch");
    }

    // The subscriber retries by picking high again.
    sub.sig
        .send(ClientMessage::SetPreferredLayer {
            target_peer: publisher.id.clone(),
            layer: SimulcastLayer::High,
        })
        .unwrap();
    let quality = timeout(WAIT, async {
        loop {
            if let ServerMessage::ConnectionQuality { quality, layer, .. } =
                sub.events.recv().await.unwrap()
            {
                return (quality, layer);
            }
        }
    })
    .await
    .expect("set_preferred_layer was not acknowledged");
    assert_eq!(quality, (ConnectionQuality::Good, SimulcastLayer::High));
    expect_layer(&mut sub, b'h').await;
    drop(publisher.tx);
}
