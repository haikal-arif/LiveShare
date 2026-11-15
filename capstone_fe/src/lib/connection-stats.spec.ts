import { describe, expect, it } from 'vitest';
import { summarizeStats } from './connection-stats';

function report(...entries: Record<string, unknown>[]): RTCStatsReport {
	return new Map(entries.map((e) => [e.id as string, e])) as unknown as RTCStatsReport;
}

const video = (packetsReceived: number, packetsLost: number, extra = {}) => ({
	id: 'in-video',
	type: 'inbound-rtp',
	kind: 'video',
	packetsReceived,
	packetsLost,
	...extra
});

describe('summarizeStats', () => {
	it('measures loss per interval, not since connect', () => {
		const first = summarizeStats(report(video(900, 100)), 'pub-1', null);
		expect(first.stats?.packet_loss_pct).toBe(10);

		const second = summarizeStats(report(video(1000, 100)), 'pub-1', first.counters);
		expect(second.stats?.packet_loss_pct).toBe(0);

		const third = summarizeStats(report(video(1095, 105)), 'pub-1', second.counters);
		expect(third.stats?.packet_loss_pct).toBe(5);
	});

	it('prefers the video stream over audio', () => {
		const audio = {
			id: 'in-audio',
			type: 'inbound-rtp',
			kind: 'audio',
			packetsReceived: 50,
			packetsLost: 50
		};
		const { stats } = summarizeStats(report(audio, video(100, 0)), 'pub-1', null);
		expect(stats?.packet_loss_pct).toBe(0);
	});

	it('reads RTT from the selected candidate pair and jitter from the stream', () => {
		const { stats } = summarizeStats(
			report(
				video(100, 0, { jitter: 0.012 }),
				{ id: 't', type: 'transport', selectedCandidatePairId: 'cp-2' },
				{
					id: 'cp-1',
					type: 'candidate-pair',
					availableIncomingBitrate: 1,
					currentRoundTripTime: 9
				},
				{
					id: 'cp-2',
					type: 'candidate-pair',
					availableIncomingBitrate: 1_200_000.4,
					currentRoundTripTime: 0.045
				}
			),
			'pub-1',
			null
		);
		expect(stats).toEqual({
			target_peer: 'pub-1',
			packet_loss_pct: 0,
			round_trip_time_ms: 45,
			jitter_ms: 12
		});
	});

	it('never reports the bitrate estimate, and omits fields the browser does not expose', () => {
		const { stats } = summarizeStats(report(video(100, 0)), 'pub-1', null);
		expect(stats).toEqual({ target_peer: 'pub-1', packet_loss_pct: 0 });
	});

	it('skips the report when nothing arrived in the interval', () => {
		expect(summarizeStats(report(), 'pub-1', null).stats).toBeNull();
		const first = summarizeStats(report(video(100, 0)), 'pub-1', null);
		expect(summarizeStats(report(video(100, 0)), 'pub-1', first.counters).stats).toBeNull();
	});
});
