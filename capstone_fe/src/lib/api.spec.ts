import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
	ApiRequestError,
	closeRoom,
	createRoom,
	getRoomDetails,
	getRoomPublishers,
	isRoomNotFound,
	openRoomEvents,
	registerStreamer,
	setStage
} from './api';

function jsonResponse(body: unknown, ok = true, status = 200) {
	return {
		ok,
		status,
		json: async () => body
	} as Response;
}

describe('api', () => {
	beforeEach(() => {
		vi.stubGlobal('fetch', vi.fn());
	});

	afterEach(() => {
		vi.unstubAllGlobals();
	});

	it('getRoomDetails hits GET /api/rooms/:room_id and returns the parsed body', async () => {
		const body = {
			room_id: 'ABC-123',
			publishers: [{ peer_id: 'pub-1' }],
			sfu_ws_url: 'wss://sfu.example.com/ws',
			ice_servers: [{ urls: ['stun:stun.example.com:3478'] }]
		};
		vi.mocked(fetch).mockResolvedValue(jsonResponse(body));

		const result = await getRoomDetails('ABC-123');

		expect(fetch).toHaveBeenCalledWith(
			expect.stringMatching(/\/api\/rooms\/ABC-123$/),
			expect.objectContaining({ method: 'GET' })
		);
		expect(result).toEqual(body);
	});

	it('URL-encodes the room id in the path', async () => {
		vi.mocked(fetch).mockResolvedValue(
			jsonResponse({ room_id: 'a b', publishers: [], sfu_ws_url: '', ice_servers: [] })
		);

		await getRoomDetails('a b');

		expect(fetch).toHaveBeenCalledWith(
			expect.stringMatching(/\/api\/rooms\/a%20b$/),
			expect.anything()
		);
	});

	it('getRoomPublishers hits GET /api/rooms/:room_id/publishers', async () => {
		const body = { room_id: 'ABC-123', publishers: [] };
		vi.mocked(fetch).mockResolvedValue(jsonResponse(body));

		const result = await getRoomPublishers('ABC-123');

		expect(fetch).toHaveBeenCalledWith(
			expect.stringMatching(/\/api\/rooms\/ABC-123\/publishers$/),
			expect.anything()
		);
		expect(result).toEqual(body);
	});

	it('throws the server-provided message when the response is not ok', async () => {
		vi.mocked(fetch).mockResolvedValue(
			jsonResponse({ error: { code: 'not_found', message: 'room not found' } }, false, 404)
		);

		await expect(getRoomDetails('missing')).rejects.toThrow('room not found');
	});

	it('falls back to a generic message when the error body cannot be parsed', async () => {
		vi.mocked(fetch).mockResolvedValue({
			ok: false,
			status: 500,
			json: async () => {
				throw new Error('not json');
			}
		} as unknown as Response);

		await expect(getRoomDetails('ABC-123')).rejects.toThrow(/500/);
	});

	it('createRoom POSTs the passcode to /api/rooms and surfaces 409 room_exists with the open room', async () => {
		vi.mocked(fetch).mockResolvedValueOnce(
			jsonResponse({ room_id: 'ABC-123', admin_token: 'k_1' }, true, 201)
		);
		expect(await createRoom('s3cret')).toEqual({ room_id: 'ABC-123', admin_token: 'k_1' });
		expect(fetch).toHaveBeenCalledWith(
			expect.stringMatching(/\/api\/rooms$/),
			expect.objectContaining({
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ passcode: 's3cret' })
			})
		);

		vi.mocked(fetch).mockResolvedValueOnce(
			jsonResponse(
				{ error: { code: 'room_exists', message: 'a room is already open' }, room_id: 'XYZ-999' },
				false,
				409
			)
		);
		const err = await createRoom('s3cret').catch((e) => e);
		expect(err).toBeInstanceOf(ApiRequestError);
		expect(err).toMatchObject({ status: 409, code: 'room_exists', roomId: 'XYZ-999' });
	});

	it('recognises room_not_found', async () => {
		vi.mocked(fetch).mockResolvedValue(
			jsonResponse({ error: { code: 'room_not_found', message: 'nope' } }, false, 404)
		);
		expect(isRoomNotFound(await getRoomDetails('ABC-123').catch((e) => e))).toBe(true);
	});

	it('sends the admin key on admin calls and the body as JSON', async () => {
		vi.mocked(fetch).mockResolvedValue({ ok: true, status: 204 } as Response);
		await closeRoom('ABC-123', 'k_secret');
		expect(fetch).toHaveBeenCalledWith(
			expect.stringMatching(/\/api\/rooms\/ABC-123$/),
			expect.objectContaining({ method: 'DELETE', headers: { 'X-Admin-Token': 'k_secret' } })
		);

		const stage = {
			template: 'concert' as const,
			placements: [{ streamer_id: 's1', x: 0.3, y: 0.2 }]
		};
		vi.mocked(fetch).mockResolvedValue(
			jsonResponse({ room_id: 'ABC-123', version: 1, ...stage, streamers: [] })
		);
		await setStage('ABC-123', 'k_secret', stage);
		expect(fetch).toHaveBeenLastCalledWith(expect.stringMatching(/\/api\/rooms\/ABC-123\/stage$/), {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json', 'X-Admin-Token': 'k_secret' },
			body: JSON.stringify(stage)
		});
	});

	it('registerStreamer PUTs to the streamer id without an admin key', async () => {
		vi.mocked(fetch).mockResolvedValue(jsonResponse({}));
		await registerStreamer('ABC-123', 'id 1', { peer_id: 'pub-1', display_name: 'Hana' });
		expect(fetch).toHaveBeenCalledWith(
			expect.stringMatching(/\/api\/rooms\/ABC-123\/streamers\/id%201$/),
			{
				method: 'PUT',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ peer_id: 'pub-1', display_name: 'Hana' })
			}
		);
	});
});

class FakeEventSource {
	static last: FakeEventSource;
	listeners = new Map<string, (e: MessageEvent) => void>();
	close = vi.fn();
	constructor(readonly url: string) {
		FakeEventSource.last = this;
	}
	addEventListener(type: string, fn: (e: MessageEvent) => void) {
		this.listeners.set(type, fn);
	}
	emit(type: string, data: unknown) {
		this.listeners.get(type)?.({ data: JSON.stringify(data) } as MessageEvent);
	}
}

describe('openRoomEvents', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('passes stages on, drops out-of-order ones, and stops on room_closed', () => {
		vi.stubGlobal('EventSource', FakeEventSource);
		const onStage = vi.fn();
		const onRoomClosed = vi.fn();
		openRoomEvents('ABC-123', { onStage, onRoomClosed });
		const source = FakeEventSource.last;
		expect(source.url).toMatch(/\/api\/rooms\/ABC-123\/events$/);

		source.emit('stage_updated', { version: 2 });
		source.emit('stage_updated', { version: 1 });
		source.emit('stage_updated', { version: 3 });
		expect(onStage.mock.calls.map(([s]) => s.version)).toEqual([2, 3]);

		source.emit('room_closed', { room_id: 'ABC-123' });
		expect(onRoomClosed).toHaveBeenCalledOnce();
		expect(source.close).toHaveBeenCalled();
	});
});
