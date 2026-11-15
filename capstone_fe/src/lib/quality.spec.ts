import { describe, expect, it } from 'vitest';
import { INITIAL_STREAM_QUALITY, isAutoSelected, qualityHint, qualityOptions } from './quality';

describe('quality picker labels', () => {
	it('shows plain labels while the forwarded layer is the one picked', () => {
		expect(isAutoSelected(INITIAL_STREAM_QUALITY)).toBe(false);
		expect(qualityOptions(INITIAL_STREAM_QUALITY).map((o) => o.label)).toEqual([
			'High',
			'Mid',
			'Low'
		]);
		expect(qualityHint(INITIAL_STREAM_QUALITY)).toBe('Stream quality');
	});

	it('tags the forwarded layer "(auto)" after a downgrade', () => {
		const q = { picked: 'high', layer: 'mid', quality: 'poor' } as const;
		expect(isAutoSelected(q)).toBe(true);
		expect(qualityOptions(q)).toEqual([
			{ value: 'high', label: 'High' },
			{ value: 'mid', label: 'Mid (auto)' },
			{ value: 'low', label: 'Low' }
		]);
		expect(qualityHint(q)).toContain('Lowered from High');
	});
});
