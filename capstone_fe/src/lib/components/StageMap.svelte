<script lang="ts">
	import Icon from '../icons/Icon.svelte';
	import type { StageTemplate } from '../contract';
	import { toStagePosition, type Pin } from '../stage/stage';

	let {
		template,
		pins,
		activeId = null,
		mini = false,
		editable = false,
		element = $bindable(),
		onPinClick,
		onMapClick,
		onMove
	}: {
		template: StageTemplate;
		pins: Pin[];
		/** Highlighted pin (e.g. the stream being watched). */
		activeId?: string | null;
		/** Small overview: no labels, not interactive. */
		mini?: boolean;
		/** Pins can be dragged to new spots. */
		editable?: boolean;
		element?: HTMLDivElement;
		onPinClick?: (pin: Pin) => void;
		/** A tap on an empty spot, as a 0-1 position. */
		onMapClick?: (x: number, y: number) => void;
		onMove?: (streamerId: string, x: number, y: number) => void;
	} = $props();

	/** The pin being dragged and where it is now. */
	let drag = $state<{ id: string; x: number; y: number; moved: boolean } | null>(null);

	const DRAG_THRESHOLD = 0.02;

	function position(event: PointerEvent) {
		return toStagePosition(event.clientX, event.clientY, element!.getBoundingClientRect());
	}

	function pinDown(event: PointerEvent, pin: Pin) {
		if (!editable || !element) return;
		event.stopPropagation();
		(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
		drag = { id: pin.streamerId, x: pin.x, y: pin.y, moved: false };
	}

	function pinMove(event: PointerEvent, pin: Pin) {
		if (!drag || drag.id !== pin.streamerId) return;
		const { x, y } = position(event);
		const moved = drag.moved || Math.hypot(x - pin.x, y - pin.y) > DRAG_THRESHOLD;
		drag = { id: pin.streamerId, x, y, moved };
	}

	function pinUp(pin: Pin) {
		if (!drag || drag.id !== pin.streamerId) return;
		const { x, y, moved } = drag;
		drag = null;
		if (moved) onMove?.(pin.streamerId, x, y);
		else onPinClick?.(pin);
	}

	function mapClick(event: MouseEvent) {
		if (!onMapClick || !element || event.target !== event.currentTarget) return;
		const { x, y } = toStagePosition(event.clientX, event.clientY, element.getBoundingClientRect());
		onMapClick(x, y);
	}

	function pinClick(event: MouseEvent, pin: Pin) {
		event.stopPropagation();
		// Editable pins report clicks from pointerup instead, to tell taps from drags.
		if (!editable) onPinClick?.(pin);
	}

	const at = (pin: Pin) => (drag?.id === pin.streamerId ? drag : pin);
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
	class="stage-map"
	class:stage-map--mini={mini}
	class:stage-map--pickable={!!onMapClick}
	bind:this={element}
	onclick={mapClick}
	data-template={template}
>
	<svg viewBox="0 0 200 260" preserveAspectRatio="none" aria-hidden="true">
		<rect width="200" height="260" rx="8" fill="#3b3b3b" />
		{#if template === 'concert'}
			<rect x="40" y="10" width="120" height="6" rx="2" fill="#6b6b6b" />
			<path d="M24 22 H176 L160 72 H40 Z" fill="#8f8f8f" />
			<text
				x="100"
				y="50"
				font-size="10"
				font-weight="700"
				fill="#e9e9e9"
				text-anchor="middle"
				letter-spacing="2">STAGE</text
			>
			<line
				x1="34"
				y1="80"
				x2="166"
				y2="80"
				stroke="#777"
				stroke-width="1.5"
				stroke-dasharray="4 3"
			/>
			<rect x="18" y="86" width="164" height="92" rx="6" fill="#474747" />
			<text x="100" y="134" font-size="6" fill="#8d8d8d" text-anchor="middle">STANDING FLOOR</text>
			<rect x="84" y="186" width="32" height="18" rx="2" fill="#5a5a5a" />
			<text x="100" y="197" font-size="5" fill="#bbb" text-anchor="middle">FOH</text>
			{#each [212, 224, 236, 248] as y (y)}
				<rect x="18" {y} width="60" height="7" rx="2" fill="#555" />
				<rect x="122" {y} width="60" height="7" rx="2" fill="#555" />
			{/each}
		{:else if template === 'conference'}
			<rect x="40" y="12" width="120" height="8" rx="2" fill="#6b6b6b" />
			<text x="100" y="18" font-size="4.5" fill="#ccc" text-anchor="middle">SCREEN</text>
			<rect x="78" y="32" width="44" height="22" rx="3" fill="#8f8f8f" />
			<text x="100" y="46" font-size="7" fill="#eee" text-anchor="middle">PODIUM</text>
			{#each [76, 98, 120, 142, 164, 186, 208, 230] as y (y)}
				<rect x="18" {y} width="72" height="12" rx="3" fill="#555" />
				<rect x="110" {y} width="72" height="12" rx="3" fill="#555" />
			{/each}
		{:else if template === 'theater'}
			<path d="M16 14 H184 V66 Q100 92 16 66 Z" fill="#8f8f8f" />
			<text
				x="100"
				y="48"
				font-size="10"
				font-weight="700"
				fill="#e9e9e9"
				text-anchor="middle"
				letter-spacing="2">STAGE</text
			>
			{#each [112, 136, 160, 184, 208, 232] as y, i (y)}
				<path
					d="M{28 - i * 3} {y} Q100 {y + 22} {172 + i * 3} {y}"
					stroke="#555"
					stroke-width="9"
					fill="none"
					stroke-linecap="round"
				/>
			{/each}
		{:else}
			<rect
				x="6"
				y="6"
				width="188"
				height="248"
				rx="6"
				fill="none"
				stroke="#5a5a5a"
				stroke-width="1.5"
				stroke-dasharray="6 5"
			/>
		{/if}
	</svg>

	{#each pins as pin (pin.streamerId)}
		{@const spot = at(pin)}
		<button
			type="button"
			class="pin"
			class:pin--active={pin.streamerId === activeId}
			class:pin--offline={!pin.live}
			class:pin--dragging={drag?.id === pin.streamerId}
			style="left: {spot.x * 100}%; top: {spot.y * 100}%;"
			aria-label={pin.live ? `Watch ${pin.name}` : `${pin.name} (offline)`}
			tabindex={mini ? -1 : 0}
			onclick={(e) => pinClick(e, pin)}
			onpointerdown={(e) => pinDown(e, pin)}
			onpointermove={(e) => pinMove(e, pin)}
			onpointerup={() => pinUp(pin)}
			onpointercancel={() => (drag = null)}
		>
			<span class="pin__avatar">
				<Icon name="user" size={mini ? 8 : 14} />
				{#if pin.live}<span class="pin__live"></span>{/if}
			</span>
			{#if !mini}<span class="pin__name">{pin.name}</span>{/if}
		</button>
	{/each}
</div>

<style>
	.stage-map {
		position: relative;
		width: 100%;
		aspect-ratio: 200 / 260;
		user-select: none;
		touch-action: none;
	}

	/* On a wide screen the full stage stays a portrait block, not a stretched one. */
	.stage-map:not(.stage-map--mini) {
		max-width: 420px;
		margin: 0 auto;
	}

	.stage-map--pickable {
		cursor: crosshair;
	}

	svg {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		pointer-events: none;
	}

	.pin {
		position: absolute;
		transform: translate(-50%, -50%);
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		padding: 0;
		border: none;
		background: none;
		color: #fff;
		cursor: pointer;
		touch-action: none;
	}

	.pin__avatar {
		position: relative;
		width: 30px;
		height: 30px;
		border-radius: 50%;
		background: #8a8a8a;
		border: 2px solid #e5e5e5;
		display: flex;
		align-items: center;
		justify-content: center;
	}

	.pin--active .pin__avatar {
		border-color: #7dd3fc;
		box-shadow: 0 0 0 3px rgba(125, 211, 252, 0.35);
	}

	.pin--offline {
		opacity: 0.5;
	}

	.pin--dragging {
		z-index: 2;
		cursor: grabbing;
	}

	.pin__live {
		position: absolute;
		top: -2px;
		right: -2px;
		width: 9px;
		height: 9px;
		border-radius: 50%;
		background: var(--live);
		border: 1.5px solid #222;
	}

	.pin__name {
		max-width: 80px;
		padding: 1px 7px;
		border-radius: 999px;
		background: rgba(0, 0, 0, 0.6);
		font-size: 10px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.stage-map--mini .pin {
		pointer-events: none;
	}

	.stage-map--mini .pin__avatar {
		width: 12px;
		height: 12px;
		border-width: 1px;
	}

	.stage-map--mini .pin__live {
		display: none;
	}
</style>
