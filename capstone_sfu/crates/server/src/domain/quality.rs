//! Per-pairing connection quality tracking (`API-Contract.md` section 3,
//! "Connection quality & simulcast"). The subscriber picks a layer; this
//! steps the pairing down from it when `connection_stats` show sustained
//! loss. It never steps back up on its own: a new `set_preferred_layer`
//! from the subscriber is the retry.

use common::signaling::{ConnectionQuality, SimulcastLayer};

/// Suggested bitrate of the publisher's `h` layer.
const HIGH_LAYER_BPS: u64 = 2_500_000;
/// Suggested bitrate of the publisher's `m` layer.
const MID_LAYER_BPS: u64 = 800_000;
const POOR_LOSS_PCT: f64 = 2.0;
const BAD_LOSS_PCT: f64 = 10.0;
/// Consecutive lossy reports needed before stepping down.
const DOWNGRADE_REPORTS: u32 = 2;

/// One `connection_stats` report, as far as quality is concerned.
#[derive(Debug, Clone, Copy)]
pub struct StatsReport {
    pub available_incoming_bitrate_bps: Option<u64>,
    pub packet_loss_pct: f64,
}

/// What a single report says. A missing bitrate is judged on loss alone.
pub fn classify(report: &StatsReport) -> ConnectionQuality {
    let bitrate = report.available_incoming_bitrate_bps;
    if report.packet_loss_pct > BAD_LOSS_PCT || bitrate.is_some_and(|b| b < MID_LAYER_BPS) {
        ConnectionQuality::Bad
    } else if report.packet_loss_pct >= POOR_LOSS_PCT || bitrate.is_some_and(|b| b < HIGH_LAYER_BPS)
    {
        ConnectionQuality::Poor
    } else {
        ConnectionQuality::Good
    }
}

fn rank(quality: ConnectionQuality) -> u8 {
    match quality {
        ConnectionQuality::Bad => 0,
        ConnectionQuality::Poor => 1,
        ConnectionQuality::Good => 2,
    }
}

fn step_down(layer: SimulcastLayer) -> SimulcastLayer {
    match layer {
        SimulcastLayer::High => SimulcastLayer::Mid,
        _ => SimulcastLayer::Low,
    }
}

/// A pairing starts at `high`/`good`: the subscriber's pick until it says
/// otherwise.
#[derive(Debug)]
pub struct QualityTracker {
    /// The layer this pairing may currently be forwarded.
    layer: SimulcastLayer,
    /// `good` while at the subscriber's pick; otherwise how bad the loss
    /// was that caused the last step down.
    quality: ConnectionQuality,
    lossy_streak: u32,
    /// Mildest of the current streak, so a single spike among milder
    /// reports doesn't count as `bad`.
    streak_quality: ConnectionQuality,
}

impl Default for QualityTracker {
    fn default() -> Self {
        Self {
            layer: SimulcastLayer::High,
            quality: ConnectionQuality::Good,
            lossy_streak: 0,
            streak_quality: ConnectionQuality::Good,
        }
    }
}

impl QualityTracker {
    pub fn quality(&self) -> ConnectionQuality {
        self.quality
    }

    pub fn layer(&self) -> SimulcastLayer {
        self.layer
    }

    /// The subscriber picked `layer`: forward it straight away, forgetting
    /// any earlier step down. Picking a higher layer is how a subscriber
    /// retries after being downgraded.
    pub fn set_preferred(&mut self, layer: SimulcastLayer) {
        *self = Self {
            layer,
            ..Self::default()
        };
    }

    /// Feeds one report in; returns true if the layer or quality changed.
    /// Sustained `poor` loss steps down one layer from the current one,
    /// sustained `bad` loss goes straight to `low`.
    pub fn update(&mut self, report: &StatsReport) -> bool {
        let observed = classify(report);
        if observed == ConnectionQuality::Good {
            self.lossy_streak = 0;
            return false;
        }
        if self.lossy_streak == 0 || rank(observed) > rank(self.streak_quality) {
            self.streak_quality = observed;
        }
        self.lossy_streak += 1;
        if self.lossy_streak < DOWNGRADE_REPORTS {
            return false;
        }
        // Judge the next layer on fresh reports only.
        self.lossy_streak = 0;

        let before = (self.layer, self.quality);
        self.layer = match self.streak_quality {
            ConnectionQuality::Bad => SimulcastLayer::Low,
            _ => step_down(self.layer),
        };
        self.quality = self.streak_quality;
        (self.layer, self.quality) != before
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ConnectionQuality::{Bad, Good, Poor};
    use SimulcastLayer::{High, Low, Mid};

    fn report(bitrate: Option<u64>, loss: f64) -> StatsReport {
        StatsReport {
            available_incoming_bitrate_bps: bitrate,
            packet_loss_pct: loss,
        }
    }

    fn loss(pct: f64) -> StatsReport {
        report(None, pct)
    }

    #[test]
    fn classify_follows_contract_thresholds() {
        assert_eq!(classify(&report(Some(3_000_000), 0.5)), Good);
        assert_eq!(classify(&report(None, 1.9)), Good);
        assert_eq!(classify(&report(Some(3_000_000), 2.0)), Poor);
        assert_eq!(classify(&report(Some(1_000_000), 0.0)), Poor);
        assert_eq!(classify(&report(Some(3_000_000), 10.5)), Bad);
        assert_eq!(classify(&report(Some(500_000), 0.0)), Bad);
    }

    #[test]
    fn single_lossy_report_does_not_downgrade() {
        let mut tracker = QualityTracker::default();
        assert!(!tracker.update(&loss(50.0)));
        assert!(!tracker.update(&loss(0.0)));
        assert!(!tracker.update(&loss(50.0)));
        assert_eq!((tracker.layer(), tracker.quality()), (High, Good));
    }

    #[test]
    fn sustained_poor_loss_steps_down_one_layer_at_a_time() {
        let mut tracker = QualityTracker::default();
        tracker.update(&loss(5.0));
        assert!(tracker.update(&loss(5.0)));
        assert_eq!((tracker.layer(), tracker.quality()), (Mid, Poor));

        assert!(!tracker.update(&loss(5.0)), "needs a fresh streak");
        assert!(tracker.update(&loss(5.0)));
        assert_eq!(tracker.layer(), Low);
    }

    #[test]
    fn sustained_bad_loss_goes_straight_to_low_via_the_milder_report() {
        let mut tracker = QualityTracker::default();
        tracker.update(&loss(50.0));
        tracker.update(&loss(5.0));
        assert_eq!((tracker.layer(), tracker.quality()), (Mid, Poor));

        let mut tracker = QualityTracker::default();
        tracker.update(&loss(50.0));
        assert!(tracker.update(&loss(50.0)));
        assert_eq!((tracker.layer(), tracker.quality()), (Low, Bad));
    }

    #[test]
    fn never_steps_back_up_on_its_own() {
        let mut tracker = QualityTracker::default();
        tracker.update(&loss(50.0));
        tracker.update(&loss(50.0));
        for _ in 0..100 {
            assert!(!tracker.update(&loss(0.0)));
        }
        assert_eq!((tracker.layer(), tracker.quality()), (Low, Bad));
    }

    #[test]
    fn a_pick_is_a_retry_that_clears_the_downgrade() {
        let mut tracker = QualityTracker::default();
        tracker.update(&loss(50.0));
        tracker.update(&loss(50.0));

        tracker.set_preferred(High);
        assert_eq!((tracker.layer(), tracker.quality()), (High, Good));
        assert!(!tracker.update(&loss(5.0)), "old streak is forgotten");
    }

    #[test]
    fn a_lower_pick_is_stepped_down_from_too() {
        let mut tracker = QualityTracker::default();
        tracker.set_preferred(Mid);
        tracker.update(&loss(5.0));
        assert!(tracker.update(&loss(5.0)));
        assert_eq!((tracker.layer(), tracker.quality()), (Low, Poor));
    }
}
