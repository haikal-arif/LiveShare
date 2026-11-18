import type { ClientMessage, ServerMessage } from './contract';

type ServerMessageType = ServerMessage['type'];
type HandlerFor<T extends ServerMessageType> = (msg: Extract<ServerMessage, { type: T }>) => void;

/** Thin typed wrapper around the SFU signaling WebSocket described in API-Contract.md. */
export interface SignalingClient {
	send(msg: ClientMessage): void;
	on<T extends ServerMessageType>(type: T, handler: HandlerFor<T>): () => void;
	close(): void;
}

/**
 * The socket URL for `sfuWsUrl`. An absolute `ws(s)://` URL is used as is; a
 * path like `/ws` is resolved against `base` (the page) with the matching
 * `ws(s):` scheme, so the socket goes to whichever address the page was loaded
 * from. Older browsers' `WebSocket` rejects relative URLs, hence doing it here.
 */
export function resolveWsUrl(sfuWsUrl: string, base?: string): string {
	const url = new URL(sfuWsUrl, base);
	if (url.protocol === 'https:') url.protocol = 'wss:';
	if (url.protocol === 'http:') url.protocol = 'ws:';
	return url.toString();
}

export function connectSignaling(sfuWsUrl: string): Promise<SignalingClient> {
	return new Promise((resolve, reject) => {
		const socket = new WebSocket(resolveWsUrl(sfuWsUrl, globalThis.location?.href));
		const handlers = new Map<ServerMessageType, Set<(msg: ServerMessage) => void>>();

		socket.addEventListener(
			'open',
			() => {
				const client: SignalingClient = {
					send(msg) {
						socket.send(JSON.stringify(msg));
					},
					on(type, handler) {
						const set = handlers.get(type) ?? new Set();
						set.add(handler as (msg: ServerMessage) => void);
						handlers.set(type, set);
						return () => set.delete(handler as (msg: ServerMessage) => void);
					},
					close() {
						socket.close();
					}
				};
				resolve(client);
			},
			{ once: true }
		);

		socket.addEventListener('error', () => reject(new Error(`Failed to reach ${sfuWsUrl}`)), {
			once: true
		});

		socket.addEventListener('message', (event) => {
			let msg: ServerMessage;
			try {
				msg = JSON.parse(event.data as string) as ServerMessage;
			} catch {
				return;
			}
			const set = handlers.get(msg.type);
			set?.forEach((handler) => handler(msg));
		});
	});
}
