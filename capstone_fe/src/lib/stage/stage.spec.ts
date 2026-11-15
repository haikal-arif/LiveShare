import { describe, expect, it } from 'vitest';
import type { Stage } from '../contract';
import { describeSpot, placeStreamer, removePlacement, splitStage, toStagePosition } from './stage';

const stage: Stage = {
	room_id: 'ABC-123',
	version: 3,
	template: 'concert',
	placements: [
		{ streamer_id: 's1', x: 0.3, y: 0.23 },
		{ streamer_id: 'ghost', x: 0.5, y: 0.5 }
	],
	streamers: [
		{ streamer_id: 's1', peer_id: 'pub-1', display_name: 'Hana A', live: true },
		{ streamer_id: 's2', peer_id: 'pub-2', display_name: 'User D', live: false }
	]
};

describe('stage helpers', () => {
	it('turns placed streamers into pins and lists the rest as unplaced', () => {
		const { pins, unplaced } = splitStage(stage);
		expect(pins).toEqual([
			{ streamerId: 's1', peerId: 'pub-1', name: 'Hana A', live: true, x: 0.3, y: 0.23 }
		]);
		expect(unplaced.map((s) => s.streamer_id)).toEqual(['s2']);
	});

	it('maps a pointer to a clamped, rounded 0-1 position', () => {
		const rect = { left: 10, top: 20, width: 200, height: 260 };
		expect(toStagePosition(70, 80, rect)).toEqual({ x: 0.3, y: 0.23 });
		expect(toStagePosition(-50, 999, rect)).toEqual({ x: 0, y: 1 });
	});

	it('moves or adds a placement and removes one', () => {
		const moved = placeStreamer(stage.placements, 's1', 0.9, 0.9);
		expect(moved).toContainEqual({ streamer_id: 's1', x: 0.9, y: 0.9 });
		expect(moved).toHaveLength(2);
		expect(placeStreamer([], 's2', 0.1, 0.2)).toEqual([{ streamer_id: 's2', x: 0.1, y: 0.2 }]);
		expect(removePlacement(moved, 's1')).toEqual([{ streamer_id: 'ghost', x: 0.5, y: 0.5 }]);
	});

	it('describes where a pin is on the template', () => {
		expect(describeSpot('concert', 0.45, 0.75)).toBe('Standing floor · right');
		expect(describeSpot('conference', 0.1, 0.5)).toBe('Podium · center');
		expect(describeSpot('blank', 0.5, 0.1)).toBe('left');
	});
});
