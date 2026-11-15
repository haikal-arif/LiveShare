<script lang="ts">
	import Icon from '../icons/Icon.svelte';
	import type { SimulcastLayer } from '../contract';
	import type { StreamQuality } from '../quality';
	import QualityPicker from './QualityPicker.svelte';

	let {
		stream = null,
		label,
		highlighted = false,
		muted = false,
		mirrored = false,
		quality = null,
		onPickLayer,
		onFocus
	}: {
		stream?: MediaStream | null;
		label: string;
		highlighted?: boolean;
		muted?: boolean;
		/** Display-only left-right flip, for the publisher's own preview. */
		mirrored?: boolean;
		/** For a watched stream: drives the quality picker. */
		quality?: StreamQuality | null;
		onPickLayer?: (layer: SimulcastLayer) => void;
		/** Makes the tile clickable: opens it full screen. */
		onFocus?: () => void;
	} = $props();

	let videoEl: HTMLVideoElement | undefined = $state();

	$effect(() => {
		if (videoEl) videoEl.srcObject = stream ?? null;
	});

	/** Button semantics, keyboard and click, only when the tile can be focused. */
	const focusAttrs = $derived(
		onFocus
			? {
					role: 'button',
					tabindex: 0,
					'aria-label': `Focus ${label}`,
					onclick: onFocus,
					onkeydown: (e: KeyboardEvent) => {
						if (e.key === 'Enter' || e.key === ' ') {
							e.preventDefault();
							onFocus();
						}
					}
				}
			: {}
	);
</script>

<div
	class="tile"
	class:tile--highlighted={highlighted}
	class:tile--focusable={!!onFocus}
	{...focusAttrs}
>
	{#if stream}
		<video bind:this={videoEl} class:mirrored autoplay playsinline {muted}></video>
	{:else}
		<div class="tile__placeholder">
			<div class="avatar">
				<Icon name="user" size={24} />
			</div>
			<span class="muted tile__connecting">connecting…</span>
		</div>
	{/if}
	<span class="tile__label">{label}</span>
	{#if stream && quality && onPickLayer}
		<!-- Changing quality must not also focus the tile. -->
		<div class="tile__quality" onclick={(e) => e.stopPropagation()} role="presentation">
			<QualityPicker {quality} onPick={onPickLayer} />
		</div>
	{/if}
</div>

<style>
	.tile {
		position: relative;
		aspect-ratio: 3 / 4;
		border-radius: 12px;
		overflow: hidden;
		background: linear-gradient(160deg, #4b4c52, #2a2b30);
		border: 2px solid transparent;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.tile--highlighted {
		border-color: #7dd3fc;
	}

	.tile--focusable {
		cursor: pointer;
	}

	.tile--focusable:focus-visible {
		outline: 2px solid #7dd3fc;
		outline-offset: 2px;
	}

	video {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	video.mirrored {
		transform: scaleX(-1);
	}

	.tile__placeholder {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 8px;
	}

	.tile__placeholder .avatar {
		background: rgba(255, 255, 255, 0.15);
		color: #fff;
	}

	.tile__connecting {
		font-size: 11px;
		color: rgba(255, 255, 255, 0.75) !important;
	}

	.tile__quality {
		position: absolute;
		top: 6px;
		right: 6px;
	}

	.tile__label {
		position: absolute;
		bottom: 6px;
		left: 8px;
		font-size: 11px;
		color: #fff;
		text-shadow: 0 1px 2px rgba(0, 0, 0, 0.6);
	}
</style>
