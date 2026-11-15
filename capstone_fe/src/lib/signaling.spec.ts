import { afterEach, describe, expect, it, vi } from 'vitest';
import { connectSignaling } from './signaling';

type Listener = (event: unknown) => void;

/** Minimal fake standing in for the browser WebSocket, driven manually by tests. */
class FakeWebSocket {
	static instances: FakeWebSocket[] = [];
	url: string;
	sent: string[] = [];
	private listeners = new Map<string, Set<Listener>>();

	constructor(url: string) {
		this.url = url;
		FakeWebSocket.instances.push(this);
	}

	addEventListener(type: string, listener: Listener) {
		(this.listeners.get(type) ?? this.listeners.set(type, new Set()).get(type)!).add(listener);
	}

	removeEventListener(type: string, listener: Listener) {
		this.listeners.get(type)?.delete(listener);
	}

	send(data: string) {
		this.sent.push(data);
	}

	close() {
		this.emit('close', {});
	}

	emit(type: string, event: unknown) {
		this.listeners.get(type)?.forEach((listener) => listener(event));
	}
}

describe('connectSignaling', () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		FakeWebSocket.instances = [];
	});

	it('resolves once the socket opens', async () => {
		vi.stubGlobal('WebSocket', FakeWebSocket);

		const pending = connectSignaling('wss://sfu.example.com/ws');
		const socket = FakeWebSocket.instances[0];
		expect(socket.url).toBe('wss://sfu.example.com/ws');
		socket.emit('open', {});

		await expect(pending).resolves.toBeTruthy();
	});

	it('rejects if the socket errors before opening', async () => {
		vi.stubGlobal('WebSocket', FakeWebSocket);

		const pending = connectSignaling('wss://sfu.example.com/ws');
		FakeWebSocket.instances[0].emit('error', {});

		await expect(pending).rejects.toThrow();
	});

	it('send() JSON-encodes the message onto the underlying socket', async () => {
		vi.stubGlobal('WebSocket', FakeWebSocket);

		const pending = connectSignaling('wss://sfu.example.com/ws');
		const socket = FakeWebSocket.instances[0];
		socket.emit('open', {});
		const client = await pending;

		client.send({ type: 'join_as_publisher', room: 'ABC-123' });

		expect(socket.sent).toEqual([JSON.stringify({ type: 'join_as_publisher', room: 'ABC-123' })]);
	});

	it('dispatches incoming messages only to handlers registered for that type', async () => {
		vi.stubGlobal('WebSocket', FakeWebSocket);

		const pending = connectSignaling('wss://sfu.example.com/ws');
		const socket = FakeWebSocket.instances[0];
		socket.emit('open', {});
		const client = await pending;

		const joined = vi.fn();
		const errored = vi.fn();
		client.on('joined', joined);
		client.on('error', errored);

		socket.emit('message', {
			data: JSON.stringify({ type: 'joined', peer_id: 'sub-1', ice_servers: [], publishers: [] })
		});

		expect(joined).toHaveBeenCalledTimes(1);
		expect(joined.mock.calls[0][0]).toMatchObject({ type: 'joined', peer_id: 'sub-1' });
		expect(errored).not.toHaveBeenCalled();
	});

	it('on() returns an unsubscribe function', async () => {
		vi.stubGlobal('WebSocket', FakeWebSocket);

		const pending = connectSignaling('wss://sfu.example.com/ws');
		const socket = FakeWebSocket.instances[0];
		socket.emit('open', {});
		const client = await pending;

		const handler = vi.fn();
		const off = client.on('publisher_joined', handler);
		off();

		socket.emit('message', {
			data: JSON.stringify({ type: 'publisher_joined', peer_id: 'pub-2' })
		});

		expect(handler).not.toHaveBeenCalled();
	});

	it('ignores malformed message payloads instead of throwing', async () => {
		vi.stubGlobal('WebSocket', FakeWebSocket);

		const pending = connectSignaling('wss://sfu.example.com/ws');
		const socket = FakeWebSocket.instances[0];
		socket.emit('open', {});
		await pending;

		expect(() => socket.emit('message', { data: 'not json' })).not.toThrow();
	});
});
