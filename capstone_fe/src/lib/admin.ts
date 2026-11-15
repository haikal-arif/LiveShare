import { reclaimAdmin } from './api';

/**
 * Admin keys, per room, in localStorage: the room creator's only proof of
 * being admin (API-Contract.md, POST /api/rooms).
 */
const storageKey = (roomId: string) => `flashlive:admin-key:${roomId}`;

export function getAdminKey(roomId: string): string | null {
	return localStorage.getItem(storageKey(roomId));
}

export function saveAdminKey(roomId: string, adminKey: string) {
	localStorage.setItem(storageKey(roomId), adminKey);
}

export function forgetAdminKey(roomId: string) {
	localStorage.removeItem(storageKey(roomId));
}

/**
 * The stored key, or one reclaimed from the server when this device's IP
 * created the room. Null when this device isn't the admin.
 */
export async function findAdminKey(roomId: string): Promise<string | null> {
	const stored = getAdminKey(roomId);
	if (stored) return stored;
	try {
		const { admin_token } = await reclaimAdmin(roomId);
		saveAdminKey(roomId, admin_token);
		return admin_token;
	} catch {
		return null;
	}
}

const STREAMER_ID_KEY = 'flashlive:streamer-id';

/**
 * This browser's stable streamer id, so a streamer who refreshes keeps
 * their spot on the stage (the SFU peer id changes on every reconnect).
 */
export function streamerId(): string {
	let id = localStorage.getItem(STREAMER_ID_KEY);
	if (!id) {
		id = crypto.randomUUID();
		localStorage.setItem(STREAMER_ID_KEY, id);
	}
	return id;
}
