use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::Json;
use common::internal::{ErrorBody, ErrorDetail};

use crate::domain::error::DomainError;

/// A failed internal REST call, rendered as `{"error":{"code","message"}}`.
pub struct ApiError(pub DomainError);

impl From<DomainError> for ApiError {
    fn from(err: DomainError) -> Self {
        ApiError(err)
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        let status = match self.0 {
            DomainError::PeerNotFound(_) => StatusCode::NOT_FOUND,
            DomainError::Internal(_) => StatusCode::INTERNAL_SERVER_ERROR,
            _ => StatusCode::BAD_REQUEST,
        };
        let code = serde_json::to_value(self.0.code())
            .ok()
            .and_then(|value| value.as_str().map(String::from))
            .unwrap_or_else(|| "internal".to_string());
        let body = ErrorBody {
            error: ErrorDetail {
                code,
                message: self.0.to_string(),
            },
        };
        (status, Json(body)).into_response()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::body::to_bytes;

    struct TestCase {
        name: &'static str,
        setup: fn() -> DomainError,
        status: StatusCode,
        code: &'static str,
    }

    #[tokio::test]
    async fn maps_each_error_to_status_and_body() {
        let cases = [
            TestCase {
                name: "missing peer is 404",
                setup: || DomainError::PeerNotFound("publisher not found".into()),
                status: StatusCode::NOT_FOUND,
                code: "peer_not_found",
            },
            TestCase {
                name: "internal failure is 500",
                setup: || DomainError::Internal("failed to create peer connection".into()),
                status: StatusCode::INTERNAL_SERVER_ERROR,
                code: "internal",
            },
            TestCase {
                name: "other errors are 400",
                setup: || DomainError::InvalidSdp("bad sdp".into()),
                status: StatusCode::BAD_REQUEST,
                code: "invalid_sdp",
            },
        ];

        for case in cases {
            println!("case: {}", case.name);
            let err = (case.setup)();
            let message = err.to_string();
            let response = ApiError(err).into_response();
            assert_eq!(response.status(), case.status, "{}", case.name);

            let bytes = to_bytes(response.into_body(), usize::MAX).await.unwrap();
            let body: ErrorBody = serde_json::from_slice(&bytes).unwrap();
            assert_eq!(body.error.code, case.code, "{}", case.name);
            assert_eq!(body.error.message, message, "{}", case.name);
        }
    }
}
