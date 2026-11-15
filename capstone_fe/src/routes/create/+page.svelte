<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon from '$lib/icons/Icon.svelte';
	import { buildRoomJoinUrl, renderQrToCanvas } from '$lib/qr';
	import { ApiRequestError, createRoom } from '$lib/api';
	import { findAdminKey, getAdminKey, saveAdminKey } from '$lib/admin';

	/** Set once the server has created (or handed back) the room. */
	let roomId = $state<string | null>(page.url.searchParams.get('room'));
	let isAdmin = $state(false);
	/** Someone else's room is already open on this server (409). */
	let otherRoom = $state<string | null>(null);
	let error = $state<string | null>(null);
	/** The server's room passcode, required to create a room. */
	let passcode = $state('');
	let creating = $state(false);
	let joinUrl = $derived(roomId ? buildRoomJoinUrl(page.url.origin, roomId) : '');

	function showRoom(id: string) {
		roomId = id;
		// Coming back here (e.g. from the stage editor) must not create another room.
		goto(resolve(`/create?room=${encodeURIComponent(id)}`), { replaceState: true });
	}

	$effect(() => {
		if (!roomId) return;
		const id = roomId;
		void findAdminKey(id).then((key) => (isAdmin = key !== null));
	});

	async function submitPasscode(event: SubmitEvent) {
		event.preventDefault();
		if (creating) return;
		creating = true;
		error = null;
		try {
			const created = await createRoom(passcode);
			saveAdminKey(created.room_id, created.admin_token);
			isAdmin = true;
			showRoom(created.room_id);
		} catch (err) {
			if (err instanceof ApiRequestError && err.code === 'room_exists' && err.roomId) {
				// Ours if this device holds (or can reclaim) its key.
				if (getAdminKey(err.roomId) || (await findAdminKey(err.roomId))) showRoom(err.roomId);
				else otherRoom = err.roomId;
			} else if (err instanceof ApiRequestError && err.code === 'invalid_passcode') {
				error = 'Wrong passcode. Try again.';
			} else {
				error = err instanceof Error ? err.message : 'Could not create the room';
			}
		} finally {
			creating = false;
		}
	}

	let canvasEl: HTMLCanvasElement | undefined = $state();
	let copied = $state(false);

	$effect(() => {
		if (canvasEl && joinUrl) void renderQrToCanvas(canvasEl, joinUrl);
	});

	async function copyLink() {
		await navigator.clipboard.writeText(joinUrl);
		copied = true;
		setTimeout(() => (copied = false), 1500);
	}

	async function share() {
		if (navigator.share) {
			try {
				await navigator.share({ title: 'FlashLive room', url: joinUrl });
				return;
			} catch {
				// user cancelled the share sheet — fall through to clipboard
			}
		}
		await copyLink();
	}
</script>

<Screen>
	<div class="screen__topbar">
		<button class="icon-btn" onclick={() => goto(resolve('/home'))} aria-label="Back">
			<Icon name="arrow-left" size={18} />
		</button>
		<div style="display: flex; align-items: center; gap: 8px;">
			<Icon name="camera" size={18} />
			<span class="screen__topbar-title">FlashLive</span>
		</div>
	</div>

	<div class="screen__body">
		{#if otherRoom}
			<div class="center-col" style="gap: 12px; margin-top: 24px;">
				<p style="margin: 0; font-size: 15px;">A room is already open on this server.</p>
				<p class="muted" style="margin: 0; font-size: 13px;">
					Only one room at a time — join <strong>{otherRoom}</strong> instead.
				</p>
			</div>
			<div style="flex: 1;"></div>
			<div class="stack">
				<Button
					variant="filled"
					icon="login"
					onclick={() => goto(resolve('/room/[id]/join?role=publisher', { id: otherRoom! }))}
				>
					Join as Streamer
				</Button>
				<Button
					icon="cast"
					onclick={() => goto(resolve('/room/[id]?role=subscriber', { id: otherRoom! }))}
				>
					Watch Live Streaming
				</Button>
			</div>
		{:else if !roomId}
			<form class="stack" style="margin-top: 24px;" onsubmit={submitPasscode}>
				<p style="margin: 0; font-size: 15px;">Enter the room passcode to create a room.</p>
				<input
					class="field"
					type="password"
					placeholder="Passcode"
					autocomplete="off"
					bind:value={passcode}
				/>
				{#if error}
					<p style="color: var(--live); font-size: 13px; margin: 0;">{error}</p>
				{/if}
				<Button variant="filled" icon="login" type="submit" disabled={creating || !passcode}>
					{creating ? 'Creating…' : 'Create Room'}
				</Button>
			</form>
		{:else}
			<div class="center-col" style="gap: 4px;">
				<p style="margin: 0; font-size: 15px;">Your room is ready!</p>
			</div>

			<div class="card center-col" style="margin-top: 20px; gap: 12px;">
				<canvas bind:this={canvasEl} width="220" height="220"></canvas>
				<Button variant="ghost" icon="copy" onclick={copyLink}>
					{copied ? 'Copied!' : roomId}
				</Button>
			</div>

			<p class="muted center-col" style="margin: 24px 0 12px; font-size: 13px;">
				Share this QR to your friend for join the streaming
			</p>

			<div style="display: flex; justify-content: center;">
				<Button variant="ghost" icon="share" onclick={share}>Share room link</Button>
			</div>

			<div style="flex: 1;"></div>

			<div class="stack">
				{#if isAdmin}
					<Button icon="grid" onclick={() => goto(resolve('/room/[id]/stage', { id: roomId! }))}>
						Set Up Stage Layout
					</Button>
				{/if}
				<Button
					variant="filled"
					icon="login"
					onclick={() => goto(resolve('/room/[id]/join?role=publisher', { id: roomId! }))}
				>
					Join your room
				</Button>
			</div>
		{/if}
	</div>
</Screen>
