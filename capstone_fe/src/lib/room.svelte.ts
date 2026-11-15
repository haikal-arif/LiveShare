import { SvelteMap } from 'svelte/reactivity';
import { findAdminKey, streamerId } from './admin';
import { getRoomDetails, isRoomNotFound, openRoomEvents, registerStreamer } from './api';
import type { PeerId, SimulcastLayer, Stage, Streamer } from './contract';
import { facingModeOf, type FacingMode } from './device';
import { INITIAL_STREAM_QUALITY, type StreamQuality } from './quality';
import { connectSignaling, type SignalingClient } from './signaling';
import {
	createPublisherConnection,
	createSubscriberTrackConnection,
	type PeerHandle,
	type SubscriberHandle
} from './webrtc';

export type Role = 'publisher' | 'subscriber';
/** `not_found`: the room was never created; `closed`: the admin ended it. */
export type ConnectionStatus =
	'idle' | 'connecting' | 'connected' | 'error' | 'not_found' | 'closed';

/**
 * Single shared "current room session" store (Svelte 5 rune-based module
 * singleton) so the camera preview started on /room/[id]/join survives the
 * navigation into /room/[id] without re-requesting getUserMedia, and so the
 * room page and its bottom-bar controls can react to the same live state.
 */
class RoomSession {
	roomId = $state<string | null>(null);
	role = $state<Role | null>(null);
	displayName = $state('');
	localStream = $state<MediaStream | null>(null);
	peerId = $state<PeerId | null>(null);
	status = $state<ConnectionStatus>('idle');
	errorMessage = $state<string | null>(null);

	/** Other publishers' peer ids currently known in the room (self excluded). */
	remotePublisherIds = $state<PeerId[]>([]);
	/** Live remote video streams, keyed by publisher peer id, as they negotiate in. */
	remoteStreams = new SvelteMap<PeerId, MediaStream>();
	/** Per watched publisher: the layer the user picked and what the SFU actually forwards. */
	remoteQuality = new SvelteMap<PeerId, StreamQuality>();

	/** The room's stage layout and streamer names, kept live over the events stream. */
	stage = $state<Stage | null>(null);
	/** This device holds the room's admin key. */
	isAdmin = $state(false);

	micEnabled = $state(true);
	cameraEnabled = $state(true);
	screenSharing = $state(false);
	/** Which camera the outgoing video comes from. */
	facingMode = $state<FacingMode>('user');
	flippingCamera = $state(false);
	/**
	 * Mirrors the publisher's own preview only. The published track is never
	 * flipped, so viewers see the camera's true orientation.
	 */
	mirrorSelf = $state(false);

	private cameraVideoTrack: MediaStreamTrack | null = null;
	/** The captured screen while sharing; stopped when sharing ends. */
	private screenStream: MediaStream | null = null;

	/**
	 * What the publisher sees of their own video: the screen while sharing,
	 * otherwise the camera. Viewers get whatever is on the sender, so this
	 * only changes the publisher's own preview.
	 */
	get previewStream(): MediaStream | null {
		return this.screenSharing && this.screenStream ? this.screenStream : this.localStream;
	}

	private signaling: SignalingClient | null = null;
	private publisherHandle: PeerHandle | null = null;
	private subscriberHandles = new Map<PeerId, SubscriberHandle>();
	private stopEvents: (() => void) | null = null;

	/** Called on the join-streamer screen once the user grants camera access. */
	setLocalPreview(stream: MediaStream, displayName: string) {
		this.localStream = stream;
		this.displayName = displayName;
		const track = stream.getVideoTracks()[0];
		if (track) this.facingMode = facingModeOf(track, 'user');
	}

	async startPublishing(roomId: string) {
		this.status = 'connecting';
		this.errorMessage = null;
		try {
			if (!this.localStream) {
				throw new Error('Camera preview must be started before publishing');
			}
			const details = await getRoomDetails(roomId);
			const signaling = await connectSignaling(details.sfu_ws_url);
			this.signaling = signaling;
			this.roomId = roomId;
			this.role = 'publisher';
			this.listen(roomId, signaling);
			this.remotePublisherIds = details.publishers
				.map((p) => p.peer_id)
				.filter((id) => id !== this.peerId);

			signaling.on('error', (msg) => {
				this.errorMessage = msg.reason;
				this.status = 'error';
			});
			signaling.on('publisher_joined', (msg) => {
				if (msg.peer_id !== this.peerId && !this.remotePublisherIds.includes(msg.peer_id)) {
					this.remotePublisherIds = [...this.remotePublisherIds, msg.peer_id];
				}
			});
			signaling.on('publisher_left', (msg) => {
				this.remotePublisherIds = this.remotePublisherIds.filter((id) => id !== msg.peer_id);
				this.remoteStreams.delete(msg.peer_id);
			});

			await new Promise<void>((resolve) => {
				const off = signaling.on('joined', (msg) => {
					this.peerId = msg.peer_id;
					off();
					resolve();
				});
				signaling.send({ type: 'join_as_publisher', room: roomId });
			});
			// Names and stage spots hang off this, so it must land before
			// anyone can place or see this streamer.
			await registerStreamer(roomId, streamerId(), {
				peer_id: this.peerId!,
				display_name: this.displayName
			});

			this.publisherHandle = createPublisherConnection(
				details.ice_servers,
				this.localStream,
				signaling
			);
			this.status = 'connected';
		} catch (err) {
			this.fail(err, 'Failed to start streaming');
			throw err;
		}
	}

	async startWatching(roomId: string) {
		this.status = 'connecting';
		this.errorMessage = null;
		this.roomId = roomId;
		this.role = 'subscriber';
		try {
			const details = await getRoomDetails(roomId);
			const signaling = await connectSignaling(details.sfu_ws_url);
			this.signaling = signaling;
			this.listen(roomId, signaling);

			signaling.on('error', (msg) => {
				this.errorMessage = msg.reason;
				this.status = 'error';
			});
			signaling.on('publisher_joined', (msg) => {
				if (!this.remotePublisherIds.includes(msg.peer_id)) {
					this.remotePublisherIds = [...this.remotePublisherIds, msg.peer_id];
					this.watchPublisher(details.ice_servers, msg.peer_id, signaling);
				}
			});
			signaling.on('publisher_left', (msg) => {
				this.remotePublisherIds = this.remotePublisherIds.filter((id) => id !== msg.peer_id);
				this.subscriberHandles.get(msg.peer_id)?.close();
				this.subscriberHandles.delete(msg.peer_id);
				this.remoteStreams.delete(msg.peer_id);
				this.remoteQuality.delete(msg.peer_id);
			});

			await new Promise<void>((resolve) => {
				const off = signaling.on('joined', (msg) => {
					this.peerId = msg.peer_id;
					this.remotePublisherIds = msg.publishers;
					off();
					resolve();
				});
				signaling.send({ type: 'join_as_subscriber', room: roomId });
			});

			for (const publisherId of this.remotePublisherIds) {
				this.watchPublisher(details.ice_servers, publisherId, signaling);
			}
			this.status = 'connected';
		} catch (err) {
			this.fail(err, 'Failed to join room');
			throw err;
		}
	}

	private watchPublisher(
		iceServers: Parameters<typeof createSubscriberTrackConnection>[0],
		publisherId: PeerId,
		signaling: SignalingClient
	) {
		const handle = createSubscriberTrackConnection(
			iceServers,
			publisherId,
			signaling,
			(stream) => this.remoteStreams.set(publisherId, stream),
			(update) => {
				const picked = this.remoteQuality.get(publisherId)?.picked ?? 'high';
				this.remoteQuality.set(publisherId, { picked, ...update });
			}
		);
		this.subscriberHandles.set(publisherId, handle);
		this.remoteQuality.set(publisherId, INITIAL_STREAM_QUALITY);
	}

	/**
	 * The user picked a quality for one stream. Shown straight away; the
	 * SFU's reply corrects it if that layer can't be forwarded.
	 */
	pickLayer(publisherId: PeerId, layer: SimulcastLayer) {
		const handle = this.subscriberHandles.get(publisherId);
		if (!handle) return;
		this.remoteQuality.set(publisherId, { picked: layer, layer, quality: 'good' });
		handle.setPreferredLayer(layer);
	}

	/** Stage updates and the end of the room, from Go's events stream and the SFU. */
	private listen(roomId: string, signaling: SignalingClient) {
		this.stopEvents?.();
		this.stopEvents = openRoomEvents(roomId, {
			onStage: (stage) => (this.stage = stage),
			onRoomClosed: () => this.roomClosed()
		});
		signaling.on('room_closed', () => this.roomClosed());
		void findAdminKey(roomId).then((key) => {
			if (this.roomId === roomId) this.isAdmin = key !== null;
		});
	}

	private fail(err: unknown, fallback: string) {
		if (isRoomNotFound(err)) {
			this.status = 'not_found';
			this.errorMessage = "This room doesn't exist. Check the code, or ask for a new link.";
			return;
		}
		this.status = 'error';
		this.errorMessage = err instanceof Error ? err.message : fallback;
	}

	/** The admin ended the room: drop every connection but keep the notice. */
	private roomClosed() {
		if (this.status === 'closed') return;
		const roomId = this.roomId;
		this.leave();
		this.roomId = roomId;
		this.status = 'closed';
		this.errorMessage = 'The admin ended this room.';
	}

	/** The registered streamer behind an SFU peer id, for names and stage spots. */
	streamerFor(peerId: PeerId): Streamer | undefined {
		return this.stage?.streamers.find((s) => s.peer_id === peerId);
	}

	/** True once startPublishing/startWatching has actually established this room+role. */
	isActiveFor(roomId: string, role: Role): boolean {
		return this.status === 'connected' && this.roomId === roomId && this.role === role;
	}

	toggleMic() {
		const track = this.localStream?.getAudioTracks()[0];
		if (!track) return;
		track.enabled = !track.enabled;
		this.micEnabled = track.enabled;
	}

	toggleCamera() {
		const track = this.localStream?.getVideoTracks()[0];
		if (!track) return;
		track.enabled = !track.enabled;
		this.cameraEnabled = track.enabled;
	}

	/** Flips the local preview left-right. Display only; the sent video is unchanged. */
	toggleMirror() {
		this.mirrorSelf = !this.mirrorSelf;
	}

	/**
	 * Switches the outgoing video between the front and back camera. The
	 * published track is replaced in place, so viewers keep the same stream.
	 */
	async flipCamera() {
		if (!this.localStream || this.screenSharing || this.flippingCamera) return;
		this.flippingCamera = true;
		try {
			const next: FacingMode = this.facingMode === 'user' ? 'environment' : 'user';
			const stream = await navigator.mediaDevices.getUserMedia({
				video: { facingMode: { ideal: next }, width: { ideal: 1280 }, height: { ideal: 720 } }
			});
			const [newTrack] = stream.getVideoTracks();
			newTrack.enabled = this.cameraEnabled;

			const sender = this.publisherHandle?.pc.getSenders().find((s) => s.track?.kind === 'video');
			try {
				await sender?.replaceTrack(newTrack);
			} catch (err) {
				newTrack.stop();
				throw err;
			}
			// The room may have been left while the camera was opening.
			if (!this.localStream) {
				newTrack.stop();
				return;
			}
			const oldTrack = this.localStream.getVideoTracks()[0];
			this.localStream = new MediaStream([newTrack, ...this.localStream.getAudioTracks()]);
			oldTrack?.stop();
			this.facingMode = facingModeOf(newTrack, next);
		} finally {
			this.flippingCamera = false;
		}
	}

	/** Swaps the outgoing video track between the camera and a captured screen/window. */
	async toggleScreenShare() {
		const sender = this.publisherHandle?.pc.getSenders().find((s) => s.track?.kind === 'video');
		if (!sender || !this.localStream) return;

		if (this.screenSharing) {
			const camTrack = this.cameraVideoTrack ?? this.localStream.getVideoTracks()[0];
			await sender.replaceTrack(camTrack);
			this.screenSharing = false;
			this.stopScreen();
			return;
		}

		const displayStream = await navigator.mediaDevices.getDisplayMedia({ video: true });
		const screenTrack = displayStream.getVideoTracks()[0];
		this.cameraVideoTrack = this.localStream.getVideoTracks()[0];
		await sender.replaceTrack(screenTrack);
		this.screenStream = displayStream;
		this.screenSharing = true;
		screenTrack.addEventListener('ended', () => {
			if (this.screenSharing) void this.toggleScreenShare();
		});
	}

	private stopScreen() {
		this.screenStream?.getTracks().forEach((t) => t.stop());
		this.screenStream = null;
	}

	leave() {
		this.stopScreen();
		this.screenSharing = false;
		this.stopEvents?.();
		this.stopEvents = null;
		this.stage = null;
		this.isAdmin = false;
		this.signaling?.send({ type: 'leave' });
		this.signaling?.close();
		this.signaling = null;
		this.publisherHandle?.close();
		this.publisherHandle = null;
		this.subscriberHandles.forEach((h) => h.close());
		this.subscriberHandles.clear();
		this.remoteStreams.clear();
		this.remoteQuality.clear();
		this.remotePublisherIds = [];
		this.localStream?.getTracks().forEach((t) => t.stop());
		this.localStream = null;
		this.roomId = null;
		this.role = null;
		this.peerId = null;
		this.status = 'idle';
		this.errorMessage = null;
	}
}

export const roomSession = new RoomSession();
