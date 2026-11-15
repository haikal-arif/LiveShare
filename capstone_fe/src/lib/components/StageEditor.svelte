<script lang="ts">
	import { untrack } from 'svelte';
	import Button from './Button.svelte';
	import StageMap from './StageMap.svelte';
	import Icon from '../icons/Icon.svelte';
	import { ApiRequestError, setStage } from '../api';
	import type { Placement, Stage, StageTemplate } from '../contract';
	import {
		placeStreamer,
		removePlacement,
		splitStage,
		TEMPLATES,
		toStagePosition
	} from '../stage/stage';

	let {
		roomId,
		adminKey,
		stage,
		onSaved,
		onCancel
	}: {
		roomId: string;
		adminKey: string;
		/** The live stage; new streamers show up in the tray as they register. */
		stage: Stage;
		onSaved: (stage: Stage) => void;
		onCancel: () => void;
	} = $props();

	// The draft starts from what's saved and is only sent on "Save Layout".
	let step = $state<'template' | 'arrange'>(
		untrack(() => (stage.template ? 'arrange' : 'template'))
	);
	let template = $state<StageTemplate>(untrack(() => stage.template ?? 'concert'));
	let placements = $state<Placement[]>(untrack(() => stage.placements));

	let draft = $derived(splitStage({ ...stage, template, placements }));
	/** Chip picked for tap-to-place, or the pin whose menu is open. */
	let selectedChip = $state<string | null>(null);
	let selectedPin = $state<string | null>(null);
	let saving = $state(false);
	let error = $state<string | null>(null);

	let mapEl: HTMLDivElement | undefined = $state();
	/** A chip being dragged from the tray, at its pointer position. */
	let chipDrag = $state<{ id: string; name: string; clientX: number; clientY: number } | null>(
		null
	);

	function place(streamerId: string, x: number, y: number) {
		placements = placeStreamer(placements, streamerId, x, y);
		selectedChip = null;
		selectedPin = null;
	}

	function chipDown(event: PointerEvent, id: string, name: string) {
		chipDrag = { id, name, clientX: event.clientX, clientY: event.clientY };
	}

	function windowMove(event: PointerEvent) {
		if (chipDrag) chipDrag = { ...chipDrag, clientX: event.clientX, clientY: event.clientY };
	}

	function windowUp(event: PointerEvent) {
		if (!chipDrag || !mapEl) return;
		const { id } = chipDrag;
		chipDrag = null;
		const rect = mapEl.getBoundingClientRect();
		const inside =
			event.clientX >= rect.left &&
			event.clientX <= rect.right &&
			event.clientY >= rect.top &&
			event.clientY <= rect.bottom;
		if (inside) {
			const { x, y } = toStagePosition(event.clientX, event.clientY, rect);
			place(id, x, y);
		} else {
			// Released where it started: a tap, so pick it for tap-to-place.
			selectedChip = selectedChip === id ? null : id;
		}
	}

	async function save(body: { template: StageTemplate | null; placements: Placement[] }) {
		saving = true;
		error = null;
		try {
			onSaved(await setStage(roomId, adminKey, body));
		} catch (err) {
			error =
				err instanceof ApiRequestError && err.code === 'not_admin'
					? "This device isn't the room's admin any more."
					: err instanceof Error
						? err.message
						: 'Saving failed';
		} finally {
			saving = false;
		}
	}

	let selectedPinName = $derived(draft.pins.find((p) => p.streamerId === selectedPin)?.name);
</script>

<svelte:window onpointermove={windowMove} onpointerup={windowUp} />

<div class="editor">
	{#if step === 'template'}
		<div class="editor__head">
			<h2>Choose a Stage</h2>
			<p class="muted">Pick the venue that looks most like your event.</p>
		</div>
		<div class="templates">
			{#each TEMPLATES as option (option.id)}
				<button
					type="button"
					class="template"
					class:template--selected={template === option.id}
					aria-pressed={template === option.id}
					onclick={() => (template = option.id)}
				>
					<StageMap template={option.id} pins={[]} mini />
					<strong>{option.name}</strong>
					<span class="muted">{option.hint}</span>
				</button>
			{/each}
		</div>
		{#if stage.template}
			<button
				type="button"
				class="link"
				disabled={saving}
				onclick={() => save({ template: null, placements: [] })}
			>
				Remove the stage, show the grid instead
			</button>
		{/if}
		<div class="editor__actions">
			<Button onclick={onCancel}>Cancel</Button>
			<Button variant="filled" onclick={() => (step = 'arrange')}>Next</Button>
		</div>
	{:else}
		<div class="editor__head">
			<h2>Arrange Streamers</h2>
			<p class="muted">
				{selectedChip
					? 'Tap the stage where they are filming from.'
					: 'Drag streamers to where they are filming from.'}
			</p>
		</div>

		<StageMap
			{template}
			pins={draft.pins}
			activeId={selectedPin}
			editable
			bind:element={mapEl}
			onMove={place}
			onPinClick={(pin) => {
				selectedChip = null;
				selectedPin = selectedPin === pin.streamerId ? null : pin.streamerId;
			}}
			onMapClick={(x, y) => {
				if (selectedChip) place(selectedChip, x, y);
				else selectedPin = null;
			}}
		/>

		{#if selectedPin}
			<div class="pin-menu">
				<span>{selectedPinName}</span>
				<button
					type="button"
					class="link"
					onclick={() => {
						placements = removePlacement(placements, selectedPin!);
						selectedPin = null;
					}}
				>
					Remove from stage
				</button>
			</div>
		{/if}

		<div class="tray">
			<span class="muted tray__title">Not on stage yet ({draft.unplaced.length})</span>
			{#if draft.unplaced.length === 0}
				<span class="muted tray__empty">
					{stage.streamers.length === 0
						? 'Streamers appear here once they join.'
						: 'Everyone is on stage.'}
				</span>
			{/if}
			<div class="chips">
				{#each draft.unplaced as streamer (streamer.streamer_id)}
					<button
						type="button"
						class="chip"
						class:chip--selected={selectedChip === streamer.streamer_id}
						class:chip--offline={!streamer.live}
						onpointerdown={(e) => chipDown(e, streamer.streamer_id, streamer.display_name)}
					>
						<Icon name="user" size={12} />
						{streamer.display_name}
					</button>
				{/each}
			</div>
		</div>

		{#if error}<p class="error">{error}</p>{/if}

		<div class="editor__actions">
			<Button onclick={() => (step = 'template')}>Back</Button>
			<Button variant="filled" disabled={saving} onclick={() => save({ template, placements })}>
				{saving ? 'Saving…' : 'Save Layout'}
			</Button>
		</div>
	{/if}
	{#if error && step === 'template'}<p class="error">{error}</p>{/if}
</div>

{#if chipDrag}
	<div class="chip chip--ghost" style="left: {chipDrag.clientX}px; top: {chipDrag.clientY}px;">
		<Icon name="user" size={12} />
		{chipDrag.name}
	</div>
{/if}

<style>
	.editor {
		display: flex;
		flex-direction: column;
		gap: 14px;
		flex: 1;
	}

	.editor__head h2 {
		margin: 0;
		font-size: 18px;
	}

	.editor__head p {
		margin: 4px 0 0;
		font-size: 13px;
	}

	.templates {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 12px;
	}

	.template {
		display: flex;
		flex-direction: column;
		gap: 4px;
		padding: 8px;
		border: 1px solid var(--border);
		border-radius: var(--radius-md);
		background: var(--control-bg);
		color: var(--text);
		text-align: left;
		cursor: pointer;
		font-size: 13px;
	}

	.template span {
		font-size: 11px;
	}

	.template--selected {
		border: 2px solid #7dd3fc;
		padding: 7px;
	}

	.editor__actions {
		display: flex;
		gap: 10px;
		margin-top: auto;
	}

	.link {
		align-self: flex-start;
		padding: 0;
		border: none;
		background: none;
		color: var(--text-muted);
		font-size: 13px;
		text-decoration: underline;
		cursor: pointer;
	}

	.pin-menu {
		display: flex;
		justify-content: space-between;
		align-items: center;
		padding: 8px 12px;
		border-radius: var(--radius-sm);
		background: var(--bg-elevated);
		font-size: 13px;
	}

	.tray__title {
		font-size: 12px;
	}

	.tray__empty {
		display: block;
		font-size: 12px;
		margin-top: 6px;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		margin-top: 6px;
	}

	.chip {
		display: inline-flex;
		align-items: center;
		gap: 5px;
		padding: 5px 10px;
		border: 1px solid var(--border);
		border-radius: 999px;
		background: var(--bg-elevated);
		color: var(--text);
		font-size: 12px;
		cursor: grab;
		touch-action: none;
	}

	.chip--selected {
		border-color: #7dd3fc;
		box-shadow: 0 0 0 2px rgba(125, 211, 252, 0.35);
	}

	.chip--offline {
		opacity: 0.6;
	}

	.chip--ghost {
		position: fixed;
		transform: translate(-50%, -50%);
		pointer-events: none;
		z-index: 10;
		border-color: #7dd3fc;
		box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
	}

	.error {
		margin: 0;
		color: var(--live);
		font-size: 13px;
	}
</style>
