import { describe, expect, it } from 'vitest';
import { facingModeOf } from './device';

function trackReporting(facingMode?: string): MediaStreamTrack {
	return { getSettings: () => ({ facingMode }) } as MediaStreamTrack;
}

describe('facingModeOf', () => {
	it('reads the front camera from the track', () => {
		expect(facingModeOf(trackReporting('user'), 'environment')).toBe('user');
	});

	it('reads the back camera from the track', () => {
		expect(facingModeOf(trackReporting('environment'), 'user')).toBe('environment');
	});

	it('falls back when the browser reports no facing mode', () => {
		expect(facingModeOf(trackReporting(undefined), 'user')).toBe('user');
	});

	it('falls back on an unrecognised facing mode', () => {
		expect(facingModeOf(trackReporting('left'), 'environment')).toBe('environment');
	});
});
