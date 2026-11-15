import type { ConnectionQuality, SimulcastLayer } from './contract';

/**
 * A watched stream's quality as the picker shows it. `picked` is what the
 * user chose; `layer` is what the SFU is actually forwarding, which is lower
 * after an automatic downgrade until the user picks again to retry.
 */
export interface StreamQuality {
	picked: SimulcastLayer;
	layer: SimulcastLayer;
	quality: ConnectionQuality;
}

export const INITIAL_STREAM_QUALITY: StreamQuality = {
	picked: 'high',
	layer: 'high',
	quality: 'good'
};

const LAYER_LABELS: Record<SimulcastLayer, string> = { high: 'High', mid: 'Mid', low: 'Low' };
const LAYERS: SimulcastLayer[] = ['high', 'mid', 'low'];

/** True when the SFU, not the user, chose the layer being forwarded. */
export function isAutoSelected(q: StreamQuality): boolean {
	return q.layer !== q.picked;
}

/** Picker options, with "(auto)" on the forwarded layer when the SFU chose it. */
export function qualityOptions(q: StreamQuality): { value: SimulcastLayer; label: string }[] {
	return LAYERS.map((value) => ({
		value,
		label:
			value === q.layer && isAutoSelected(q) ? `${LAYER_LABELS[value]} (auto)` : LAYER_LABELS[value]
	}));
}

/** Tooltip explaining an automatic downgrade and how to retry. */
export function qualityHint(q: StreamQuality): string {
	if (!isAutoSelected(q)) return 'Stream quality';
	return `Lowered from ${LAYER_LABELS[q.picked]} automatically: your connection couldn't keep up. Pick a higher quality to retry.`;
}
