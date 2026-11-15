import { PUBLIC_API_BASE_URL } from '$env/static/public';
import type {
	ApiError,
	CreatedRoom,
	Placement,
	RoomDetails,
	RoomPublishers,
	Stage,
	StageTemplate,
	Streamer
} from './contract';

/** A non-2xx response, carrying the contract's error code (e.g. room_not_found). */
export class ApiRequestError extends Error {
	constructor(
		message: string,
		readonly status: number,
		readonly code: string | null,
		/** Set on 409 room_exists: the room already open on the server. */
		readonly roomId: string | null = null
	) {
		super(message);
		this.name = 'ApiRequestError';
	}
}

export function isRoomNotFound(err: unknown): boolean {
	return err instanceof ApiRequestError && err.code === 'room_not_found';
}

const roomPath = (roomId: string) => `/api/rooms/${encodeURIComponent(roomId)}`;

async function request<T>(
	method: string,
	path: string,
	options: { body?: unknown; adminKey?: string } = {}
): Promise<T> {
	const headers: Record<string, string> = {};
	if (options.body !== undefined) headers['Content-Type'] = 'application/json';
	if (options.adminKey) headers['X-Admin-Token'] = options.adminKey;
	const init: RequestInit = { method, headers };
	if (options.body !== undefined) init.body = JSON.stringify(options.body);

	const res = await fetch(`${PUBLIC_API_BASE_URL}${path}`, init);
	if (!res.ok) {
		const body = (await res.json().catch(() => null)) as ApiError | null;
		throw new ApiRequestError(
			body?.error?.message ?? `Request to ${path} failed with ${res.status}`,
			res.status,
			body?.error?.code ?? null,
			body?.room_id ?? null
		);
	}
	if (res.status === 204) return undefined as T;
	return (await res.json()) as T;
}

/**
 * POST /api/rooms — the caller becomes admin. 403 invalid_passcode for a wrong
 * passcode; 409 room_exists if one is open.
 */
export function createRoom(passcode: string): Promise<CreatedRoom> {
	return request<CreatedRoom>('POST', '/api/rooms', { body: { passcode } });
}

/** GET /api/rooms/:room_id — 404 room_not_found for a room that was never created. */
export function getRoomDetails(roomId: string): Promise<RoomDetails> {
	return request<RoomDetails>('GET', roomPath(roomId));
}

/** GET /api/rooms/:room_id/publishers — cheap polling without re-fetching ICE config. */
export function getRoomPublishers(roomId: string): Promise<RoomPublishers> {
	return request<RoomPublishers>('GET', `${roomPath(roomId)}/publishers`);
}

/** DELETE /api/rooms/:room_id — admin: ends the room for everyone. */
export function closeRoom(roomId: string, adminKey: string): Promise<void> {
	return request<void>('DELETE', roomPath(roomId), { adminKey });
}

/** POST /api/rooms/:room_id/admin/reclaim — works from the IP that created the room. */
export function reclaimAdmin(roomId: string): Promise<CreatedRoom> {
	return request<CreatedRoom>('POST', `${roomPath(roomId)}/admin/reclaim`);
}

/** PUT /api/rooms/:room_id/streamers/:streamer_id — after every SFU `joined`. */
export function registerStreamer(
	roomId: string,
	streamerId: string,
	body: { peer_id: string; display_name: string }
): Promise<Streamer> {
	return request<Streamer>(
		'PUT',
		`${roomPath(roomId)}/streamers/${encodeURIComponent(streamerId)}`,
		{ body }
	);
}

/** GET /api/rooms/:room_id/stage */
export function getStage(roomId: string): Promise<Stage> {
	return request<Stage>('GET', `${roomPath(roomId)}/stage`);
}

/** PUT /api/rooms/:room_id/stage — admin: replaces the template and every placement. */
export function setStage(
	roomId: string,
	adminKey: string,
	body: { template: StageTemplate | null; placements: Placement[] }
): Promise<Stage> {
	return request<Stage>('PUT', `${roomPath(roomId)}/stage`, { body, adminKey });
}

export interface RoomEventHandlers {
	onStage(stage: Stage): void;
	onRoomClosed(): void;
}

/**
 * GET /api/rooms/:room_id/events as an EventSource. Stages arrive in full;
 * out-of-order ones (lower version) are dropped. EventSource reconnects by
 * itself, and the first event after a reconnect is always a fresh stage.
 */
export function openRoomEvents(roomId: string, handlers: RoomEventHandlers): () => void {
	const source = new EventSource(`${PUBLIC_API_BASE_URL}${roomPath(roomId)}/events`);
	let version = -1;
	source.addEventListener('stage_updated', (event) => {
		const stage = JSON.parse((event as MessageEvent<string>).data) as Stage;
		if (stage.version < version) return;
		version = stage.version;
		handlers.onStage(stage);
	});
	source.addEventListener('room_closed', () => {
		source.close();
		handlers.onRoomClosed();
	});
	return () => source.close();
}
