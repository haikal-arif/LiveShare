import QRCode from 'qrcode';

/** Renders a QR code for `text` onto `canvas`, fully offline. */
export function renderQrToCanvas(canvas: HTMLCanvasElement, text: string): Promise<void> {
	return QRCode.toCanvas(canvas, text, {
		width: 220,
		margin: 1,
		color: { dark: '#111827', light: '#ffffff' }
	});
}

/**
 * Builds the URL a join-QR should encode: opens straight to /home pre-filled
 * with the room. Takes `origin` explicitly (e.g. from `page.url.origin`)
 * rather than reading `window.location` so it stays safe to call during SSR.
 */
export function buildRoomJoinUrl(origin: string, roomId: string): string {
	return `${origin}/home?room=${encodeURIComponent(roomId)}`;
}

/**
 * Recovers a room id from a scanned QR payload. Handles our own
 * buildRoomJoinUrl() output (?room=... query param) and falls back to
 * treating the raw decoded text as the room id.
 */
export function extractRoomIdFromScan(decodedText: string): string {
	try {
		const url = new URL(decodedText);
		const room = url.searchParams.get('room');
		if (room) return room;
	} catch {
		// not a URL — fall through and use the raw text
	}
	return decodedText;
}
