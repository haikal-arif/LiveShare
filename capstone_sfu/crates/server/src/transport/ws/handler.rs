use std::net::SocketAddr;

use axum::extract::ws::WebSocketUpgrade;
use axum::extract::{ConnectInfo, State};
use axum::http::HeaderMap;
use axum::response::IntoResponse;
use axum::routing::get;
use axum::Router;

use super::session::run;
use crate::transport::state::AppState;

pub fn router() -> Router<AppState> {
    Router::new().route("/ws", get(upgrade))
}

async fn upgrade(
    ws: WebSocketUpgrade,
    State(state): State<AppState>,
    headers: HeaderMap,
    connect: Option<ConnectInfo<SocketAddr>>,
) -> impl IntoResponse {
    let ip = client_ip(&headers, connect.map(|ConnectInfo(addr)| addr));
    ws.on_upgrade(move |socket| run(socket, state.rooms, ip))
}

/// The address the client connected from: the `X-Real-IP` header the
/// reverse proxy sets, else the socket's own peer address.
fn client_ip(headers: &HeaderMap, peer: Option<SocketAddr>) -> Option<String> {
    headers
        .get("x-real-ip")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|ip| !ip.is_empty())
        .map(String::from)
        .or_else(|| peer.map(|addr| addr.ip().to_string()))
}

#[cfg(test)]
mod tests {
    use super::*;

    struct TestCase {
        name: &'static str,
        setup: fn() -> (HeaderMap, Option<SocketAddr>),
        check_result: fn(Option<String>),
    }

    #[test]
    fn picks_client_ip() {
        let cases = [
            TestCase {
                name: "header wins over socket address",
                setup: || {
                    let mut headers = HeaderMap::new();
                    headers.insert("x-real-ip", "203.0.113.7".parse().unwrap());
                    (headers, Some("10.0.0.1:5000".parse().unwrap()))
                },
                check_result: |ip| assert_eq!(ip.as_deref(), Some("203.0.113.7")),
            },
            TestCase {
                name: "falls back to socket address",
                setup: || (HeaderMap::new(), Some("10.0.0.1:5000".parse().unwrap())),
                check_result: |ip| assert_eq!(ip.as_deref(), Some("10.0.0.1")),
            },
            TestCase {
                name: "blank header falls back to socket address",
                setup: || {
                    let mut headers = HeaderMap::new();
                    headers.insert("x-real-ip", " ".parse().unwrap());
                    (headers, Some("10.0.0.1:5000".parse().unwrap()))
                },
                check_result: |ip| assert_eq!(ip.as_deref(), Some("10.0.0.1")),
            },
            TestCase {
                name: "unknown when neither is available",
                setup: || (HeaderMap::new(), None),
                check_result: |ip| assert_eq!(ip, None),
            },
        ];
        for case in cases {
            println!("case: {}", case.name);
            let (headers, peer) = (case.setup)();
            (case.check_result)(client_ip(&headers, peer));
        }
    }
}
