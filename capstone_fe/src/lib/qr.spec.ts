import { describe, expect, it } from 'vitest';
import { buildRoomJoinUrl, extractRoomIdFromScan } from './qr';

describe('buildRoomJoinUrl', () => {
	it('builds a /home URL carrying the room as a query param', () => {
		expect(buildRoomJoinUrl('http://192.168.1.50:5183', 'ABC-123')).toBe(
			'http://192.168.1.50:5183/home?room=ABC-123'
		);
	});

	it('URL-encodes special characters in the room id', () => {
		expect(buildRoomJoinUrl('http://localhost:5183', 'room with spaces')).toBe(
			'http://localhost:5183/home?room=room%20with%20spaces'
		);
	});
});

describe('extractRoomIdFromScan', () => {
	it('recovers the room id from a buildRoomJoinUrl-style URL', () => {
		const url = buildRoomJoinUrl('http://localhost:5183', 'ABC-123');
		expect(extractRoomIdFromScan(url)).toBe('ABC-123');
	});

	it('falls back to the raw text when it is not a URL', () => {
		expect(extractRoomIdFromScan('ABC-123')).toBe('ABC-123');
	});

	it('falls back to the raw text when the URL has no room param', () => {
		expect(extractRoomIdFromScan('http://example.com/somewhere')).toBe(
			'http://example.com/somewhere'
		);
	});
});
