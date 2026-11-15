use common::signaling::{ErrorCode, ServerMessage};

/// A failure the SFU reports back to the peer that caused it. Each variant
/// maps to one wire `ErrorCode`; the message is the human-readable `reason`.
#[derive(Debug, Clone, PartialEq, Eq, thiserror::Error)]
pub enum DomainError {
    #[error("{0}")]
    InvalidMessage(String),
    #[error("{0}")]
    NotJoined(String),
    #[error("{0}")]
    AlreadyJoined(String),
    #[error("{0}")]
    PeerNotFound(String),
    #[error("{0}")]
    WrongRole(String),
    #[error("{0}")]
    InvalidSdp(String),
    #[error("{0}")]
    InvalidState(String),
    #[error("{0}")]
    Internal(String),
}

impl DomainError {
    pub fn code(&self) -> ErrorCode {
        match self {
            Self::InvalidMessage(_) => ErrorCode::InvalidMessage,
            Self::NotJoined(_) => ErrorCode::NotJoined,
            Self::AlreadyJoined(_) => ErrorCode::AlreadyJoined,
            Self::PeerNotFound(_) => ErrorCode::PeerNotFound,
            Self::WrongRole(_) => ErrorCode::WrongRole,
            Self::InvalidSdp(_) => ErrorCode::InvalidSdp,
            Self::InvalidState(_) => ErrorCode::InvalidState,
            Self::Internal(_) => ErrorCode::Internal,
        }
    }
}

impl From<DomainError> for ServerMessage {
    fn from(err: DomainError) -> Self {
        ServerMessage::Error {
            code: err.code(),
            reason: err.to_string(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    struct TestCase {
        name: &'static str,
        setup: fn() -> DomainError,
        check_result: fn(ServerMessage),
    }

    #[test]
    fn converts_to_error_message_with_code_and_reason() {
        let cases = [
            TestCase {
                name: "wrong role",
                setup: || DomainError::WrongRole("only a subscriber may request a track".into()),
                check_result: |msg| match msg {
                    ServerMessage::Error { code, reason } => {
                        assert_eq!(code, ErrorCode::WrongRole);
                        assert_eq!(reason, "only a subscriber may request a track");
                    }
                    other => panic!("expected error, got {other:?}"),
                },
            },
            TestCase {
                name: "invalid sdp",
                setup: || DomainError::InvalidSdp("invalid sdp_offer: bad".into()),
                check_result: |msg| match msg {
                    ServerMessage::Error { code, .. } => assert_eq!(code, ErrorCode::InvalidSdp),
                    other => panic!("expected error, got {other:?}"),
                },
            },
        ];
        for case in cases {
            let msg: ServerMessage = (case.setup)().into();
            println!("case: {}", case.name);
            (case.check_result)(msg);
        }
    }
}
