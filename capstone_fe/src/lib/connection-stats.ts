import type { ConnectionStats, PeerId } from './contract';
import type { SignalingClient } from './signaling';

/** How often a subscriber reports each pairing's stats to the SFU (API-Contract.md). */
export const STATS_INTERVAL_MS = 2000;

/** Running packet counters, so loss is measured per interval rather than since connect. */
export interface PacketCounters {
	packetsReceived: number;
	packetsLost: number;
}

type StatsEntry = Record<string, unknown> & { type: string; id: string };

function num(value: unknown): number | undefined {
	return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

/** The candidate pair ICE is actually using, found via the transport or, failing that, by flags. */
function selectedCandidatePair(entries: StatsEntry[]): StatsEntry | undefined {
	const byId = new Map(entries.map((e) => [e.id, e]));
	for (const e of entries) {
		if (e.type === 'transport' && typeof e.selectedCandidatePairId === 'string') {
			const pair = byId.get(e.selectedCandidatePairId);
			if (pair) return pair;
		}
	}
	return entries.find(
		(e) => e.type === 'candidate-pair' && e.nominated === true && e.state === 'succeeded'
	);
}

/**
 * Turns one `getStats()` report into a `connection_stats` payload. Loss is
 * taken from the video inbound stream (audio if there's no video) since
 * `previous`. Returns `stats: null` when no packets arrived in the interval:
 * there's nothing to judge, and reporting 0% loss would claim a good link.
 *
 * `available_incoming_bitrate_bps` is deliberately left out. Chrome's
 * estimate only tracks what it's currently being sent (the SFU doesn't
 * probe), so on a perfect link it sits below the high layer's bitrate and
 * the SFU would downgrade the subscriber, then never upgrade it again.
 * Loss is what actually signals congestion here.
 */
export function summarizeStats(
	report: RTCStatsReport,
	targetPeer: PeerId,
	previous: PacketCounters | null
): { stats: ConnectionStats | null; counters: PacketCounters | null } {
	const entries: StatsEntry[] = [];
	report.forEach((entry) => entries.push(entry as StatsEntry));

	const inbound = entries.filter((e) => e.type === 'inbound-rtp');
	const stream = inbound.find((e) => e.kind === 'video') ?? inbound.find((e) => e.kind === 'audio');
	if (!stream) return { stats: null, counters: previous };

	const counters: PacketCounters = {
		packetsReceived: num(stream.packetsReceived) ?? 0,
		// Can dip negative when duplicates arrive; that's not negative loss.
		packetsLost: Math.max(0, num(stream.packetsLost) ?? 0)
	};
	const received = counters.packetsReceived - (previous?.packetsReceived ?? 0);
	const lost = Math.max(0, counters.packetsLost - (previous?.packetsLost ?? 0));
	if (received <= 0) return { stats: null, counters };

	const stats: ConnectionStats = {
		target_peer: targetPeer,
		packet_loss_pct: (lost / (lost + received)) * 100
	};
	const pair = selectedCandidatePair(entries);
	const rtt = num(pair?.currentRoundTripTime);
	if (rtt !== undefined) stats.round_trip_time_ms = rtt * 1000;
	const jitter = num(stream.jitter);
	if (jitter !== undefined) stats.jitter_ms = jitter * 1000;

	return { stats, counters };
}

/**
 * Sends `connection_stats` for one subscriber pairing every
 * `STATS_INTERVAL_MS` until the returned stop function is called. The SFU
 * uses these to pick which simulcast layer this subscriber gets.
 */
export function startStatsReporter(
	pc: RTCPeerConnection,
	targetPeer: PeerId,
	signaling: SignalingClient
): () => void {
	let previous: PacketCounters | null = null;
	let stopped = false;

	const timer = setInterval(async () => {
		let report: RTCStatsReport;
		try {
			report = await pc.getStats();
		} catch {
			// The connection is closing; the next tick (if any) will try again.
			return;
		}
		if (stopped) return;
		const { stats, counters } = summarizeStats(report, targetPeer, previous);
		previous = counters;
		if (stats) signaling.send({ type: 'connection_stats', ...stats });
	}, STATS_INTERVAL_MS);

	return () => {
		stopped = true;
		clearInterval(timer);
	};
}
