<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Button from '$lib/components/Button.svelte';
	import StageEditor from '$lib/components/StageEditor.svelte';
	import { findAdminKey, saveAdminKey } from '$lib/admin';
	import { getStage, isRoomNotFound, openRoomEvents } from '$lib/api';
	import type { Stage } from '$lib/contract';

	let roomId = $derived(page.params.id as string);
	let adminKey = $state<string | null>(null);
	let stage = $state<Stage | null>(null);
	let problem = $state<string | null>(null);
	let checkingAdmin = $state(true);
	let pastedKey = $state('');

	$effect(() => {
		const id = roomId;
		let stop: (() => void) | undefined;
		getStage(id)
			.then((initial) => {
				stage = initial;
				// New streamers show up in the tray while the admin arranges.
				stop = openRoomEvents(id, {
					onStage: (next) => (stage = next),
					onRoomClosed: () => (problem = 'This room has been ended.')
				});
			})
			.catch((err) => {
				problem = isRoomNotFound(err)
					? "This room doesn't exist."
					: err instanceof Error
						? err.message
						: 'Could not load the stage';
			});
		void findAdminKey(id).then((key) => {
			adminKey = key;
			checkingAdmin = false;
		});
		return () => stop?.();
	});

	function backToRoom() {
		goto(resolve(`/create?room=${encodeURIComponent(roomId)}`));
	}

	function useKey() {
		const key = pastedKey.trim();
		if (!key) return;
		saveAdminKey(roomId, key);
		adminKey = key;
	}
</script>

<Screen>
	<div class="screen__body">
		{#if problem}
			<p style="color: var(--live); font-size: 13px;">{problem}</p>
			<Button onclick={() => goto(resolve('/home'))}>Back to home</Button>
		{:else if checkingAdmin || !stage}
			<p class="muted">Loading…</p>
		{:else if !adminKey}
			<div class="stack">
				<h2 style="margin: 0; font-size: 18px;">Admin only</h2>
				<p class="muted" style="margin: 0; font-size: 13px;">
					Only the device that created room {roomId} can set up its stage. If you created it on another
					device, paste the room's admin key.
				</p>
				<input class="field" placeholder="Admin key (k_…)" bind:value={pastedKey} />
				<Button variant="filled" onclick={useKey}>Use key</Button>
				<Button onclick={backToRoom}>Back</Button>
			</div>
		{:else}
			<StageEditor {roomId} {adminKey} {stage} onSaved={backToRoom} onCancel={backToRoom} />
		{/if}
	</div>
</Screen>
