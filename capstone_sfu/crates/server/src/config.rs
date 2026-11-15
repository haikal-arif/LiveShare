use std::ops::RangeInclusive;

use common::signaling::IceServer;

pub struct Config {
    /// Address this single process binds for both the public `/ws`
    /// signaling endpoint and the internal-only `/internal/*` REST API.
    /// The two are only actually separated by the reverse proxy /
    /// network topology in front of this process, not by port.
    pub bind_addr: String,
    /// UDP ports WebRTC media is bound to, one per `PeerConnection`, so a
    /// firewall only has to open this range. `None` lets the OS pick.
    pub udp_ports: Option<RangeInclusive<u16>>,
    /// Addresses advertised to clients as the SFU's ICE host candidate in
    /// place of the interface address. Needed when the SFU sits behind NAT,
    /// e.g. in a container whose own address no browser can reach. Empty =
    /// advertise the real interface addresses.
    pub public_ips: Vec<String>,
    /// Handed to clients verbatim in the `joined` message. Defaults to
    /// the same public STUN server `capstone-be` defaults to, so both
    /// services agree absent explicit configuration.
    pub ice_servers: Vec<IceServer>,
    /// Most peers (sharers and watchers together) one room accepts. `None`
    /// = no limit. Joins past it are refused until someone leaves.
    pub max_participants: Option<usize>,
}

impl Config {
    pub fn load() -> Self {
        let bind_addr = std::env::var("BIND_ADDR").unwrap_or_else(|_| "0.0.0.0:9000".to_string());

        let udp_ports =
            std::env::var("SFU_UDP_PORTS").unwrap_or_else(|_| "50000-50199".to_string());
        let udp_ports = Some(
            parse_port_range(&udp_ports)
                .expect("SFU_UDP_PORTS must be a port or an inclusive range like 50000-50199"),
        );

        let public_ips = parse_ip_list(&std::env::var("SFU_PUBLIC_IPS").unwrap_or_default());

        let ice_servers_json = std::env::var("ICE_SERVERS_JSON")
            .unwrap_or_else(|_| r#"[{"urls":["stun:stun.l.google.com:19302"]}]"#.to_string());
        let mut ice_servers: Vec<IceServer> = serde_json::from_str(&ice_servers_json)
            .expect("ICE_SERVERS_JSON must be a JSON array of { urls: [...] }");
        ice_servers.extend(turn_server(
            &std::env::var("TURN_URLS").unwrap_or_default(),
            &std::env::var("TURN_USERNAME").unwrap_or_default(),
            &std::env::var("TURN_CREDENTIAL").unwrap_or_default(),
        ));

        let max_participants = parse_max_participants(
            &std::env::var("MAX_PARTICIPANTS").unwrap_or_default(),
        )
        .expect("MAX_PARTICIPANTS must be a non-negative whole number (0 or empty = no limit)");

        Self {
            bind_addr,
            udp_ports,
            public_ips,
            ice_servers,
            max_participants,
        }
    }
}

/// The TURN entry from `TURN_URLS` (comma-separated) plus its credentials.
/// `None` when no URLs are given, so TURN is simply off.
fn turn_server(urls: &str, username: &str, credential: &str) -> Option<IceServer> {
    let urls: Vec<String> = urls
        .split(',')
        .map(str::trim)
        .filter(|u| !u.is_empty())
        .map(str::to_string)
        .collect();
    if urls.is_empty() {
        return None;
    }
    Some(IceServer {
        urls,
        username: Some(username.to_string()),
        credential: Some(credential.to_string()),
    })
}

/// Parses `MAX_PARTICIPANTS`. Empty or `0` means no limit.
fn parse_max_participants(s: &str) -> Option<Option<usize>> {
    let s = s.trim();
    if s.is_empty() {
        return Some(None);
    }
    match s.parse::<usize>().ok()? {
        0 => Some(None),
        n => Some(Some(n)),
    }
}

/// Parses a comma-separated list of IPs, ignoring blanks.
fn parse_ip_list(s: &str) -> Vec<String> {
    s.split(',')
        .map(str::trim)
        .filter(|ip| !ip.is_empty())
        .map(str::to_string)
        .collect()
}

/// Parses `"50000"` or `"50000-50199"`.
fn parse_port_range(s: &str) -> Option<RangeInclusive<u16>> {
    let (start, end) = s.split_once('-').unwrap_or((s, s));
    let start: u16 = start.trim().parse().ok()?;
    let end: u16 = end.trim().parse().ok()?;
    (start != 0 && start <= end).then_some(start..=end)
}

#[cfg(test)]
mod tests {
    use super::{parse_ip_list, parse_max_participants, parse_port_range, turn_server};

    #[test]
    fn builds_turn_entry_only_when_urls_are_set() {
        assert!(turn_server("", "u", "p").is_none());
        assert!(turn_server(" , ", "u", "p").is_none());

        let server = turn_server("turn:a.example:3478?transport=udp, turn:a.example:3478?transport=tcp", "u", "p")
            .expect("turn entry");
        assert_eq!(
            server.urls,
            vec![
                "turn:a.example:3478?transport=udp",
                "turn:a.example:3478?transport=tcp"
            ]
        );
        assert_eq!(server.username.as_deref(), Some("u"));
        assert_eq!(server.credential.as_deref(), Some("p"));
    }

    #[test]
    fn parses_max_participants() {
        assert_eq!(parse_max_participants(""), Some(None));
        assert_eq!(parse_max_participants("0"), Some(None));
        assert_eq!(parse_max_participants(" 12 "), Some(Some(12)));
        assert_eq!(parse_max_participants("-1"), None);
        assert_eq!(parse_max_participants("many"), None);
    }

    #[test]
    fn parses_ip_list_and_skips_blanks() {
        assert_eq!(parse_ip_list(""), Vec::<String>::new());
        assert_eq!(parse_ip_list("10.0.0.5"), vec!["10.0.0.5"]);
        assert_eq!(
            parse_ip_list(" 10.0.0.5, ,192.168.1.2 "),
            vec!["10.0.0.5", "192.168.1.2"]
        );
    }

    #[test]
    fn parses_single_port_and_range() {
        assert_eq!(parse_port_range("50000"), Some(50000..=50000));
        assert_eq!(parse_port_range("50000-50199"), Some(50000..=50199));
    }

    #[test]
    fn rejects_invalid_ranges() {
        assert_eq!(parse_port_range("0"), None);
        assert_eq!(parse_port_range("50199-50000"), None);
        assert_eq!(parse_port_range("abc"), None);
    }
}
