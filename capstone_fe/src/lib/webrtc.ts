import { startStatsReporter } from './connection-stats';
import type { ConnectionQuality, IceServer, PeerId, SimulcastLayer } from './contract';
import type { SignalingClient } from './signaling';

/**
 * The three simulcast layers a publisher sends (API-Contract.md, "Connection
 * quality & simulcast"). The SFU can't transcode, so these are what it picks
 * from for each subscriber. The browser may drop layers when the camera
 * resolution or its own upload bandwidth is too low; the SFU copes with
 * whichever ones arrive.
 */
export const SIMULCAST_ENCODINGS: readonly RTCRtpEncodingParameters[] = [
	{ rid: 'h', maxBitrate: 2_500_000 },
	{ rid: 'm', scaleResolutionDownBy: 2, maxBitrate: 800_000 },
	{ rid: 'l', scaleResolutionDownBy: 4, maxBitrate: 250_000 }
];

function toRtcIceServers(iceServers: IceServer[]): RTCIceServer[] {
	return iceServers.map((s) => ({
		urls: s.urls,
		...(s.username !== undefined && { username: s.username }),
		...(s.credential !== undefined && { credential: s.credential })
	}));
}

/** ICE candidates travel as an opaque string on the wire; we JSON-encode the init dict. */
function encodeCandidate(candidate: RTCIceCandidate): string {
	return JSON.stringify(candidate.toJSON());
}

function decodeCandidate(candidate: string): RTCIceCandidateInit {
	return JSON.parse(candidate) as RTCIceCandidateInit;
}

export interface PeerHandle {
	pc: RTCPeerConnection;
	close(): void;
}

/**
 * Publisher <-> SFU: the publisher is always the (impolite) offerer, including
 * for renegotiation — reuses the same sdp_offer/sdp_answer messages, target_peer
 * is always null on this pairing since there's only one counterparty (the SFU).
 */
export function createPublisherConnection(
	iceServers: IceServer[],
	localStream: MediaStream,
	signaling: SignalingClient
): PeerHandle {
	const pc = new RTCPeerConnection({ iceServers: toRtcIceServers(iceServers) });

	for (const track of localStream.getTracks()) {
		if (track.kind === 'video') {
			pc.addTransceiver(track, {
				direction: 'sendonly',
				streams: [localStream],
				sendEncodings: SIMULCAST_ENCODINGS.map((encoding) => ({ ...encoding }))
			});
		} else {
			pc.addTrack(track, localStream);
		}
	}

	pc.onnegotiationneeded = async () => {
		const offer = await pc.createOffer();
		await pc.setLocalDescription(offer);
		if (pc.localDescription) {
			signaling.send({ type: 'sdp_offer', target_peer: null, sdp: pc.localDescription.sdp });
		}
	};

	pc.onicecandidate = (event) => {
		if (event.candidate) {
			signaling.send({
				type: 'ice_candidate',
				target_peer: null,
				candidate: encodeCandidate(event.candidate)
			});
		}
	};

	const offHandlers = [
		signaling.on('sdp_answer', async (msg) => {
			if (msg.from_peer !== null) return;
			await pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp });
		}),
		signaling.on('ice_candidate', async (msg) => {
			if (msg.from_peer !== null) return;
			await pc.addIceCandidate(decodeCandidate(msg.candidate));
		})
	];

	return {
		pc,
		close() {
			offHandlers.forEach((off) => off());
			pc.close();
		}
	};
}

export interface QualityUpdate {
	quality: ConnectionQuality;
	layer: SimulcastLayer;
}

export interface SubscriberHandle extends PeerHandle {
	/**
	 * Picks the layer to watch. Applied at once, even above a layer the SFU
	 * downgraded to, so this is also how the user retries a higher quality.
	 * The SFU answers with connection_quality either way.
	 */
	setPreferredLayer(layer: SimulcastLayer): void;
}

/**
 * Subscriber <-> SFU, one connection per publisher being watched: strictly
 * offerer(SFU)/answerer(subscriber). Triggered by sending request_track;
 * the SFU replies with sdp_offer{from_peer: targetPeerId}. While open it
 * reports connection_stats so the SFU can pick a simulcast layer, and
 * passes the SFU's connection_quality verdicts to `onQuality`.
 */
export function createSubscriberTrackConnection(
	iceServers: IceServer[],
	targetPeer: PeerId,
	signaling: SignalingClient,
	onTrack: (stream: MediaStream) => void,
	onQuality: (update: QualityUpdate) => void = () => {}
): SubscriberHandle {
	const pc = new RTCPeerConnection({ iceServers: toRtcIceServers(iceServers) });

	pc.ontrack = (event) => {
		onTrack(event.streams[0]);
	};

	pc.onicecandidate = (event) => {
		if (event.candidate) {
			signaling.send({
				type: 'ice_candidate',
				target_peer: targetPeer,
				candidate: encodeCandidate(event.candidate)
			});
		}
	};

	const offHandlers = [
		signaling.on('sdp_offer', async (msg) => {
			if (msg.from_peer !== targetPeer) return;
			await pc.setRemoteDescription({ type: 'offer', sdp: msg.sdp });
			const answer = await pc.createAnswer();
			await pc.setLocalDescription(answer);
			if (pc.localDescription) {
				signaling.send({
					type: 'sdp_answer',
					target_peer: targetPeer,
					sdp: pc.localDescription.sdp
				});
			}
		}),
		signaling.on('ice_candidate', async (msg) => {
			if (msg.from_peer !== targetPeer) return;
			await pc.addIceCandidate(decodeCandidate(msg.candidate));
		}),
		signaling.on('connection_quality', (msg) => {
			if (msg.from_peer !== targetPeer) return;
			onQuality({ quality: msg.quality, layer: msg.layer });
		})
	];

	signaling.send({ type: 'request_track', target_peer: targetPeer });
	const stopStats = startStatsReporter(pc, targetPeer, signaling);

	return {
		pc,
		setPreferredLayer(layer) {
			signaling.send({ type: 'set_preferred_layer', target_peer: targetPeer, layer });
		},
		close() {
			stopStats();
			offHandlers.forEach((off) => off());
			pc.close();
		}
	};
}
