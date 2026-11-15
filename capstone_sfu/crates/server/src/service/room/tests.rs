//! Room logic against a fake media engine: no WebRTC, just the signaling
//! rules the room owns.

use std::any::Any;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use async_trait::async_trait;
use common::signaling::{ErrorCode, PeerId, ServerMessage, SimulcastLayer};
use tokio::sync::{mpsc, oneshot, watch};

use super::{Room, RoomCommand};
use crate::domain::peer::{ConnectionHandle, Outbound, PeerRole};
use crate::domain::ports::{
    MediaError, MediaEvent, MediaEventSink, MediaGateway, MediaSession, MediaTrack,
};

struct FakeTrack;

#[async_trait]
impl MediaTrack for FakeTrack {
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

#[derive(Default)]
struct FakeGateway {
    fail_new_session: bool,
    offer_error: Option<MediaError>,
    ice_error: Option<MediaError>,
    answer_error: Option<MediaError>,
    create_offer_error: Option<MediaError>,
    attach_error: Option<MediaError>,
    /// The sink of every publisher session, so a test can emit events as the engine would.
    publisher_sinks: Mutex<Vec<Arc<dyn MediaEventSink>>>,
    /// `stream_id` of every track attached to a subscriber session.
    attached: Arc<Mutex<Vec<String>>>,
}

struct FakeSession {
    offer_error: Option<MediaError>,
    ice_error: Option<MediaError>,
    answer_error: Option<MediaError>,
    create_offer_error: Option<MediaError>,
    attach_error: Option<MediaError>,
    attached: Arc<Mutex<Vec<String>>>,
}

#[async_trait]
impl MediaGateway for FakeGateway {
    async fn new_publisher_session(
        &self,
        events: Arc<dyn MediaEventSink>,
    ) -> Result<Arc<dyn MediaSession>, MediaError> {
        if self.fail_new_session {
            return Err(MediaError::Engine("boom".into()));
        }
        self.publisher_sinks.lock().unwrap().push(events);
        Ok(Arc::new(FakeSession {
            offer_error: self.offer_error.clone(),
            ice_error: self.ice_error.clone(),
            answer_error: self.answer_error.clone(),
            create_offer_error: self.create_offer_error.clone(),
            attach_error: self.attach_error.clone(),
            attached: self.attached.clone(),
        }))
    }

    async fn new_subscriber_session(
        &self,
        _events: Arc<dyn MediaEventSink>,
    ) -> Result<Arc<dyn MediaSession>, MediaError> {
        if self.fail_new_session {
            return Err(MediaError::Engine("boom".into()));
        }
        Ok(Arc::new(FakeSession {
            offer_error: None,
            ice_error: None,
            answer_error: self.answer_error.clone(),
            create_offer_error: self.create_offer_error.clone(),
            attach_error: self.attach_error.clone(),
            attached: self.attached.clone(),
        }))
    }
}

#[async_trait]
impl MediaSession for FakeSession {
    async fn accept_offer(&self, _sdp: String) -> Result<String, MediaError> {
        match &self.offer_error {
            Some(err) => Err(err.clone()),
            None => Ok("fake-answer".to_string()),
        }
    }
    async fn create_offer(&self) -> Result<String, MediaError> {
        match &self.create_offer_error {
            Some(err) => Err(err.clone()),
            None => Ok("fake-offer".to_string()),
        }
    }
    async fn apply_answer(&self, _sdp: String) -> Result<(), MediaError> {
        match &self.answer_error {
            Some(err) => Err(err.clone()),
            None => Ok(()),
        }
    }
    async fn add_ice_candidate(&self, _candidate_json: &str) -> Result<(), MediaError> {
        match &self.ice_error {
            Some(err) => Err(err.clone()),
            None => Ok(()),
        }
    }
    async fn attach_track(
        &self,
        _track: &Arc<dyn MediaTrack>,
        stream_id: &str,
        _layer: watch::Receiver<SimulcastLayer>,
    ) -> Result<(), MediaError> {
        if let Some(err) = &self.attach_error {
            return Err(err.clone());
        }
        self.attached.lock().unwrap().push(stream_id.to_string());
        Ok(())
    }
    async fn close(&self) {}
}

const WAIT: Duration = Duration::from_millis(500);
const QUIET: Duration = Duration::from_millis(150);

struct TestCase {
    name: &'static str,
    gateway: fn() -> FakeGateway,
    check_result: fn(ServerMessage),
}

#[tokio::test]
async fn publisher_offer_outcomes() {
    let cases = [
        TestCase {
            name: "accepted offer gets the engine's answer",
            gateway: FakeGateway::default,
            check_result: |msg| match msg {
                ServerMessage::SdpAnswer { from_peer, sdp } => {
                    assert_eq!(from_peer, None);
                    assert_eq!(sdp, "fake-answer");
                }
                other => panic!("expected sdp_answer, got {other:?}"),
            },
        },
        TestCase {
            name: "rejected sdp is invalid_sdp",
            gateway: || FakeGateway {
                offer_error: Some(MediaError::InvalidSdp("bad sdp".into())),
                ..Default::default()
            },
            check_result: |msg| match msg {
                ServerMessage::Error { code, reason } => {
                    assert_eq!(code, ErrorCode::InvalidSdp);
                    assert_eq!(reason, "invalid sdp_offer: bad sdp");
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        TestCase {
            name: "engine failing to create a session is internal",
            gateway: || FakeGateway {
                fail_new_session: true,
                ..Default::default()
            },
            check_result: |msg| match msg {
                ServerMessage::Error { code, .. } => assert_eq!(code, ErrorCode::Internal),
                other => panic!("expected error, got {other:?}"),
            },
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let room = Room::spawn(vec![], Arc::new((case.gateway)()), None);
        let (out_tx, mut out_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Publisher,
            handle: ConnectionHandle {
                outbound: out_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let peer_id = joined.await.unwrap().unwrap();
        let first = tokio::time::timeout(WAIT, out_rx.recv()).await.unwrap();
        assert!(matches!(
            first,
            Some(Outbound::Message(ServerMessage::Joined { .. }))
        ));

        room.send(RoomCommand::SdpOffer {
            peer_id,
            target_peer: None,
            sdp: "offer".to_string(),
        })
        .await
        .unwrap();
        match tokio::time::timeout(WAIT, out_rx.recv()).await.unwrap() {
            Some(Outbound::Message(msg)) => (case.check_result)(msg),
            other => panic!("{}: expected a message, got {other:?}", case.name),
        }
    }
}

struct IceCase {
    name: &'static str,
    offer_first: bool,
    ice_error: Option<MediaError>,
    check_result: fn(Option<ServerMessage>),
}

#[tokio::test]
async fn publisher_ice_candidate_outcomes() {
    let cases = [
        IceCase {
            name: "before any offer is invalid_state",
            offer_first: false,
            ice_error: None,
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::InvalidState)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        IceCase {
            name: "malformed candidate is invalid_message",
            offer_first: true,
            ice_error: Some(MediaError::InvalidCandidate),
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, reason }) => {
                    assert_eq!(code, ErrorCode::InvalidMessage);
                    assert_eq!(reason, "invalid ice candidate");
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        IceCase {
            name: "engine failure adding a candidate is only logged",
            offer_first: true,
            ice_error: Some(MediaError::Engine("late".into())),
            check_result: |msg| assert!(msg.is_none(), "got {msg:?}"),
        },
        IceCase {
            name: "accepted candidate sends nothing back",
            offer_first: true,
            ice_error: None,
            check_result: |msg| assert!(msg.is_none(), "got {msg:?}"),
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let gateway = FakeGateway {
            ice_error: case.ice_error.clone(),
            ..Default::default()
        };
        let room = Room::spawn(vec![], Arc::new(gateway), None);
        let (out_tx, mut out_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Publisher,
            handle: ConnectionHandle {
                outbound: out_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let peer_id = joined.await.unwrap().unwrap();
        let _joined = tokio::time::timeout(WAIT, out_rx.recv()).await.unwrap();

        if case.offer_first {
            room.send(RoomCommand::SdpOffer {
                peer_id: peer_id.clone(),
                target_peer: None,
                sdp: "offer".to_string(),
            })
            .await
            .unwrap();
            let _answer = tokio::time::timeout(WAIT, out_rx.recv()).await.unwrap();
        }
        room.send(RoomCommand::IceCandidate {
            peer_id,
            target_peer: None,
            candidate: "{}".to_string(),
        })
        .await
        .unwrap();

        let msg = match tokio::time::timeout(QUIET, out_rx.recv()).await {
            Ok(Some(Outbound::Message(msg))) => Some(msg),
            Ok(other) => panic!("{}: unexpected {other:?}", case.name),
            Err(_) => None,
        };
        (case.check_result)(msg);
    }
}

struct DeferCase {
    name: &'static str,
    /// Whether the publisher's track exists before the subscriber asks for it.
    publish_before_request: bool,
}

/// The contract: a subscriber's offer is deferred (no error, no empty
/// offer) until the publisher has a live track, then sent without asking again.
#[tokio::test]
async fn subscriber_offer_waits_for_a_publisher_track() {
    let cases = [
        DeferCase {
            name: "track arrives after request_track",
            publish_before_request: false,
        },
        DeferCase {
            name: "track already there when request_track arrives",
            publish_before_request: true,
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let gateway = Arc::new(FakeGateway::default());
        let attached = gateway.attached.clone();
        let room = Room::spawn(vec![], gateway.clone(), None);

        let (pub_tx, mut pub_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Publisher,
            handle: ConnectionHandle {
                outbound: pub_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let publisher_id = joined.await.unwrap().unwrap();
        let _ = tokio::time::timeout(WAIT, pub_rx.recv()).await.unwrap();
        // The publisher's session only exists once it has sent an offer.
        room.send(RoomCommand::SdpOffer {
            peer_id: publisher_id.clone(),
            target_peer: None,
            sdp: "offer".to_string(),
        })
        .await
        .unwrap();
        let _answer = tokio::time::timeout(WAIT, pub_rx.recv()).await.unwrap();
        let sink = gateway.publisher_sinks.lock().unwrap()[0].clone();

        let (sub_tx, mut sub_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Subscriber,
            handle: ConnectionHandle {
                outbound: sub_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let subscriber_id = joined.await.unwrap().unwrap();
        let _ = tokio::time::timeout(WAIT, sub_rx.recv()).await.unwrap();

        if case.publish_before_request {
            sink.emit(MediaEvent::TrackPublished {
                track: Arc::new(FakeTrack),
            });
            tokio::time::sleep(QUIET).await;
        }
        room.send(RoomCommand::RequestTrack {
            peer_id: subscriber_id,
            target_peer: publisher_id.clone(),
        })
        .await
        .unwrap();

        if !case.publish_before_request {
            let early = tokio::time::timeout(QUIET, sub_rx.recv()).await;
            assert!(
                early.is_err(),
                "{}: offered with no track: {early:?}",
                case.name
            );
            sink.emit(MediaEvent::TrackPublished {
                track: Arc::new(FakeTrack),
            });
        }

        match tokio::time::timeout(WAIT, sub_rx.recv()).await.unwrap() {
            Some(Outbound::Message(ServerMessage::SdpOffer { from_peer, sdp })) => {
                assert_eq!(from_peer.as_deref(), Some(publisher_id.as_str()));
                assert_eq!(sdp, "fake-offer");
            }
            other => panic!("{}: expected sdp_offer, got {other:?}", case.name),
        }
        assert_eq!(
            attached.lock().unwrap().as_slice(),
            [publisher_id],
            "{}: track attached once, streamed under the publisher's id",
            case.name
        );
    }
}

struct ErrorCase {
    name: &'static str,
    /// Which joined peer sends the message; `Publisher` reuses the publisher.
    sender: PeerRole,
    gateway: fn() -> FakeGateway,
    /// Whether the subscriber requests the publisher's track first, which
    /// creates a pairing for the message to act on.
    request_first: bool,
    /// Builds the command from (sender id, publisher id).
    command: fn(&PeerId, &PeerId) -> RoomCommand,
    /// `None` when the room sends nothing back to the sender.
    check_result: fn(Option<ServerMessage>),
}

/// Requests that break the signaling rules get an error back, or are dropped
/// when the sender is unknown to the room.
#[tokio::test]
async fn invalid_requests_get_error_outcomes() {
    let cases = [
        ErrorCase {
            name: "subscriber sending an offer is wrong_role",
            sender: PeerRole::Subscriber,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, _| RoomCommand::SdpOffer {
                peer_id: sender.clone(),
                target_peer: None,
                sdp: "offer".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::WrongRole)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "publisher offer with a target_peer is invalid_message",
            sender: PeerRole::Publisher,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, _| RoomCommand::SdpOffer {
                peer_id: sender.clone(),
                target_peer: Some("sub-x".to_string()),
                sdp: "offer".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::InvalidMessage)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "publisher requesting a track is wrong_role",
            sender: PeerRole::Publisher,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, publisher| RoomCommand::RequestTrack {
                peer_id: sender.clone(),
                target_peer: publisher.clone(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::WrongRole)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "engine failing to create a subscriber session is internal",
            sender: PeerRole::Subscriber,
            gateway: || FakeGateway {
                fail_new_session: true,
                ..Default::default()
            },
            request_first: false,
            command: |sender, publisher| RoomCommand::RequestTrack {
                peer_id: sender.clone(),
                target_peer: publisher.clone(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => assert_eq!(code, ErrorCode::Internal),
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "publisher sending an answer is wrong_role",
            sender: PeerRole::Publisher,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, publisher| RoomCommand::SdpAnswer {
                peer_id: sender.clone(),
                target_peer: publisher.clone(),
                sdp: "answer".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::WrongRole)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "answer with no pending offer is invalid_state",
            sender: PeerRole::Subscriber,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, publisher| RoomCommand::SdpAnswer {
                peer_id: sender.clone(),
                target_peer: publisher.clone(),
                sdp: "answer".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::InvalidState)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "answer the engine rejects is invalid_sdp",
            sender: PeerRole::Subscriber,
            gateway: || FakeGateway {
                answer_error: Some(MediaError::InvalidSdp("bad sdp".into())),
                ..Default::default()
            },
            request_first: true,
            command: |sender, publisher| RoomCommand::SdpAnswer {
                peer_id: sender.clone(),
                target_peer: publisher.clone(),
                sdp: "answer".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, reason }) => {
                    assert_eq!(code, ErrorCode::InvalidSdp);
                    assert_eq!(reason, "invalid sdp_answer: bad sdp");
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "publisher ice candidate with a target_peer is invalid_message",
            sender: PeerRole::Publisher,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, _| RoomCommand::IceCandidate {
                peer_id: sender.clone(),
                target_peer: Some("sub-x".to_string()),
                candidate: "{}".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::InvalidMessage)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "subscriber ice candidate for an unknown publisher is peer_not_found",
            sender: PeerRole::Subscriber,
            gateway: FakeGateway::default,
            request_first: false,
            command: |sender, _| RoomCommand::IceCandidate {
                peer_id: sender.clone(),
                target_peer: Some("nobody".to_string()),
                candidate: "{}".to_string(),
            },
            check_result: |msg| match msg {
                Some(ServerMessage::Error { code, .. }) => {
                    assert_eq!(code, ErrorCode::PeerNotFound)
                }
                other => panic!("expected error, got {other:?}"),
            },
        },
        ErrorCase {
            name: "ice candidate from an unjoined peer is dropped",
            sender: PeerRole::Publisher,
            gateway: FakeGateway::default,
            request_first: false,
            command: |_, _| RoomCommand::IceCandidate {
                peer_id: "ghost".to_string(),
                target_peer: None,
                candidate: "{}".to_string(),
            },
            check_result: |msg| assert!(msg.is_none(), "got {msg:?}"),
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let room = Room::spawn(vec![], Arc::new((case.gateway)()), None);

        let (pub_tx, mut pub_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Publisher,
            handle: ConnectionHandle {
                outbound: pub_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let publisher_id = joined.await.unwrap().unwrap();
        let _joined = tokio::time::timeout(WAIT, pub_rx.recv()).await.unwrap();

        let (sender_id, mut sender_rx) = match case.sender {
            PeerRole::Publisher => (publisher_id.clone(), pub_rx),
            PeerRole::Subscriber => {
                let (sub_tx, mut sub_rx) = mpsc::channel::<Outbound>(16);
                let (reply, joined) = oneshot::channel();
                room.send(RoomCommand::Join {
                    role: PeerRole::Subscriber,
                    handle: ConnectionHandle {
                        outbound: sub_tx,
                        remote_ip: None,
                    },
                    reply,
                })
                .await
                .unwrap();
                let subscriber_id = joined.await.unwrap().unwrap();
                let _joined = tokio::time::timeout(WAIT, sub_rx.recv()).await.unwrap();
                (subscriber_id, sub_rx)
            }
        };

        if case.request_first {
            room.send(RoomCommand::RequestTrack {
                peer_id: sender_id.clone(),
                target_peer: publisher_id.clone(),
            })
            .await
            .unwrap();
        }
        room.send((case.command)(&sender_id, &publisher_id))
            .await
            .unwrap();

        let msg = match tokio::time::timeout(QUIET, sender_rx.recv()).await {
            Ok(Some(Outbound::Message(msg))) => Some(msg),
            Ok(other) => panic!("{}: unexpected {other:?}", case.name),
            Err(_) => None,
        };
        (case.check_result)(msg);
    }
}

struct TrackCase {
    name: &'static str,
    gateway: fn() -> FakeGateway,
    check_result: fn(ServerMessage),
}

/// An engine failure while negotiating a subscriber's track: a failed attach
/// is only logged, while a failed offer is reported to the subscriber.
#[tokio::test]
async fn subscriber_track_engine_failures() {
    let cases = [
        TrackCase {
            name: "failed attach is logged and the offer still goes out",
            gateway: || FakeGateway {
                attach_error: Some(MediaError::Engine("attach".into())),
                ..Default::default()
            },
            check_result: |msg| match msg {
                ServerMessage::SdpOffer { from_peer, sdp } => {
                    assert!(from_peer.is_some());
                    assert_eq!(sdp, "fake-offer");
                }
                other => panic!("expected sdp_offer, got {other:?}"),
            },
        },
        TrackCase {
            name: "engine failing to create an offer is internal",
            gateway: || FakeGateway {
                create_offer_error: Some(MediaError::Engine("offer".into())),
                ..Default::default()
            },
            check_result: |msg| match msg {
                ServerMessage::Error { code, .. } => assert_eq!(code, ErrorCode::Internal),
                other => panic!("expected error, got {other:?}"),
            },
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let gateway = Arc::new((case.gateway)());
        let room = Room::spawn(vec![], gateway.clone(), None);

        let (pub_tx, mut pub_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Publisher,
            handle: ConnectionHandle {
                outbound: pub_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let publisher_id = joined.await.unwrap().unwrap();
        let _joined = tokio::time::timeout(WAIT, pub_rx.recv()).await.unwrap();
        // The publisher's session only exists once it has sent an offer.
        room.send(RoomCommand::SdpOffer {
            peer_id: publisher_id.clone(),
            target_peer: None,
            sdp: "offer".to_string(),
        })
        .await
        .unwrap();
        let _answer = tokio::time::timeout(WAIT, pub_rx.recv()).await.unwrap();
        let sink = gateway.publisher_sinks.lock().unwrap()[0].clone();
        sink.emit(MediaEvent::TrackPublished {
            track: Arc::new(FakeTrack),
        });
        tokio::time::sleep(QUIET).await;

        let (sub_tx, mut sub_rx) = mpsc::channel::<Outbound>(16);
        let (reply, joined) = oneshot::channel();
        room.send(RoomCommand::Join {
            role: PeerRole::Subscriber,
            handle: ConnectionHandle {
                outbound: sub_tx,
                remote_ip: None,
            },
            reply,
        })
        .await
        .unwrap();
        let subscriber_id = joined.await.unwrap().unwrap();
        let _joined = tokio::time::timeout(WAIT, sub_rx.recv()).await.unwrap();

        room.send(RoomCommand::RequestTrack {
            peer_id: subscriber_id,
            target_peer: publisher_id,
        })
        .await
        .unwrap();
        match tokio::time::timeout(WAIT, sub_rx.recv()).await.unwrap() {
            Some(Outbound::Message(msg)) => (case.check_result)(msg),
            other => panic!("{}: expected a message, got {other:?}", case.name),
        }
    }
}

#[tokio::test]
async fn join_is_refused_once_the_room_is_full() {
    let room = Room::spawn(vec![], Arc::new(FakeGateway::default()), Some(1));

    let (out_tx, _out_rx) = mpsc::channel::<Outbound>(16);
    let (reply, first) = oneshot::channel();
    room.send(RoomCommand::Join {
        role: PeerRole::Publisher,
        handle: ConnectionHandle {
            outbound: out_tx,
            remote_ip: None,
        },
        reply,
    })
    .await
    .unwrap();
    assert!(first.await.unwrap().is_ok(), "first peer fits");

    let (out_tx, _out_rx) = mpsc::channel::<Outbound>(16);
    let (reply, second) = oneshot::channel();
    room.send(RoomCommand::Join {
        role: PeerRole::Subscriber,
        handle: ConnectionHandle {
            outbound: out_tx,
            remote_ip: None,
        },
        reply,
    })
    .await
    .unwrap();
    let err = second.await.unwrap().unwrap_err();
    assert!(
        matches!(err, crate::domain::error::DomainError::InvalidState(_)),
        "second peer refused with InvalidState, got {err:?}"
    );
}
