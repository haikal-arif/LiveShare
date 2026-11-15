use std::sync::Arc;

use axum::extract::ws::{Message as WsMessage, WebSocket};
use common::signaling::{ClientMessage, PeerId, ServerMessage};
use futures_util::{SinkExt, StreamExt};
use tokio::sync::{mpsc, oneshot};

use super::mapping::to_command;
use crate::domain::error::DomainError;
use crate::domain::peer::{ConnectionHandle, Outbound, PeerRole};
use crate::service::registry::RoomService;
use crate::service::room::RoomCommand;

/// Once joined, a connection is pinned to a single room/role for its
/// lifetime — subsequent messages route straight to that room's actor by
/// this `peer_id`.
struct Session {
    peer_id: PeerId,
    room_tx: mpsc::Sender<RoomCommand>,
}

/// Runs one client connection until either side hangs up.
pub(super) async fn run(socket: WebSocket, rooms: Arc<RoomService>, remote_ip: Option<String>) {
    let (mut ws_tx, mut ws_rx) = socket.split();
    let (outbound_tx, mut outbound_rx) = mpsc::channel::<Outbound>(64);

    let mut writer = tokio::spawn(async move {
        while let Some(outbound) = outbound_rx.recv().await {
            match outbound {
                Outbound::Message(msg) => {
                    let Ok(text) = serde_json::to_string(&msg) else {
                        continue;
                    };
                    if ws_tx.send(WsMessage::Text(text)).await.is_err() {
                        break;
                    }
                }
                Outbound::Close => {
                    let _ = ws_tx.send(WsMessage::Close(None)).await;
                    break;
                }
            }
        }
    });

    let mut session: Option<Session> = None;

    loop {
        let next = tokio::select! {
            next = ws_rx.next() => next,
            // The writer only stops once the socket is going away, e.g.
            // after `room_closed`; don't wait for the client to hang up.
            _ = &mut writer => break,
        };
        let Some(Ok(msg)) = next else { break };
        let text = match msg {
            WsMessage::Text(text) => text,
            WsMessage::Close(_) => break,
            _ => continue,
        };

        let Ok(client_msg) = serde_json::from_str::<ClientMessage>(&text) else {
            send_error(
                &outbound_tx,
                DomainError::InvalidMessage("invalid message".into()),
            );
            continue;
        };

        match decide(session.as_ref().map(|s| &s.peer_id), client_msg) {
            Step::Join { role, room } => {
                session = join(&rooms, room, role, &outbound_tx, &remote_ip).await;
            }
            Step::Forward(cmd) => {
                if let Some(s) = &session {
                    let _ = s.room_tx.send(cmd).await;
                }
            }
            Step::Reject(error) => send_error(&outbound_tx, error),
            Step::Leave => break,
        }
    }

    if let Some(s) = session {
        let _ = s
            .room_tx
            .send(RoomCommand::Disconnect { peer_id: s.peer_id })
            .await;
    }
    writer.abort();
}

/// Joins the room. `None` means the join was refused (the room is full, or
/// its actor is gone); the error has already been sent to the client and
/// the connection stays unjoined, so it can try again later.
async fn join(
    rooms: &Arc<RoomService>,
    room: String,
    role: PeerRole,
    outbound_tx: &mpsc::Sender<Outbound>,
    remote_ip: &Option<String>,
) -> Option<Session> {
    let room_tx = rooms.get_or_create(&room);
    let (reply, rx) = oneshot::channel();
    let handle = ConnectionHandle {
        outbound: outbound_tx.clone(),
        remote_ip: remote_ip.clone(),
    };

    // The room channel only fails if that room's actor task has panicked.
    let sent = room_tx
        .send(RoomCommand::Join {
            role,
            handle,
            reply,
        })
        .await;
    let outcome = match sent {
        Ok(()) => rx.await.unwrap_or_else(|_| {
            Err(DomainError::Internal("room did not answer the join".into()))
        }),
        Err(_) => Err(DomainError::Internal("room is unavailable".into())),
    };

    match outcome {
        Ok(peer_id) => Some(Session { peer_id, room_tx }),
        Err(error) => {
            send_error(outbound_tx, error);
            None
        }
    }
}

/// What the loop must do with one client message. Produced by [`decide`],
/// which holds all the rules and does no I/O.
enum Step {
    Join { role: PeerRole, room: String },
    Forward(RoomCommand),
    Reject(DomainError),
    Leave,
}

/// Decides what a message means given whether the connection has joined.
/// `joined` is the peer id of the connection's session, if it has one.
fn decide(joined: Option<&PeerId>, msg: ClientMessage) -> Step {
    match (joined, msg) {
        (None, ClientMessage::JoinAsPublisher { room }) => Step::Join {
            role: PeerRole::Publisher,
            room,
        },
        (None, ClientMessage::JoinAsSubscriber { room }) => Step::Join {
            role: PeerRole::Subscriber,
            room,
        },
        (None, _) => Step::Reject(DomainError::NotJoined(
            "must join_as_publisher or join_as_subscriber first".into(),
        )),
        (
            Some(_),
            ClientMessage::JoinAsPublisher { .. } | ClientMessage::JoinAsSubscriber { .. },
        ) => Step::Reject(DomainError::AlreadyJoined("already joined".into())),
        (Some(_), ClientMessage::Leave {}) => Step::Leave,
        (Some(peer_id), msg) => match to_command(peer_id, msg) {
            Some(cmd) => Step::Forward(cmd),
            // Unreachable: join and leave were matched above.
            None => Step::Reject(DomainError::Internal("unroutable message".into())),
        },
    }
}

/// Best-effort: a full or closed channel just drops the error.
fn send_error(outbound_tx: &mpsc::Sender<Outbound>, error: DomainError) {
    let _ = outbound_tx.try_send(ServerMessage::from(error).into());
}

#[cfg(test)]
mod tests {
    use super::*;
    use common::signaling::SimulcastLayer;

    struct TestCase {
        name: &'static str,
        setup: fn() -> (Option<PeerId>, ClientMessage),
        check_result: fn(Step),
    }

    #[test]
    fn decides_what_to_do_with_each_message() {
        let cases = [
            TestCase {
                name: "unjoined publisher join starts a publisher join",
                setup: || (None, ClientMessage::JoinAsPublisher { room: "r".into() }),
                check_result: |step| match step {
                    Step::Join { role, room } => {
                        assert_eq!(role, PeerRole::Publisher);
                        assert_eq!(room, "r");
                    }
                    _ => panic!("expected Join"),
                },
            },
            TestCase {
                name: "unjoined subscriber join starts a subscriber join",
                setup: || (None, ClientMessage::JoinAsSubscriber { room: "r".into() }),
                check_result: |step| match step {
                    Step::Join { role, room } => {
                        assert_eq!(role, PeerRole::Subscriber);
                        assert_eq!(room, "r");
                    }
                    _ => panic!("expected Join"),
                },
            },
            TestCase {
                name: "unjoined non-join message is rejected as not joined",
                setup: || {
                    (
                        None,
                        ClientMessage::SetPreferredLayer {
                            target_peer: "pub-1".into(),
                            layer: SimulcastLayer::Low,
                        },
                    )
                },
                check_result: |step| match step {
                    Step::Reject(DomainError::NotJoined(_)) => {}
                    _ => panic!("expected NotJoined"),
                },
            },
            TestCase {
                name: "unjoined leave is rejected as not joined",
                setup: || (None, ClientMessage::Leave {}),
                check_result: |step| match step {
                    Step::Reject(DomainError::NotJoined(_)) => {}
                    _ => panic!("expected NotJoined"),
                },
            },
            TestCase {
                name: "joined connection joining again is rejected",
                setup: || {
                    (
                        Some("sub-1".into()),
                        ClientMessage::JoinAsSubscriber { room: "r".into() },
                    )
                },
                check_result: |step| match step {
                    Step::Reject(DomainError::AlreadyJoined(_)) => {}
                    _ => panic!("expected AlreadyJoined"),
                },
            },
            TestCase {
                name: "joined connection leave ends the loop",
                setup: || (Some("sub-1".into()), ClientMessage::Leave {}),
                check_result: |step| match step {
                    Step::Leave => {}
                    _ => panic!("expected Leave"),
                },
            },
            TestCase {
                name: "joined connection message is forwarded with its peer id",
                setup: || {
                    (
                        Some("sub-1".into()),
                        ClientMessage::SetPreferredLayer {
                            target_peer: "pub-1".into(),
                            layer: SimulcastLayer::High,
                        },
                    )
                },
                check_result: |step| match step {
                    Step::Forward(RoomCommand::SetPreferredLayer {
                        peer_id,
                        target_peer,
                        layer,
                    }) => {
                        assert_eq!(peer_id, "sub-1");
                        assert_eq!(target_peer, "pub-1");
                        assert_eq!(layer, SimulcastLayer::High);
                    }
                    _ => panic!("expected Forward(SetPreferredLayer)"),
                },
            },
        ];
        for case in cases {
            println!("case: {}", case.name);
            let (joined, msg) = (case.setup)();
            (case.check_result)(decide(joined.as_ref(), msg));
        }
    }
}
