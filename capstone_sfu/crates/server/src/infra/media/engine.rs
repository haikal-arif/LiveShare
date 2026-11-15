use std::ops::RangeInclusive;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;

use common::signaling::IceServer;
use rtc::interceptor::Slot;
use webrtc::peer_connection::{
    register_default_interceptors, MediaEngine, PeerConnection, PeerConnectionBuilder,
    PeerConnectionEventHandler, RTCConfigurationBuilder, RTCIceServer, Registry,
};

use super::keyframe::KeyframeRequestTagger;

pub type SharedPeerConnection = Arc<dyn PeerConnection>;

/// Shared, per-process WebRTC configuration used to build every
/// `PeerConnection` the SFU owns.
pub struct MediaContext {
    ice_servers: Vec<RTCIceServer>,
    /// UDP ports media sockets are bound to, one per `PeerConnection`, so
    /// the firewall only needs this range open. `None` = OS-picked ports.
    udp_ports: Option<RangeInclusive<u16>>,
    /// Advertised in place of the interface address as the host candidate
    /// (see `Config::public_ips`). Empty = advertise the real addresses.
    public_ips: Arc<[String]>,
    next_port: AtomicUsize,
}

impl MediaContext {
    pub fn new(ice_servers: &[IceServer], udp_ports: Option<RangeInclusive<u16>>) -> Self {
        Self {
            ice_servers: ice_servers
                .iter()
                .map(|s| RTCIceServer {
                    urls: s.urls.clone(),
                    username: s.username.clone().unwrap_or_default(),
                    credential: s.credential.clone().unwrap_or_default(),
                    ..Default::default()
                })
                .collect(),
            udp_ports,
            public_ips: Arc::from([]),
            next_port: AtomicUsize::new(0),
        }
    }

    /// Advertises `ips` as the host candidate of every connection, for an
    /// SFU behind NAT. The rewrite happens where candidates are handed to
    /// clients (`session.rs`): webrtc-rs 0.21 derives host candidates from
    /// the socket's own address and ignores `SettingEngine::nat_1to1_ips`.
    pub fn with_public_ips(mut self, ips: Vec<String>) -> Self {
        self.public_ips = ips.into();
        self
    }

    pub(super) fn public_ips(&self) -> Arc<[String]> {
        self.public_ips.clone()
    }

    /// Picks the UDP address for the next `PeerConnection`, rotating through
    /// `udp_ports`. The connection binds asynchronously and only logs a
    /// failed bind, so ports still held by another connection are skipped
    /// here by test-binding them first.
    fn next_udp_addr(&self) -> webrtc::error::Result<String> {
        let Some(ports) = &self.udp_ports else {
            return Ok("0.0.0.0:0".to_string());
        };
        let count = (ports.end() - ports.start()) as usize + 1;
        for _ in 0..count {
            let offset = self.next_port.fetch_add(1, Ordering::Relaxed) % count;
            let port = ports.start() + offset as u16;
            if std::net::UdpSocket::bind(("0.0.0.0", port)).is_ok() {
                return Ok(format!("0.0.0.0:{port}"));
            }
        }
        Err(std::io::Error::new(
            std::io::ErrorKind::AddrInUse,
            format!(
                "all SFU UDP ports {}-{} are in use",
                ports.start(),
                ports.end()
            ),
        )
        .into())
    }

    pub async fn new_peer_connection(
        &self,
        handler: Arc<dyn PeerConnectionEventHandler>,
    ) -> webrtc::error::Result<SharedPeerConnection> {
        // A `MediaEngine`/interceptor `Registry` is consumed per connection,
        // so they're rebuilt each time rather than shared.
        let mut media_engine = MediaEngine::default();
        media_engine.register_default_codecs()?;
        let registry = register_default_interceptors(Registry::new(), &mut media_engine)?
            .with(Slot::from(14_000), KeyframeRequestTagger::default());
        let config = RTCConfigurationBuilder::new()
            .with_ice_servers(self.ice_servers.clone())
            .build();

        let pc = PeerConnectionBuilder::new()
            .with_configuration(config)
            .with_media_engine(media_engine)
            .with_interceptor_registry(registry)
            .with_handler(handler)
            .with_udp_addrs(vec![self.next_udp_addr()?])
            .build()
            .await?;
        Ok(Arc::new(pc))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn next_udp_addr_uses_configured_port_and_skips_busy_ones() {
        let socket = std::net::UdpSocket::bind("0.0.0.0:0").unwrap();
        let port = socket.local_addr().unwrap().port();
        let media = MediaContext::new(&[], Some(port..=port));

        assert!(
            media.next_udp_addr().is_err(),
            "port is held, so none are free"
        );
        drop(socket);
        assert_eq!(media.next_udp_addr().unwrap(), format!("0.0.0.0:{port}"));
    }

    #[test]
    fn next_udp_addr_defaults_to_ephemeral() {
        let media = MediaContext::new(&[], None);
        assert_eq!(media.next_udp_addr().unwrap(), "0.0.0.0:0");
    }
}
