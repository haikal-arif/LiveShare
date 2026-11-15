use common::signaling::PeerId;
use uuid::Uuid;

use crate::domain::peer::PeerRole;

/// Generates a short, human-scannable peer id: `pub-<8 hex>` /
/// `sub-<8 hex>`, matching the `pub-abc123` / `sub-xyz789` shape used in
/// `API-Contract.md`'s examples.
pub fn generate(role: PeerRole) -> PeerId {
    let prefix = match role {
        PeerRole::Publisher => "pub",
        PeerRole::Subscriber => "sub",
    };
    let suffix = Uuid::new_v4().simple().to_string();
    format!("{prefix}-{}", &suffix[..8])
}
