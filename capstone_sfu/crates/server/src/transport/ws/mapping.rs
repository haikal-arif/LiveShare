use common::signaling::{ClientMessage, PeerId};

use crate::domain::quality::StatsReport;
use crate::service::room::RoomCommand;

/// The room command a joined peer's message turns into. `None` for the
/// messages the session handles itself (`join_as_*`, `leave`).
pub(super) fn to_command(peer_id: &PeerId, msg: ClientMessage) -> Option<RoomCommand> {
    let peer_id = peer_id.clone();
    Some(match msg {
        ClientMessage::SdpOffer { target_peer, sdp } => RoomCommand::SdpOffer {
            peer_id,
            target_peer,
            sdp,
        },
        ClientMessage::SdpAnswer { target_peer, sdp } => RoomCommand::SdpAnswer {
            peer_id,
            target_peer,
            sdp,
        },
        ClientMessage::IceCandidate {
            target_peer,
            candidate,
        } => RoomCommand::IceCandidate {
            peer_id,
            target_peer,
            candidate,
        },
        ClientMessage::RequestTrack { target_peer } => RoomCommand::RequestTrack {
            peer_id,
            target_peer,
        },
        // `round_trip_time_ms` and `jitter_ms` are accepted but unused.
        ClientMessage::ConnectionStats {
            target_peer,
            available_incoming_bitrate_bps,
            packet_loss_pct,
            ..
        } => RoomCommand::ConnectionStats {
            peer_id,
            target_peer,
            report: StatsReport {
                available_incoming_bitrate_bps,
                packet_loss_pct,
            },
        },
        ClientMessage::SetPreferredLayer { target_peer, layer } => RoomCommand::SetPreferredLayer {
            peer_id,
            target_peer,
            layer,
        },
        ClientMessage::JoinAsPublisher { .. }
        | ClientMessage::JoinAsSubscriber { .. }
        | ClientMessage::Leave {} => return None,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use common::signaling::SimulcastLayer;

    struct TestCase {
        name: &'static str,
        setup: fn() -> ClientMessage,
        check_result: fn(Option<RoomCommand>),
    }

    #[test]
    fn maps_client_messages_to_room_commands() {
        let cases = [
            TestCase {
                name: "set_preferred_layer carries peer, target and layer",
                setup: || ClientMessage::SetPreferredLayer {
                    target_peer: "pub-1".into(),
                    layer: SimulcastLayer::Low,
                },
                check_result: |cmd| match cmd {
                    Some(RoomCommand::SetPreferredLayer {
                        peer_id,
                        target_peer,
                        layer,
                    }) => {
                        assert_eq!(peer_id, "sub-1");
                        assert_eq!(target_peer, "pub-1");
                        assert_eq!(layer, SimulcastLayer::Low);
                    }
                    _ => panic!("expected SetPreferredLayer"),
                },
            },
            TestCase {
                name: "connection_stats keeps loss and bitrate, drops rtt and jitter",
                setup: || ClientMessage::ConnectionStats {
                    target_peer: "pub-1".into(),
                    packet_loss_pct: 3.5,
                    available_incoming_bitrate_bps: Some(900_000),
                    round_trip_time_ms: Some(40.0),
                    jitter_ms: Some(2.0),
                },
                check_result: |cmd| match cmd {
                    Some(RoomCommand::ConnectionStats { report, .. }) => {
                        assert_eq!(report.packet_loss_pct, 3.5);
                        assert_eq!(report.available_incoming_bitrate_bps, Some(900_000));
                    }
                    _ => panic!("expected ConnectionStats"),
                },
            },
            TestCase {
                name: "leave is handled by the session, not the room",
                setup: || ClientMessage::Leave {},
                check_result: |cmd| assert!(cmd.is_none()),
            },
            TestCase {
                name: "join is handled by the session, not the room",
                setup: || ClientMessage::JoinAsPublisher { room: "r".into() },
                check_result: |cmd| assert!(cmd.is_none()),
            },
        ];
        for case in cases {
            println!("case: {}", case.name);
            (case.check_result)(to_command(&"sub-1".to_string(), (case.setup)()));
        }
    }
}
