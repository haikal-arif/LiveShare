import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ClientMessage, ServerMessage } from './contract';
import type { SignalingClient } from './signaling';
import { STATS_INTERVAL_MS } from './connection-stats';
import {
	createPublisherConnection,
	createSubscriberTrackConnection,
	SIMULCAST_ENCODINGS
} from './webrtc';

/** Minimal fake standing in for the browser RTCPeerConnection. */
class FakeRTCPeerConnection {
	static instances: FakeRTCPeerConnection[] = [];

	onnegotiationneeded: (() => void) | null = null;
	onicecandidate: ((event: { candidate: unknown }) => void) | null = null;
	ontrack: ((event: { streams: MediaStream[] }) => void) | null = null;
	localDescription: { type: string; sdp: string } | null = null;

	addTrack = vi.fn();
	addTransceiver = vi.fn();
	getStats = vi.fn(async () => new Map() as unknown as RTCStatsReport);
	getSenders = vi.fn(() => [] as { track: { kind: string } | null }[]);
	createOffer = vi.fn(async () => ({ type: 'offer', sdp: 'offer-sdp' }));
	createAnswer = vi.fn(async () => ({ type: 'answer', sdp: 'answer-sdp' }));
	setLocalDescription = vi.fn(async (desc: { type: string; sdp: string }) => {
		this.localDescription = desc;
	});
	setRemoteDescription = vi.fn(async () => {});
	addIceCandidate = vi.fn(async () => {});
	close = vi.fn();

	config: unknown;

	constructor(config: unknown) {
		this.config = config;
		FakeRTCPeerConnection.instances.push(this);
	}
}

/** Fake SignalingClient that lets tests manually fire server messages. */
function createFakeSignaling() {
	const handlers = new Map<string, ((msg: ServerMessage) => void)[]>();
	const offMocks: ReturnType<typeof vi.fn>[] = [];

	const client: SignalingClient = {
		send: vi.fn(),
		on: vi.fn((type, handler) => {
			const list = handlers.get(type) ?? [];
			list.push(handler as (msg: ServerMessage) => void);
			handlers.set(type, list);
			const off = vi.fn(() => {
				const idx = list.indexOf(handler as (msg: ServerMessage) => void);
				if (idx >= 0) list.splice(idx, 1);
			});
			offMocks.push(off);
			return off;
		}),
		close: vi.fn()
	};

	return {
		client,
		offMocks,
		/** Runs all matching handlers and waits for their (possibly async) work to settle. */
		async emit(msg: ServerMessage) {
			await Promise.all(handlers.get(msg.type)?.map((handler) => handler(msg)) ?? []);
		}
	};
}

function fakeStream(kinds: Array<'audio' | 'video'>): MediaStream {
	const tracks = kinds.map((kind) => ({ kind }));
	return { getTracks: () => tracks } as unknown as MediaStream;
}

describe('createPublisherConnection', () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		FakeRTCPeerConnection.instances = [];
	});

	it('sends audio as-is and video as h/m/l simulcast layers', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const stream = fakeStream(['audio', 'video']);
		const { client } = createFakeSignaling();

		createPublisherConnection([{ urls: ['stun:stun.example.com:3478'] }], stream, client);

		const pc = FakeRTCPeerConnection.instances[0];
		const [audio, video] = stream.getTracks();
		expect(pc.addTrack).toHaveBeenCalledExactlyOnceWith(audio, stream);
		expect(pc.addTransceiver).toHaveBeenCalledExactlyOnceWith(video, {
			direction: 'sendonly',
			streams: [stream],
			sendEncodings: SIMULCAST_ENCODINGS
		});
		expect(SIMULCAST_ENCODINGS.map((e) => e.rid)).toEqual(['h', 'm', 'l']);
	});

	it('is always the offerer: sends sdp_offer with target_peer null on negotiation', async () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client } = createFakeSignaling();

		createPublisherConnection([], fakeStream(['video']), client);
		const pc = FakeRTCPeerConnection.instances[0];

		await pc.onnegotiationneeded?.();

		expect(pc.createOffer).toHaveBeenCalled();
		expect(client.send).toHaveBeenCalledWith({
			type: 'sdp_offer',
			target_peer: null,
			sdp: 'offer-sdp'
		});
	});

	it('sends local ICE candidates with target_peer null', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client } = createFakeSignaling();

		createPublisherConnection([], fakeStream(['video']), client);
		const pc = FakeRTCPeerConnection.instances[0];
		const candidate = { toJSON: () => ({ candidate: 'candidate:1', sdpMid: '0' }) };

		pc.onicecandidate?.({ candidate });

		expect(client.send).toHaveBeenCalledWith({
			type: 'ice_candidate',
			target_peer: null,
			candidate: JSON.stringify({ candidate: 'candidate:1', sdpMid: '0' })
		});
	});

	it('applies an sdp_answer from the SFU (from_peer null) as the remote description', async () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client, emit } = createFakeSignaling();

		createPublisherConnection([], fakeStream(['video']), client);
		const pc = FakeRTCPeerConnection.instances[0];

		await emit({ type: 'sdp_answer', from_peer: null, sdp: 'answer-sdp' });

		expect(pc.setRemoteDescription).toHaveBeenCalledWith({ type: 'answer', sdp: 'answer-sdp' });
	});

	it('ignores sdp_answer/ice_candidate meant for a different pairing (from_peer set)', async () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client, emit } = createFakeSignaling();

		createPublisherConnection([], fakeStream(['video']), client);
		const pc = FakeRTCPeerConnection.instances[0];

		await emit({ type: 'sdp_answer', from_peer: 'pub-someone-else', sdp: 'irrelevant' });

		expect(pc.setRemoteDescription).not.toHaveBeenCalled();
	});

	it('close() unsubscribes from signaling and closes the peer connection', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client, offMocks } = createFakeSignaling();

		const handle = createPublisherConnection([], fakeStream(['video']), client);
		handle.close();

		offMocks.forEach((off) => expect(off).toHaveBeenCalled());
		expect(FakeRTCPeerConnection.instances[0].close).toHaveBeenCalled();
	});
});

describe('createSubscriberTrackConnection', () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		FakeRTCPeerConnection.instances = [];
	});

	it('immediately asks for the publisher track it was created for', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client } = createFakeSignaling();

		createSubscriberTrackConnection([], 'pub-abc123', client, () => {});

		expect(client.send).toHaveBeenCalledWith({
			type: 'request_track',
			target_peer: 'pub-abc123'
		} satisfies ClientMessage);
	});

	it('answers the sdp_offer for its own publisher and ignores others', async () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client, emit } = createFakeSignaling();

		createSubscriberTrackConnection([], 'pub-abc123', client, () => {});
		const pc = FakeRTCPeerConnection.instances[0];

		await emit({ type: 'sdp_offer', from_peer: 'pub-def456', sdp: 'not-for-me' });
		expect(pc.setRemoteDescription).not.toHaveBeenCalled();

		await emit({ type: 'sdp_offer', from_peer: 'pub-abc123', sdp: 'offer-from-sfu' });

		expect(pc.setRemoteDescription).toHaveBeenCalledWith({ type: 'offer', sdp: 'offer-from-sfu' });
		expect(pc.createAnswer).toHaveBeenCalled();
		expect(client.send).toHaveBeenCalledWith({
			type: 'sdp_answer',
			target_peer: 'pub-abc123',
			sdp: 'answer-sdp'
		});
	});

	it('exposes the incoming remote stream via onTrack', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client } = createFakeSignaling();
		const onTrack = vi.fn();

		createSubscriberTrackConnection([], 'pub-abc123', client, onTrack);
		const pc = FakeRTCPeerConnection.instances[0];
		const stream = {} as MediaStream;

		pc.ontrack?.({ streams: [stream] });

		expect(onTrack).toHaveBeenCalledWith(stream);
	});

	it('reports connection_quality for its own publisher and ignores others', async () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client, emit } = createFakeSignaling();
		const onQuality = vi.fn();

		createSubscriberTrackConnection([], 'pub-abc123', client, () => {}, onQuality);

		await emit({
			type: 'connection_quality',
			from_peer: 'pub-def456',
			quality: 'bad',
			layer: 'low'
		});
		expect(onQuality).not.toHaveBeenCalled();

		await emit({
			type: 'connection_quality',
			from_peer: 'pub-abc123',
			quality: 'poor',
			layer: 'mid'
		});
		expect(onQuality).toHaveBeenCalledExactlyOnceWith({ quality: 'poor', layer: 'mid' });
	});

	it('sends connection_stats every interval until closed', async () => {
		vi.useFakeTimers();
		try {
			vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
			const { client } = createFakeSignaling();
			const handle = createSubscriberTrackConnection([], 'pub-abc123', client, () => {});
			const pc = FakeRTCPeerConnection.instances[0];
			let received = 0;
			pc.getStats.mockImplementation(async () => {
				received += 100;
				return new Map([
					[
						'in',
						{
							id: 'in',
							type: 'inbound-rtp',
							kind: 'video',
							packetsReceived: received,
							packetsLost: 0
						}
					]
				]) as unknown as RTCStatsReport;
			});

			await vi.advanceTimersByTimeAsync(STATS_INTERVAL_MS);
			expect(client.send).toHaveBeenCalledWith({
				type: 'connection_stats',
				target_peer: 'pub-abc123',
				packet_loss_pct: 0
			} satisfies ClientMessage);

			handle.close();
			const sent = vi.mocked(client.send).mock.calls.length;
			await vi.advanceTimersByTimeAsync(STATS_INTERVAL_MS * 3);
			expect(client.send).toHaveBeenCalledTimes(sent);
		} finally {
			vi.useRealTimers();
		}
	});

	it('sends the picked layer as set_preferred_layer for its publisher', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client } = createFakeSignaling();

		const handle = createSubscriberTrackConnection([], 'pub-abc123', client, () => {});
		handle.setPreferredLayer('high');

		expect(client.send).toHaveBeenCalledWith({
			type: 'set_preferred_layer',
			target_peer: 'pub-abc123',
			layer: 'high'
		} satisfies ClientMessage);
	});

	it('scopes outgoing ICE candidates to its target publisher', () => {
		vi.stubGlobal('RTCPeerConnection', FakeRTCPeerConnection);
		const { client } = createFakeSignaling();

		createSubscriberTrackConnection([], 'pub-abc123', client, () => {});
		const pc = FakeRTCPeerConnection.instances[0];
		const candidate = { toJSON: () => ({ candidate: 'candidate:1' }) };

		pc.onicecandidate?.({ candidate });

		expect(client.send).toHaveBeenCalledWith({
			type: 'ice_candidate',
			target_peer: 'pub-abc123',
			candidate: JSON.stringify({ candidate: 'candidate:1' })
		});
	});
});
