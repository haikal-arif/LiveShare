<script lang="ts">
	import Icon from '../icons/Icon.svelte';
	import QualityPicker from './QualityPicker.svelte';
	import StageMap from './StageMap.svelte';
	import type { SimulcastLayer, StageTemplate } from '../contract';
	import type { StreamQuality } from '../quality';
	import type { Pin } from '../stage/stage';

	let {
		name,
		spot = null,
		stream,
		quality = null,
		template = null,
		pins = [],
		activeId = null,
		onPickLayer,
		onBack,
		onPrev,
		onNext
	}: {
		name: string;
		/** Where on the stage this camera is, e.g. "Standing floor · right". */
		spot?: string | null;
		stream: MediaStream | null;
		quality?: StreamQuality | null;
		/** With a template, a mini stage map shows where this camera is. */
		template?: StageTemplate | null;
		pins?: Pin[];
		activeId?: string | null;
		onPickLayer: (layer: SimulcastLayer) => void;
		onBack: () => void;
		onPrev?: () => void;
		onNext?: () => void;
	} = $props();

	let videoEl: HTMLVideoElement | undefined = $state();

	$effect(() => {
		if (videoEl) videoEl.srcObject = stream;
	});
</script>

<div class="full" role="dialog" aria-label="Watching {name}">
	{#if stream}
		<video bind:this={videoEl} autoplay playsinline></video>
	{:else}
		<div class="full__waiting"><Icon name="user" size={40} /><span>connecting…</span></div>
	{/if}

	<div class="full__top">
		<button class="icon-btn" onclick={onBack} aria-label="Back to stage">
			<Icon name="arrow-left" size={18} />
		</button>
		<div class="full__title">
			<strong>{name}</strong>
			{#if spot}<span>{spot}</span>{/if}
		</div>
		{#if quality}<QualityPicker {quality} onPick={onPickLayer} />{/if}
	</div>

	{#if onPrev}
		<button class="full__nav full__nav--prev" onclick={onPrev} aria-label="Previous streamer"
			>‹</button
		>
	{/if}
	{#if onNext}
		<button class="full__nav full__nav--next" onclick={onNext} aria-label="Next streamer">›</button>
	{/if}

	{#if template}
		<button class="full__map" onclick={onBack} aria-label="Show the whole stage">
			<StageMap {template} {pins} {activeId} mini />
		</button>
	{/if}
</div>

<style>
	.full {
		position: fixed;
		inset: 0;
		z-index: 20;
		background: #111;
		color: #fff;
	}

	video {
		width: 100%;
		height: 100%;
		object-fit: contain;
	}

	.full__waiting {
		height: 100%;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 8px;
		color: rgba(255, 255, 255, 0.6);
		font-size: 13px;
	}

	.full__top {
		position: absolute;
		top: 16px;
		left: 12px;
		right: 12px;
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.full__top .icon-btn {
		background: rgba(0, 0, 0, 0.45);
		border-color: transparent;
		color: #fff;
	}

	.full__title {
		flex: 1;
		display: flex;
		flex-direction: column;
		text-shadow: 0 1px 2px rgba(0, 0, 0, 0.6);
	}

	.full__title span {
		font-size: 12px;
		opacity: 0.85;
	}

	.full__nav {
		position: absolute;
		top: 50%;
		transform: translateY(-50%);
		width: 40px;
		height: 56px;
		border: none;
		background: rgba(0, 0, 0, 0.25);
		color: #fff;
		font-size: 28px;
		cursor: pointer;
	}

	.full__nav--prev {
		left: 0;
		border-radius: 0 8px 8px 0;
	}

	.full__nav--next {
		right: 0;
		border-radius: 8px 0 0 8px;
	}

	.full__map {
		position: absolute;
		right: 12px;
		bottom: 24px;
		width: 84px;
		padding: 0;
		border: 1.5px solid #fff;
		border-radius: 8px;
		overflow: hidden;
		background: #333;
		cursor: pointer;
	}
</style>
