<script lang="ts">
	import type { SimulcastLayer } from '../contract';
	import { isAutoSelected, qualityHint, qualityOptions, type StreamQuality } from '../quality';

	let {
		quality,
		onPick
	}: {
		quality: StreamQuality;
		onPick: (layer: SimulcastLayer) => void;
	} = $props();
</script>

<select
	class="quality"
	class:quality--auto={isAutoSelected(quality)}
	class:quality--bad={isAutoSelected(quality) && quality.quality === 'bad'}
	aria-label="Stream quality"
	title={qualityHint(quality)}
	value={quality.layer}
	onchange={(e) => onPick(e.currentTarget.value as SimulcastLayer)}
>
	{#each qualityOptions(quality) as option (option.value)}
		<option value={option.value}>{option.label}</option>
	{/each}
</select>

<style>
	.quality {
		padding: 2px 6px;
		border-radius: 999px;
		border: 1px solid transparent;
		font-size: 11px;
		color: #fff;
		background: rgba(0, 0, 0, 0.55);
		cursor: pointer;
	}

	.quality option {
		color: initial;
	}

	/* Lowered automatically: yellow for loss, red for heavy loss. */
	.quality--auto {
		border-color: #facc15;
	}

	.quality--bad {
		border-color: #f87171;
	}
</style>
