import { describe, expect, it } from 'vitest';
import { generateRoomCode, normalizeRoomCode } from './roomcode';

describe('generateRoomCode', () => {
	it('produces an XXX-XXX shaped code', () => {
		expect(generateRoomCode()).toMatch(/^[A-Z2-9]{3}-[A-Z2-9]{3}$/);
	});

	it('never uses visually ambiguous characters', () => {
		for (let i = 0; i < 200; i++) {
			expect(generateRoomCode()).not.toMatch(/[0O1I]/);
		}
	});

	it('is not hardcoded to a single value', () => {
		const codes = new Set(Array.from({ length: 50 }, () => generateRoomCode()));
		expect(codes.size).toBeGreaterThan(1);
	});
});

describe('normalizeRoomCode', () => {
	it('trims surrounding whitespace', () => {
		expect(normalizeRoomCode('  ABC-123  ')).toBe('ABC-123');
	});

	it('uppercases lowercase input', () => {
		expect(normalizeRoomCode('abc-123')).toBe('ABC-123');
	});

	it('passes through an already-normalized code unchanged', () => {
		expect(normalizeRoomCode('ABC-123')).toBe('ABC-123');
	});
});
