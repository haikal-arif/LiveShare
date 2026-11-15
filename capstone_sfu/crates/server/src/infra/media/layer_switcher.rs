use std::time::{Duration, Instant};

use common::signaling::SimulcastLayer;
use rtc::rtp;

const KEYFRAME_RETRY: Duration = Duration::from_secs(1);

/// Decides which layer's packets a relay forwards and rewrites their
/// sequence numbers/timestamps so the subscriber sees one continuous
/// stream across layer switches.
#[derive(Debug)]
pub(super) struct LayerSwitcher {
    clock_rate: u32,
    current: Option<SimulcastLayer>,
    pub(super) target: Option<SimulcastLayer>,
    /// When the switch to `target` started waiting for its keyframe.
    pending_since: Option<Instant>,
    seq_offset: u16,
    ts_offset: u32,
    last: Option<(u16, u32, Instant)>,
}

impl LayerSwitcher {
    pub(super) fn new(clock_rate: u32) -> Self {
        Self {
            clock_rate,
            current: None,
            target: None,
            pending_since: None,
            seq_offset: 0,
            ts_offset: 0,
            last: None,
        }
    }

    /// Returns true if this starts a switch that needs a keyframe.
    pub(super) fn set_target(&mut self, layer: SimulcastLayer, now: Instant) -> bool {
        if self.target == Some(layer) {
            return false;
        }
        self.target = Some(layer);
        let switching = self.current.is_some_and(|current| current != layer);
        self.pending_since = switching.then_some(now);
        switching
    }

    /// True (at most once per `KEYFRAME_RETRY`) while a switch is still
    /// waiting on its keyframe.
    pub(super) fn keyframe_overdue(&mut self, now: Instant) -> bool {
        match self.pending_since {
            Some(since) if now.duration_since(since) >= KEYFRAME_RETRY => {
                self.pending_since = Some(now);
                true
            }
            _ => false,
        }
    }

    /// Whether to forward a packet from `layer`, rewriting `header` in place
    /// if so. The very first layer starts without waiting for a keyframe
    /// (the subscriber asks for one as soon as it connects); later switches
    /// wait for one so the decoder never sees a mix of layers.
    pub(super) fn admit(
        &mut self,
        layer: SimulcastLayer,
        header: &mut rtp::header::Header,
        keyframe: bool,
        now: Instant,
    ) -> bool {
        if self.current != Some(layer) {
            let wanted = self.target.is_none_or(|target| target == layer);
            if !wanted || (self.current.is_some() && !keyframe) {
                return false;
            }
            if let Some((last_seq, last_ts, last_at)) = self.last {
                let elapsed = now.duration_since(last_at).as_millis() as u64;
                let ts_gap = (elapsed * self.clock_rate as u64 / 1000).max(1) as u32;
                self.seq_offset = last_seq
                    .wrapping_add(1)
                    .wrapping_sub(header.sequence_number);
                self.ts_offset = last_ts.wrapping_add(ts_gap).wrapping_sub(header.timestamp);
            }
            self.current = Some(layer);
            self.pending_since = None;
        }
        header.sequence_number = header.sequence_number.wrapping_add(self.seq_offset);
        header.timestamp = header.timestamp.wrapping_add(self.ts_offset);
        self.last = Some((header.sequence_number, header.timestamp, now));
        true
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use SimulcastLayer::{High, Low, Mid};

    fn header(seq: u16, ts: u32) -> rtp::header::Header {
        rtp::header::Header {
            sequence_number: seq,
            timestamp: ts,
            ..Default::default()
        }
    }

    #[test]
    fn switcher_starts_immediately_then_switches_only_on_keyframe() {
        let t0 = Instant::now();
        let mut sw = LayerSwitcher::new(90_000);
        assert!(!sw.set_target(High, t0), "first target isn't a switch");

        let mut h = header(100, 9_000);
        assert!(sw.admit(High, &mut h, false, t0));
        assert_eq!(
            (h.sequence_number, h.timestamp),
            (100, 9_000),
            "passthrough"
        );
        assert!(
            !sw.admit(Low, &mut header(5, 50), true, t0),
            "not the target"
        );

        assert!(sw.set_target(Low, t0));
        assert!(
            !sw.admit(Low, &mut header(6, 60), false, t0),
            "waits for keyframe"
        );
        let mut h = header(101, 12_000);
        assert!(
            sw.admit(High, &mut h, false, t0),
            "keeps old layer meanwhile"
        );

        let t1 = t0 + Duration::from_millis(10);
        let mut l = header(7, 70);
        assert!(sw.admit(Low, &mut l, true, t1));
        assert_eq!(l.sequence_number, 102, "sequence continues across switch");
        assert_eq!(
            l.timestamp,
            12_000 + 900,
            "timestamp advances by elapsed time"
        );
        assert!(
            !sw.admit(High, &mut header(102, 15_000), true, t1),
            "old layer dropped"
        );

        let mut l = header(8, 3_070);
        assert!(sw.admit(Low, &mut l, false, t1));
        assert_eq!((l.sequence_number, l.timestamp), (103, 15_900));
    }

    #[test]
    fn switcher_retries_keyframe_while_switch_is_pending() {
        let t0 = Instant::now();
        let mut sw = LayerSwitcher::new(90_000);
        sw.set_target(High, t0);
        sw.admit(High, &mut header(1, 1), false, t0);
        sw.set_target(Mid, t0);
        assert!(!sw.keyframe_overdue(t0 + Duration::from_millis(500)));
        assert!(sw.keyframe_overdue(t0 + Duration::from_secs(1)));
        assert!(!sw.keyframe_overdue(t0 + Duration::from_millis(1500)));
        sw.admit(Mid, &mut header(9, 9), true, t0 + Duration::from_secs(2));
        assert!(!sw.keyframe_overdue(t0 + Duration::from_secs(5)));
    }
}
