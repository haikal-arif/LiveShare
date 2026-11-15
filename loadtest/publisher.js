// Publisher emulation for FlashLive. Each VU is a streamer: it checks the room
// over REST, opens the SFU's /ws signaling socket, completes the publisher
// WebRTC handshake (capstone_sfu/API-Contract.md, section 3), and streams a
// short looping video as its camera.
//
// k6 has no WebRTC stack of its own, so the publisher runs inside headless
// Chromium through the xk6-browser extension. The page does the real work:
// getUserMedia (fed a y4m file by Chromium's fake camera), RTCPeerConnection,
// DTLS/SRTP and the signaling messages. The page code follows the frontend's
// publisher path (capstone_fe/src/lib/webrtc.ts).
//
// Prerequisites:
//   1. A k6 binary with xk6-browser, e.g.
//        xk6 build v2.3.0 --with github.com/grafana/xk6-browser@latest
//      The stock k6 binary does not include k6/x/browser.
//   2. Chromium, given by K6_BROWSER_EXECUTABLE_PATH.
//   3. The test video: ./make-test-video.sh
//   4. A room that is already open (created by an admin through the app).
//
// Run (the fake camera flags come from K6_BROWSER_ARGS, read at k6 startup):
//   K6_BROWSER_EXECUTABLE_PATH=/path/to/chromium \
//   K6_BROWSER_ARGS="use-fake-ui-for-media-stream,use-fake-device-for-media-stream,use-file-for-fake-video-capture=/abs/path/publisher.y4m,ignore-certificate-errors" \
//   ROOM_ID=ABC-234 PUBLISHERS=3 HOLD_SECONDS=60 \
//   k6 run loadtest/publisher.js
//
// Env:
//   ROOM_ID        required, the room to publish into
//   BASE_URL       gateway origin (default https://localhost:8443)
//   PUBLISHERS     concurrent publishers, one per VU (default 1)
//   HOLD_SECONDS   how long each publisher stays live (default 30)

import { browser } from 'k6/x/browser/async';
import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8443';
const ROOM_ID = __ENV.ROOM_ID;
const PUBLISHERS = Number(__ENV.PUBLISHERS || 1);
const HOLD_SECONDS = Number(__ENV.HOLD_SECONDS || 30);

if (!ROOM_ID) {
	throw new Error('set ROOM_ID to an open room');
}

export const options = {
	insecureSkipTLSVerify: true,
	scenarios: {
		publishers: {
			executor: 'per-vu-iterations',
			vus: PUBLISHERS,
			iterations: 1,
			maxDuration: '10m',
			options: {
				browser: {
					type: 'chromium'
				}
			}
		}
	},
	thresholds: {
		checks: ['rate==1.0']
	}
};

export function setup() {
	const res = http.get(`${BASE_URL}/api/rooms/${ROOM_ID}`);
	if (res.status !== 200) {
		throw new Error(`room ${ROOM_ID} is not joinable: HTTP ${res.status} ${res.body}`);
	}
}

export default async function () {
	const context = await browser.newContext({
		ignoreHTTPSErrors: true,
		permissions: ['camera', 'microphone']
	});
	const page = await context.newPage();

	try {
		// Any static file from the same origin. getUserMedia needs a secure
		// context, and this keeps the page free of app JavaScript.
		await page.goto(`${BASE_URL}/robots.txt`);

		const joined = await page.evaluate(startPublisher, {
			wsUrl: `${BASE_URL.replace(/^http/, 'ws')}/ws`,
			room: ROOM_ID,
			connectTimeoutMs: 20000
		});
		check(joined, {
			'SFU assigned a peer id': (r) => r.peerId !== '',
			'publisher connected': (r) => r.connectionState === 'connected'
		});

		sleep(HOLD_SECONDS);

		const stats = await page.evaluate(stopPublisher);
		check(stats, {
			'video kept streaming': (s) => s.framesSent > s.framesAtConnect,
			'no SFU errors': (s) => s.serverErrors.length === 0
		});
	} finally {
		await page.close();
		await context.close();
	}
}

// Runs inside the browser page. xk6-browser sends only this function's source
// text to Chromium, so every helper it needs lives inside the body.
async function startPublisher(cfg) {
	const SIMULCAST = [
		{ rid: 'h', maxBitrate: 2500000 },
		{ rid: 'm', scaleResolutionDownBy: 2, maxBitrate: 800000 },
		{ rid: 'l', scaleResolutionDownBy: 4, maxBitrate: 250000 }
	];

	const stream = await navigator.mediaDevices.getUserMedia({
		video: { width: 320, height: 240, frameRate: 15 },
		audio: true
	});
	const ws = new WebSocket(cfg.wsUrl);
	const state = { ws, stream, pc: null, peerId: '', serverErrors: [] };
	window.__flashlivePublisher = state;

	state.framesSent = async () => {
		let total = 0;
		const report = await state.pc.getStats();
		report.forEach((s) => {
			// Simulcast sends one outbound-rtp per layer, so sum them.
			if (s.type === 'outbound-rtp' && s.kind === 'video') {
				total += s.framesSent || 0;
			}
		});
		return total;
	};

	const send = (msg) => ws.send(JSON.stringify(msg));

	let resolveConnected;
	let rejectConnected;
	const connected = new Promise((resolve, reject) => {
		resolveConnected = resolve;
		rejectConnected = reject;
	});

	ws.onopen = () => send({ type: 'join_as_publisher', room: cfg.room });
	ws.onerror = () => rejectConnected(new Error(`websocket error on ${cfg.wsUrl}`));
	ws.onclose = () => rejectConnected(new Error('websocket closed before media connected'));

	ws.onmessage = async (event) => {
		const msg = JSON.parse(event.data);
		try {
			switch (msg.type) {
				case 'joined': {
					// Publisher is the impolite offerer. The SFU's peer is target_peer: null.
					state.peerId = msg.peer_id;
					const pc = new RTCPeerConnection({
						iceServers: msg.ice_servers.map((s) => ({ urls: s.urls }))
					});
					state.pc = pc;

					pc.onnegotiationneeded = async () => {
						await pc.setLocalDescription(await pc.createOffer());
						send({ type: 'sdp_offer', target_peer: null, sdp: pc.localDescription.sdp });
					};
					pc.onicecandidate = (event) => {
						if (event.candidate) {
							send({
								type: 'ice_candidate',
								target_peer: null,
								candidate: JSON.stringify(event.candidate.toJSON())
							});
						}
					};
					pc.onconnectionstatechange = () => {
						if (pc.connectionState === 'connected') resolveConnected();
						if (pc.connectionState === 'failed') {
							rejectConnected(new Error('peer connection failed'));
						}
					};

					for (const track of stream.getTracks()) {
						if (track.kind === 'video') {
							pc.addTransceiver(track, {
								direction: 'sendonly',
								streams: [stream],
								sendEncodings: SIMULCAST.map((e) => ({ ...e }))
							});
						} else {
							pc.addTrack(track, stream);
						}
					}
					break;
				}
				case 'sdp_answer':
					if (msg.from_peer === null) {
						await state.pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp });
					}
					break;
				case 'ice_candidate':
					if (msg.from_peer === null) {
						await state.pc.addIceCandidate(JSON.parse(msg.candidate));
					}
					break;
				case 'error':
					state.serverErrors.push(`${msg.code}: ${msg.reason}`);
					break;
				case 'room_closed':
					rejectConnected(new Error('room closed by admin'));
					break;
			}
		} catch (err) {
			state.serverErrors.push(`client: ${err.message}`);
		}
	};

	const timer = setTimeout(
		() => rejectConnected(new Error('timed out waiting for media to connect')),
		cfg.connectTimeoutMs
	);
	try {
		await connected;
	} finally {
		clearTimeout(timer);
	}

	state.framesAtConnect = await state.framesSent();
	return {
		peerId: state.peerId,
		connectionState: state.pc.connectionState,
		framesAtConnect: state.framesAtConnect
	};
}

// Runs inside the browser page: leaves the room and returns what was sent.
async function stopPublisher() {
	const state = window.__flashlivePublisher;
	const framesSent = await state.framesSent();
	const result = {
		peerId: state.peerId,
		connectionState: state.pc.connectionState,
		framesSent,
		framesAtConnect: state.framesAtConnect,
		serverErrors: state.serverErrors
	};

	state.ws.send(JSON.stringify({ type: 'leave' }));
	state.stream.getTracks().forEach((t) => t.stop());
	state.pc.close();
	state.ws.close();
	return result;
}
