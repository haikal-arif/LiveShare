//! `RoomService::close_all`, the step `main` runs on SIGTERM / Ctrl-C.

use std::time::Duration;

use common::signaling::{ClientMessage, IceServer, ServerMessage};
use futures_util::{SinkExt, StreamExt};
use server::config::Config;
use tokio::net::TcpListener;
use tokio_tungstenite::tungstenite::Message as WsMessage;

struct Case {
    name: &'static str,
    /// (room, join as publisher?) for each connected client.
    clients: Vec<(&'static str, bool)>,
}

#[tokio::test]
async fn close_all_tells_every_peer_in_every_room_then_closes_sockets() {
    let cases = [
        Case {
            name: "no rooms is a no-op",
            clients: vec![],
        },
        Case {
            name: "one room, one publisher and one subscriber",
            clients: vec![("room-a", true), ("room-a", false)],
        },
        Case {
            name: "several rooms",
            clients: vec![("room-a", true), ("room-b", false), ("room-c", true)],
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let (app, rooms) = server::build(Config {
            bind_addr: "127.0.0.1:0".to_string(),
            udp_ports: None,
            max_participants: None,
            public_ips: vec![],
            ice_servers: vec![IceServer {
                urls: vec!["stun:stun.example.com:3478".to_string()],
                username: None,
                credential: None,
            }],
        });
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        tokio::spawn(async move { axum::serve(listener, app).await.unwrap() });

        let mut sockets = Vec::new();
        for (room, as_publisher) in &case.clients {
            let (mut ws, _) = tokio_tungstenite::connect_async(format!("ws://{addr}/ws"))
                .await
                .unwrap();
            let join = if *as_publisher {
                ClientMessage::JoinAsPublisher {
                    room: room.to_string(),
                }
            } else {
                ClientMessage::JoinAsSubscriber {
                    room: room.to_string(),
                }
            };
            ws.send(WsMessage::Text(serde_json::to_string(&join).unwrap()))
                .await
                .unwrap();
            loop {
                let WsMessage::Text(text) = ws.next().await.unwrap().unwrap() else {
                    continue;
                };
                if matches!(
                    serde_json::from_str(&text).unwrap(),
                    ServerMessage::Joined { .. }
                ) {
                    break;
                }
            }
            sockets.push(ws);
        }

        rooms.close_all().await;

        for mut ws in sockets {
            let saw_room_closed = tokio::time::timeout(Duration::from_secs(5), async {
                let mut saw = false;
                loop {
                    match ws.next().await {
                        None | Some(Err(_)) | Some(Ok(WsMessage::Close(_))) => return saw,
                        Some(Ok(WsMessage::Text(text))) => {
                            if matches!(serde_json::from_str(&text), Ok(ServerMessage::RoomClosed))
                            {
                                saw = true;
                            }
                        }
                        Some(Ok(_)) => {}
                    }
                }
            })
            .await
            .expect("socket was not closed");
            assert!(
                saw_room_closed,
                "{}: no room_closed before close",
                case.name
            );
        }
    }
}
