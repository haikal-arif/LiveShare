import type { Placement, Stage, StageTemplate, Streamer } from '../contract';

export interface TemplateInfo {
	id: StageTemplate;
	name: string;
	hint: string;
}

export const TEMPLATES: TemplateInfo[] = [
	{ id: 'concert', name: 'Concert', hint: 'Stage + standing floor' },
	{ id: 'conference', name: 'Conference', hint: 'Podium + seat rows' },
	{ id: 'theater', name: 'Theater', hint: 'Curved seating' },
	{ id: 'blank', name: 'Blank', hint: 'Free positioning' }
];

/** A streamer as drawn on the stage map. */
export interface Pin {
	streamerId: string;
	peerId: string;
	name: string;
	live: boolean;
	x: number;
	y: number;
}

/** Streamers with a placement become pins; the rest are "not on stage yet". */
export function splitStage(stage: Stage): { pins: Pin[]; unplaced: Streamer[] } {
	const byId = new Map(stage.streamers.map((s) => [s.streamer_id, s]));
	const pins: Pin[] = [];
	for (const p of stage.placements) {
		const streamer = byId.get(p.streamer_id);
		if (!streamer) continue;
		pins.push({
			streamerId: streamer.streamer_id,
			peerId: streamer.peer_id,
			name: streamer.display_name,
			live: streamer.live,
			x: p.x,
			y: p.y
		});
	}
	const placed = new Set(stage.placements.map((p) => p.streamer_id));
	return { pins, unplaced: stage.streamers.filter((s) => !placed.has(s.streamer_id)) };
}

const clamp01 = (n: number) => Math.min(1, Math.max(0, n));

/** A pointer position over the map element as a 0-1 stage position. */
export function toStagePosition(
	clientX: number,
	clientY: number,
	rect: { left: number; top: number; width: number; height: number }
): { x: number; y: number } {
	// Two decimals is finer than a finger and keeps payloads tidy.
	const round = (n: number) => Math.round(n * 100) / 100;
	return {
		x: round(clamp01((clientX - rect.left) / rect.width)),
		y: round(clamp01((clientY - rect.top) / rect.height))
	};
}

/** Moves or adds one placement, keeping every other placement as it is. */
export function placeStreamer(
	placements: Placement[],
	streamerId: string,
	x: number,
	y: number
): Placement[] {
	const rest = placements.filter((p) => p.streamer_id !== streamerId);
	return [...rest, { streamer_id: streamerId, x, y }];
}

export function removePlacement(placements: Placement[], streamerId: string): Placement[] {
	return placements.filter((p) => p.streamer_id !== streamerId);
}

/** Label for where a pin sits, shown under the name in the full view. */
export function describeSpot(template: StageTemplate, y: number, x: number): string {
	const side = x < 0.4 ? 'left' : x > 0.6 ? 'right' : 'center';
	if (template === 'blank') return side;
	const zones: Record<Exclude<StageTemplate, 'blank'>, [number, string][]> = {
		concert: [
			[0.3, 'Stage'],
			[0.7, 'Standing floor'],
			[1, 'Seats']
		],
		conference: [
			[0.25, 'Podium'],
			[1, 'Seating']
		],
		theater: [
			[0.35, 'Stage'],
			[1, 'Seating']
		]
	};
	const zone = zones[template].find(([limit]) => y <= limit)?.[1] ?? '';
	return `${zone} · ${side}`;
}
