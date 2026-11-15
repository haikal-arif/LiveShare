//! `GET /internal/rooms/:room_id/peers/:peer_id` and
//! `POST /internal/rooms/:room_id/peers/:peer_id/disconnect`
//! (`contracts.md` section 2). Each case boots its own server and clients.

use std::net::SocketAddr;
use std::time::Duration;

use common::internal::{ErrorBody, PeerResponse};
use common::signaling::{ClientMessage, IceServer, ServerMessage};
use futures_util::{SinkExt, StreamExt};
use server::config::Config;
use tokio::net::TcpListener;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::Message as WsMessage;

fn config() -> Config {
    Config {
        bind_addr: "127.0.0.1:0".to_string(),
        udp_ports: None,
        max_participants: None,
        public_ips: vec![],
        ice_servers: vec![IceServer {
            urls: vec!["stun:stun.example.com:3478".to_string()],
            username: None,
            credential: None,
        }],
    }
}

struct LookupCase {
    name: &'static str,
    join_as_publisher: bool,
    x_real_ip: Option<&'static str>,
    /// Overrides the joined peer's real id in the request path.
    peer_id: Option<&'static str>,
    /// Overrides the joined room in the request path.
    room_id: Option<&'static str>,
    expected_status: u16,
    check_result: fn(&str, &str),
}

#[tokio::test]
async fn peer_lookup_returns_ip_or_not_found() {
    let cases = [
        LookupCase {
            name: "publisher behind the proxy reports X-Real-IP",
            join_as_publisher: true,
            x_real_ip: Some("203.0.113.7"),
            peer_id: None,
            room_id: None,
            expected_status: 200,
            check_result: |body, peer_id| {
                let peer: PeerResponse = serde_json::from_str(body).unwrap();
                assert_eq!(peer.room_id, "room-peers");
                assert_eq!(peer.peer_id, peer_id);
                assert_eq!(peer.ip.as_deref(), Some("203.0.113.7"));
            },
        },
        LookupCase {
            name: "subscriber without the header falls back to the socket address",
            join_as_publisher: false,
            x_real_ip: None,
            peer_id: None,
            room_id: None,
            expected_status: 200,
            check_result: |body, _| {
                let peer: PeerResponse = serde_json::from_str(body).unwrap();
                assert_eq!(peer.ip.as_deref(), Some("127.0.0.1"));
            },
        },
        LookupCase {
            name: "unknown peer is peer_not_found",
            join_as_publisher: true,
            x_real_ip: None,
            peer_id: Some("pub-nope"),
            room_id: None,
            expected_status: 404,
            check_result: |body, _| {
                let err: ErrorBody = serde_json::from_str(body).unwrap();
                assert_eq!(err.error.code, "peer_not_found");
                assert!(
                    err.error.message.contains("pub-nope"),
                    "{}",
                    err.error.message
                );
            },
        },
        LookupCase {
            name: "unknown room is peer_not_found too",
            join_as_publisher: true,
            x_real_ip: None,
            peer_id: None,
            room_id: Some("never-joined"),
            expected_status: 404,
            check_result: |body, _| {
                let err: ErrorBody = serde_json::from_str(body).unwrap();
                assert_eq!(err.error.code, "peer_not_found");
            },
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let app = server::app(config());
        tokio::spawn(async move {
            axum::serve(
                listener,
                app.into_make_service_with_connect_info::<SocketAddr>(),
            )
            .await
            .unwrap();
        });

        let mut request = format!("ws://{addr}/ws").into_client_request().unwrap();
        if let Some(ip) = case.x_real_ip {
            request
                .headers_mut()
                .insert("x-real-ip", ip.parse().unwrap());
        }
        let (mut ws, _) = tokio_tungstenite::connect_async(request).await.unwrap();
        let join = if case.join_as_publisher {
            ClientMessage::JoinAsPublisher {
                room: "room-peers".to_string(),
            }
        } else {
            ClientMessage::JoinAsSubscriber {
                room: "room-peers".to_string(),
            }
        };
        ws.send(WsMessage::Text(serde_json::to_string(&join).unwrap()))
            .await
            .unwrap();
        let peer_id = loop {
            let WsMessage::Text(text) = ws.next().await.unwrap().unwrap() else {
                continue;
            };
            if let ServerMessage::Joined { peer_id, .. } = serde_json::from_str(&text).unwrap() {
                break peer_id;
            }
        };

        let response = reqwest::get(format!(
            "http://{addr}/internal/rooms/{}/peers/{}",
            case.room_id.unwrap_or("room-peers"),
            case.peer_id.unwrap_or(&peer_id),
        ))
        .await
        .unwrap();
        assert_eq!(
            response.status().as_u16(),
            case.expected_status,
            "{}",
            case.name
        );
        let body = response.text().await.unwrap();
        (case.check_result)(&body, &peer_id);
    }
}

struct DisconnectCase {
    name: &'static str,
    join_as_publisher: bool,
    peer_id: Option<&'static str>,
    expected_status: u16,
    /// Whether the peer's websocket should end after the call.
    expect_socket_closed: bool,
    /// Whether a lookup right afterwards still finds the peer.
    expect_peer_gone: bool,
}

#[tokio::test]
async fn disconnect_removes_peer_and_closes_its_socket() {
    let cases = [
        DisconnectCase {
            name: "publisher is removed and its socket closed",
            join_as_publisher: true,
            peer_id: None,
            expected_status: 204,
            expect_socket_closed: true,
            expect_peer_gone: true,
        },
        DisconnectCase {
            name: "subscriber is removed and its socket closed",
            join_as_publisher: false,
            peer_id: None,
            expected_status: 204,
            expect_socket_closed: true,
            expect_peer_gone: true,
        },
        DisconnectCase {
            name: "unknown peer is peer_not_found and others stay connected",
            join_as_publisher: true,
            peer_id: Some("pub-nope"),
            expected_status: 404,
            expect_socket_closed: false,
            expect_peer_gone: false,
        },
    ];

    for case in cases {
        println!("case: {}", case.name);
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let app = server::app(config());
        tokio::spawn(async move {
            axum::serve(
                listener,
                app.into_make_service_with_connect_info::<SocketAddr>(),
            )
            .await
            .unwrap();
        });

        let (mut ws, _) = tokio_tungstenite::connect_async(format!("ws://{addr}/ws"))
            .await
            .unwrap();
        let join = if case.join_as_publisher {
            ClientMessage::JoinAsPublisher {
                room: "room-kick".to_string(),
            }
        } else {
            ClientMessage::JoinAsSubscriber {
                room: "room-kick".to_string(),
            }
        };
        ws.send(WsMessage::Text(serde_json::to_string(&join).unwrap()))
            .await
            .unwrap();
        let peer_id = loop {
            let WsMessage::Text(text) = ws.next().await.unwrap().unwrap() else {
                continue;
            };
            if let ServerMessage::Joined { peer_id, .. } = serde_json::from_str(&text).unwrap() {
                break peer_id;
            }
        };

        let client = reqwest::Client::new();
        let target = case.peer_id.unwrap_or(&peer_id);
        let response = client
            .post(format!(
                "http://{addr}/internal/rooms/room-kick/peers/{target}/disconnect"
            ))
            .send()
            .await
            .unwrap();
        assert_eq!(
            response.status().as_u16(),
            case.expected_status,
            "{}",
            case.name
        );

        // Nothing is sent before the close: the next frame is the close itself.
        let ended = tokio::time::timeout(Duration::from_secs(2), async {
            loop {
                match ws.next().await {
                    None | Some(Err(_)) | Some(Ok(WsMessage::Close(_))) => return true,
                    Some(Ok(WsMessage::Text(text))) => panic!("unexpected frame: {text}"),
                    Some(Ok(_)) => continue,
                }
            }
        })
        .await
        .unwrap_or(false);
        assert_eq!(ended, case.expect_socket_closed, "{}", case.name);

        let lookup = client
            .get(format!(
                "http://{addr}/internal/rooms/room-kick/peers/{peer_id}"
            ))
            .send()
            .await
            .unwrap();
        let gone = lookup.status().as_u16() == 404;
        assert_eq!(gone, case.expect_peer_gone, "{}", case.name);
    }
}

#[tokio::test]
async fn disconnecting_a_publisher_tells_subscribers_it_left() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let app = server::app(config());
    tokio::spawn(async move {
        axum::serve(
            listener,
            app.into_make_service_with_connect_info::<SocketAddr>(),
        )
        .await
        .unwrap();
    });

    let (mut publisher, _) = tokio_tungstenite::connect_async(format!("ws://{addr}/ws"))
        .await
        .unwrap();
    let join = ClientMessage::JoinAsPublisher {
        room: "room-left".to_string(),
    };
    publisher
        .send(WsMessage::Text(serde_json::to_string(&join).unwrap()))
        .await
        .unwrap();
    let publisher_id = loop {
        let WsMessage::Text(text) = publisher.next().await.unwrap().unwrap() else {
            continue;
        };
        if let ServerMessage::Joined { peer_id, .. } = serde_json::from_str(&text).unwrap() {
            break peer_id;
        }
    };

    let (mut subscriber, _) = tokio_tungstenite::connect_async(format!("ws://{addr}/ws"))
        .await
        .unwrap();
    let join = ClientMessage::JoinAsSubscriber {
        room: "room-left".to_string(),
    };
    subscriber
        .send(WsMessage::Text(serde_json::to_string(&join).unwrap()))
        .await
        .unwrap();
    loop {
        let WsMessage::Text(text) = subscriber.next().await.unwrap().unwrap() else {
            continue;
        };
        if matches!(
            serde_json::from_str(&text).unwrap(),
            ServerMessage::Joined { .. }
        ) {
            break;
        }
    }

    let status = reqwest::Client::new()
        .post(format!(
            "http://{addr}/internal/rooms/room-left/peers/{publisher_id}/disconnect"
        ))
        .send()
        .await
        .unwrap()
        .status();
    assert_eq!(status.as_u16(), 204);

    let left = tokio::time::timeout(Duration::from_secs(2), async {
        loop {
            let WsMessage::Text(text) = subscriber.next().await.unwrap().unwrap() else {
                continue;
            };
            if let ServerMessage::PublisherLeft { peer_id } = serde_json::from_str(&text).unwrap() {
                return peer_id;
            }
        }
    })
    .await
    .expect("subscriber never saw publisher_left");
    assert_eq!(left, publisher_id);
}
