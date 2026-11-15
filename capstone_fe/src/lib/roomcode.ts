const ALPHABET = 'ABCDEFGHJKLMNPQRSTUVWXYZ23456789'; // no 0/O/1/I to avoid ambiguity

/** Generates a wireframe-style room code, e.g. "ABC-123". */
export function generateRoomCode(): string {
	const random = crypto.getRandomValues(new Uint32Array(6));
	const chars = Array.from(random, (n) => ALPHABET[n % ALPHABET.length]);
	return `${chars.slice(0, 3).join('')}-${chars.slice(3).join('')}`;
}

/** Normalizes user-entered / scanned room codes for comparison and lookup. */
export function normalizeRoomCode(input: string): string {
	return input.trim().toUpperCase();
}
