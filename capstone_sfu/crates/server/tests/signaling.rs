use common::signaling::{
    ClientMessage, ConnectionQuality, IceServer, ServerMessage, SimulcastLayer,
};
use futures_util::{SinkExt, StreamExt};
use server::config::Config;
use tokio::net::TcpListener;
use tokio_tungstenite::tungstenite::Message as WsMessage;

/// Spins up the real app on an ephemeral port and returns its base
/// `http://` and `ws://` URLs. The server task runs until the test
/// process exits.
async fn spawn_app() -> (String, String) {
    let config = Config {
        bind_addr: "127.0.0.1:0".to_string(),
        udp_ports: None,
        max_participants: None,
        public_ips: vec![],
        ice_servers: vec![IceServer {
            urls: vec!["stun:stun.example.com:3478".to_string()],
            username: None,
            credential: None,
        }],
    };
    let app = server::app(config);

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    tokio::spawn(async move {
        axum::serve(listener, app).await.unwrap();
    });

    (format!("http://{addr}"), format!("ws://{addr}/ws"))
}

async fn connect(
    ws_url: &str,
) -> tokio_tungstenite::WebSocketStream<tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>> {
    let (stream, _) = tokio_tungstenite::connect_async(ws_url)
        .await
        .expect("ws connect");
    stream
}

async fn send(
    ws: &mut tokio_tungstenite::WebSocketStream<
        tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>,
    >,
    msg: &ClientMessage,
) {
    let text = serde_json::to_string(msg).unwrap();
    ws.send(WsMessage::Text(text)).await.unwrap();
}

async fn recv(
    ws: &mut tokio_tungstenite::WebSocketStream<
        tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>,
    >,
) -> ServerMessage {
    loop {
        match ws.next().await.expect("stream ended").expect("ws error") {
            WsMessage::Text(text) => return serde_json::from_str(&text).unwrap(),
            _ => continue,
        }
    }
}

#[tokio::test]
async fn publisher_join_gets_joined_message() {
    let (_, ws_url) = spawn_app().await;
    let mut ws = connect(&ws_url).await;

    send(
        &mut ws,
        &ClientMessage::JoinAsPublisher {
            room: "room-1".to_string(),
        },
    )
    .await;

    match recv(&mut ws).await {
        ServerMessage::Joined {
            peer_id,
            publishers,
            ice_servers,
        } => {
            assert!(peer_id.starts_with("pub-"));
            assert!(publishers.is_empty());
            assert_eq!(ice_servers[0].urls[0], "stun:stun.example.com:3478");
        }
        other => panic!("expected Joined, got {other:?}"),
    }
}

#[tokio::test]
async fn subscriber_sees_existing_publisher_in_snapshot_and_publisher_joined_broadcast() {
    let (_, ws_url) = spawn_app().await;

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-2".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-2".to_string(),
        },
    )
    .await;
    match recv(&mut subscriber).await {
        ServerMessage::Joined { publishers, .. } => assert_eq!(publishers, vec![pub_id.clone()]),
        other => panic!("expected Joined, got {other:?}"),
    }

    // A second publisher joining the same room should broadcast to the
    // already-connected subscriber.
    let mut publisher2 = connect(&ws_url).await;
    send(
        &mut publisher2,
        &ClientMessage::JoinAsPublisher {
            room: "room-2".to_string(),
        },
    )
    .await;
    let pub_id2 = match recv(&mut publisher2).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };

    match recv(&mut subscriber).await {
        ServerMessage::PublisherJoined { peer_id } => assert_eq!(peer_id, pub_id2),
        other => panic!("expected PublisherJoined, got {other:?}"),
    }
}

#[tokio::test]
async fn request_track_for_publisher_without_media_is_accepted_and_offer_deferred() {
    let (_, ws_url) = spawn_app().await;

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-3".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-3".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;

    send(
        &mut subscriber,
        &ClientMessage::RequestTrack {
            target_peer: pub_id.clone(),
        },
    )
    .await;
    // The publisher never negotiated media, so there are no tracks to offer
    // yet: no error, and no (empty) sdp_offer either. The offer itself is
    // covered by `tests/media.rs`, which uses real WebRTC clients.
    let next =
        tokio::time::timeout(std::time::Duration::from_millis(300), recv(&mut subscriber)).await;
    assert!(next.is_err(), "expected no message yet, got {next:?}");
}

#[tokio::test]
async fn request_track_for_unknown_publisher_returns_error() {
    let (_, ws_url) = spawn_app().await;

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-4".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;

    send(
        &mut subscriber,
        &ClientMessage::RequestTrack {
            target_peer: "pub-nonexistent".to_string(),
        },
    )
    .await;
    match recv(&mut subscriber).await {
        ServerMessage::Error { reason, .. } => assert!(reason.contains("not found")),
        other => panic!("expected Error, got {other:?}"),
    }
}

#[tokio::test]
async fn publisher_disconnect_broadcasts_publisher_left() {
    let (_, ws_url) = spawn_app().await;

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-5".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-5".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;

    publisher.close(None).await.unwrap();

    match recv(&mut subscriber).await {
        ServerMessage::PublisherLeft { peer_id } => assert_eq!(peer_id, pub_id),
        other => panic!("expected PublisherLeft, got {other:?}"),
    }
}

#[tokio::test]
async fn internal_rest_reflects_room_state() {
    let (http_url, ws_url) = spawn_app().await;
    let client = reqwest::Client::new();

    let empty: serde_json::Value = client
        .get(format!("{http_url}/internal/rooms/room-6/publishers"))
        .send()
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(empty["publishers"], serde_json::json!([]));

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-6".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-6".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;

    // Give the room actor a moment to process both joins.
    tokio::time::sleep(std::time::Duration::from_millis(50)).await;

    let publishers: serde_json::Value = client
        .get(format!("{http_url}/internal/rooms/room-6/publishers"))
        .send()
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(
        publishers["publishers"],
        serde_json::json!([{"peer_id": pub_id}])
    );

    let stats: serde_json::Value = client
        .get(format!("{http_url}/internal/rooms/room-6/stats"))
        .send()
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(stats["publisher_count"], 1);
    assert_eq!(stats["subscriber_count"], 1);
}

#[tokio::test]
async fn internal_health_ok() {
    let (http_url, _) = spawn_app().await;
    let resp: serde_json::Value = reqwest::get(format!("{http_url}/internal/health"))
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(resp["status"], "ok");
}

fn stats(target_peer: &str, loss: f64) -> ClientMessage {
    ClientMessage::ConnectionStats {
        target_peer: target_peer.to_string(),
        available_incoming_bitrate_bps: Some(3_000_000),
        packet_loss_pct: loss,
        round_trip_time_ms: Some(40.0),
        jitter_ms: Some(10.0),
    }
}

#[tokio::test]
async fn connection_stats_parses_with_only_required_fields() {
    let msg: ClientMessage = serde_json::from_str(
        r#"{"type":"connection_stats","target_peer":"pub-1","packet_loss_pct":3.5}"#,
    )
    .unwrap();
    assert!(matches!(
        msg,
        ClientMessage::ConnectionStats {
            available_incoming_bitrate_bps: None,
            ..
        }
    ));
}

#[tokio::test]
async fn connection_stats_from_publisher_or_for_unrequested_publisher_is_rejected() {
    let (_, ws_url) = spawn_app().await;

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-7".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };
    send(&mut publisher, &stats(&pub_id, 0.0)).await;
    match recv(&mut publisher).await {
        ServerMessage::Error { reason, .. } => {
            assert!(reason.contains("only a subscriber"), "{reason}")
        }
        other => panic!("expected Error, got {other:?}"),
    }

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-7".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;
    send(&mut subscriber, &stats(&pub_id, 0.0)).await;
    match recv(&mut subscriber).await {
        ServerMessage::Error { reason, .. } => {
            assert!(reason.contains("requested publisher"), "{reason}")
        }
        other => panic!("expected Error, got {other:?}"),
    }
}

#[tokio::test]
async fn sustained_loss_reports_bad_quality_and_shows_in_internal_stats() {
    let (http_url, ws_url) = spawn_app().await;

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-8".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-8".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;
    send(
        &mut subscriber,
        &ClientMessage::RequestTrack {
            target_peer: pub_id.clone(),
        },
    )
    .await;

    // Good reports change nothing, and a single bad one isn't enough.
    send(&mut subscriber, &stats(&pub_id, 0.0)).await;
    send(&mut subscriber, &stats(&pub_id, 25.0)).await;
    let next =
        tokio::time::timeout(std::time::Duration::from_millis(300), recv(&mut subscriber)).await;
    assert!(next.is_err(), "expected no message yet, got {next:?}");

    send(&mut subscriber, &stats(&pub_id, 25.0)).await;
    match recv(&mut subscriber).await {
        ServerMessage::ConnectionQuality {
            from_peer,
            quality,
            layer,
        } => {
            assert_eq!(from_peer, pub_id);
            assert_eq!(quality, ConnectionQuality::Bad);
            // No media from this publisher yet, so this is the layer it'll
            // get once there is.
            assert_eq!(layer, SimulcastLayer::Low);
        }
        other => panic!("expected ConnectionQuality, got {other:?}"),
    }

    let stats: serde_json::Value = reqwest::get(format!("{http_url}/internal/rooms/room-8/stats"))
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(
        stats["subscriber_quality"],
        serde_json::json!({"good": 0, "poor": 0, "bad": 1})
    );
}

#[tokio::test]
async fn set_preferred_layer_is_acknowledged_and_retries_after_a_downgrade() {
    let (http_url, ws_url) = spawn_app().await;

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-9".to_string(),
        },
    )
    .await;
    let pub_id = match recv(&mut publisher).await {
        ServerMessage::Joined { peer_id, .. } => peer_id,
        other => panic!("expected Joined, got {other:?}"),
    };
    send(
        &mut publisher,
        &ClientMessage::SetPreferredLayer {
            target_peer: pub_id.clone(),
            layer: SimulcastLayer::High,
        },
    )
    .await;
    match recv(&mut publisher).await {
        ServerMessage::Error { reason, .. } => {
            assert!(reason.contains("only a subscriber"), "{reason}")
        }
        other => panic!("expected Error, got {other:?}"),
    }

    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-9".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;
    send(
        &mut subscriber,
        &ClientMessage::RequestTrack {
            target_peer: pub_id.clone(),
        },
    )
    .await;

    // A pick is always answered, even when nothing changed.
    send(
        &mut subscriber,
        &ClientMessage::SetPreferredLayer {
            target_peer: pub_id.clone(),
            layer: SimulcastLayer::High,
        },
    )
    .await;
    let expect_quality = |msg: ServerMessage| match msg {
        ServerMessage::ConnectionQuality { quality, layer, .. } => (quality, layer),
        other => panic!("expected ConnectionQuality, got {other:?}"),
    };
    assert_eq!(
        expect_quality(recv(&mut subscriber).await),
        (ConnectionQuality::Good, SimulcastLayer::High)
    );

    // Sustained poor loss steps down one layer, and good reports alone
    // never bring it back.
    send(&mut subscriber, &stats(&pub_id, 5.0)).await;
    send(&mut subscriber, &stats(&pub_id, 5.0)).await;
    assert_eq!(
        expect_quality(recv(&mut subscriber).await),
        (ConnectionQuality::Poor, SimulcastLayer::Mid)
    );
    for _ in 0..5 {
        send(&mut subscriber, &stats(&pub_id, 0.0)).await;
    }
    let next =
        tokio::time::timeout(std::time::Duration::from_millis(300), recv(&mut subscriber)).await;
    assert!(next.is_err(), "expected no auto-upgrade, got {next:?}");

    // Picking high again is the retry.
    send(
        &mut subscriber,
        &ClientMessage::SetPreferredLayer {
            target_peer: pub_id.clone(),
            layer: SimulcastLayer::High,
        },
    )
    .await;
    assert_eq!(
        expect_quality(recv(&mut subscriber).await),
        (ConnectionQuality::Good, SimulcastLayer::High)
    );
    let stats: serde_json::Value = reqwest::get(format!("{http_url}/internal/rooms/room-9/stats"))
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(stats["subscriber_quality"]["good"], 1);
}

/// Reads until the server closes the socket, returning what arrived first.
async fn expect_room_closed_then_disconnect(
    ws: &mut tokio_tungstenite::WebSocketStream<
        tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>,
    >,
) {
    let closed = tokio::time::timeout(std::time::Duration::from_secs(5), async {
        loop {
            match ws.next().await {
                Some(Ok(WsMessage::Text(text))) => {
                    if matches!(
                        serde_json::from_str(&text).unwrap(),
                        ServerMessage::RoomClosed
                    ) {
                        break;
                    }
                }
                Some(Ok(_)) => {}
                other => panic!("socket ended before room_closed: {other:?}"),
            }
        }
        // After room_closed the server closes the socket itself.
        loop {
            match ws.next().await {
                Some(Ok(WsMessage::Close(_))) | None | Some(Err(_)) => return,
                Some(Ok(_)) => {}
            }
        }
    })
    .await;
    assert!(closed.is_ok(), "socket was not closed after room_closed");
}

#[tokio::test]
async fn closing_a_room_tells_everyone_disconnects_them_and_resets_the_room() {
    let (http_url, ws_url) = spawn_app().await;
    let client = reqwest::Client::new();

    // Unknown rooms close fine too, so Go can retry.
    let resp = client
        .post(format!("{http_url}/internal/rooms/never-joined/close"))
        .send()
        .await
        .unwrap();
    assert_eq!(resp.status(), 204);

    let mut publisher = connect(&ws_url).await;
    send(
        &mut publisher,
        &ClientMessage::JoinAsPublisher {
            room: "room-10".to_string(),
        },
    )
    .await;
    let _ = recv(&mut publisher).await;
    let mut subscriber = connect(&ws_url).await;
    send(
        &mut subscriber,
        &ClientMessage::JoinAsSubscriber {
            room: "room-10".to_string(),
        },
    )
    .await;
    let _ = recv(&mut subscriber).await;

    let resp = client
        .post(format!("{http_url}/internal/rooms/room-10/close"))
        .send()
        .await
        .unwrap();
    assert_eq!(resp.status(), 204);

    expect_room_closed_then_disconnect(&mut publisher).await;
    expect_room_closed_then_disconnect(&mut subscriber).await;

    let publishers: serde_json::Value = client
        .get(format!("{http_url}/internal/rooms/room-10/publishers"))
        .send()
        .await
        .unwrap()
        .json()
        .await
        .unwrap();
    assert_eq!(publishers["publishers"], serde_json::json!([]));

    // The same room id can be used again afterwards.
    let mut again = connect(&ws_url).await;
    send(
        &mut again,
        &ClientMessage::JoinAsSubscriber {
            room: "room-10".to_string(),
        },
    )
    .await;
    match recv(&mut again).await {
        ServerMessage::Joined { publishers, .. } => assert!(publishers.is_empty()),
        other => panic!("expected Joined, got {other:?}"),
    }
}
