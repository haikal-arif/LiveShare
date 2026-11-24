<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Icon from '$lib/icons/Icon.svelte';
	import VideoTile from '$lib/components/VideoTile.svelte';
	import StageMap from '$lib/components/StageMap.svelte';
	import StageEditor from '$lib/components/StageEditor.svelte';
	import FullStream from '$lib/components/FullStream.svelte';
	import Button from '$lib/components/Button.svelte';
	import { roomSession } from '$lib/room.svelte';
	import { buildRoomJoinUrl } from '$lib/qr';
	import { getAdminKey } from '$lib/admin';
	import { closeRoom } from '$lib/api';
	import type { PeerId } from '$lib/contract';
	import { describeSpot, splitStage, type Pin } from '$lib/stage/stage';
	import { isPhone } from '$lib/device';

	let roomId = $derived(page.params.id as string);
	let role = $derived(
		page.url.searchParams.get('role') === 'subscriber' ? 'subscriber' : 'publisher'
	);
	let roomTitle = $derived(roomId);
	const phone = isPhone();

	// Only re-run when the URL (roomId/role) changes, never when roomSession's
	// own reactive fields change — reading those via isActiveFor() inside an
	// untracked $state mutation would otherwise re-trigger this effect and
	// retry the connection in a loop.
	$effect(() => {
		const id = roomId;
		const r = role;
		untrack(() => {
			if (r === 'publisher') {
				if (!roomSession.isActiveFor(id, 'publisher')) {
					goto(resolve('/room/[id]/join?role=publisher', { id }));
				}
				return;
			}
			if (!roomSession.isActiveFor(id, 'subscriber')) {
				roomSession.startWatching(id).catch(() => {});
			}
		});
	});

	onDestroy(() => {
		roomSession.leave();
	});

	let participantCount = $derived(
		(role === 'publisher' ? 1 : 0) + roomSession.remotePublisherIds.length
	);

	async function shareLink() {
		const url = buildRoomJoinUrl(page.url.origin, roomId);
		if (navigator.share) {
			try {
				await navigator.share({ title: 'FlashLive room', url });
				return;
			} catch {
				// fall through to clipboard
			}
		}
		await navigator.clipboard.writeText(url);
	}

	function rename() {
		const next = prompt('Rename room', roomTitle);
		if (next && next.trim()) roomTitle = next.trim();
	}

	function leaveRoom() {
		roomSession.leave();
		goto(resolve('/home'));
	}

	// ---- Publisher's own view ----

	// Once the publisher is live, their camera fills the whole window
	// instead of sitting as one tile in the shared room layout.
	let selfFull = $derived(
		role === 'publisher' && roomSession.status === 'connected' && roomSession.localStream !== null
	);
	let selfVideo: HTMLVideoElement | undefined = $state();

	$effect(() => {
		if (selfVideo) selfVideo.srcObject = roomSession.previewStream;
	});

	const nameOf = (peerId: PeerId) =>
		roomSession.streamerFor(peerId)?.display_name ?? `User ${peerId.slice(0, 6)}`;

	// ---- Stage view (watchers, once the admin has set a stage) ----

	let stage = $derived(roomSession.stage);
	let template = $derived(stage?.template ?? null);
	let split = $derived(stage ? splitStage(stage) : { pins: [], unplaced: [] });
	let stageView = $derived(role === 'subscriber' && template !== null);
	let livePeers = $derived(new Set(roomSession.remotePublisherIds));
	/** Live streams not placed on the stage: unplaced streamers, or not registered (yet). */
	let moreStreams = $derived(
		roomSession.remotePublisherIds.filter(
			(peerId) => !split.pins.some((pin) => pin.peerId === peerId && pin.live)
		)
	);
	/** Everything that can be watched, in the order ‹ › steps through. */
	let watchable = $derived([
		...split.pins.filter((pin) => pin.live && livePeers.has(pin.peerId)).map((pin) => pin.peerId),
		...moreStreams
	]);

	let watching = $state<PeerId | null>(null);
	let watchingName = $state('');
	let notice = $state<string | null>(null);

	function watch(peerId: PeerId) {
		notice = null;
		watching = peerId;
		watchingName = nameOf(peerId);
	}

	function openPin(pin: Pin) {
		if (pin.live && livePeers.has(pin.peerId)) watch(pin.peerId);
		else notice = `${pin.name} isn't streaming right now.`;
	}

	function step(by: number) {
		if (!watching) return;
		const i = watchable.indexOf(watching);
		watch(watchable[(i + by + watchable.length) % watchable.length]);
	}

	// A stream ending while it's watched drops back to the stage (product 4.2).
	$effect(() => {
		if (watching && !livePeers.has(watching)) {
			untrack(() => {
				notice = `${watchingName} stopped streaming. Pick another spot on the stage.`;
				watching = null;
			});
		}
	});

	let watchingPin = $derived(split.pins.find((pin) => pin.peerId === watching) ?? null);

	// ---- Admin ----

	let editing = $state(false);
	let ending = $state(false);

	async function endRoom() {
		const key = getAdminKey(roomId);
		if (!key || !confirm('End this room for everyone?')) return;
		ending = true;
		try {
			await closeRoom(roomId, key);
			// room_closed from the SFU and the events stream updates the page.
		} catch (err) {
			alert(err instanceof Error ? err.message : 'Could not end the room');
		} finally {
			ending = false;
		}
	}
</script>

{#snippet publisherControls()}
	<button class="icon-btn" onclick={() => roomSession.toggleCamera()} aria-label="Toggle camera">
		<Icon name={roomSession.cameraEnabled ? 'video' : 'video-off'} size={18} />
	</button>
	<button
		class="icon-btn"
		class:icon-btn--active={roomSession.mirrorSelf}
		aria-pressed={roomSession.mirrorSelf}
		onclick={() => roomSession.toggleMirror()}
		aria-label="Mirror my view"
		title="Mirror my view (preview only)"
	>
		<Icon name="mirror" size={18} />
	</button>
	{#if phone}
		<button
			class="icon-btn"
			disabled={roomSession.flippingCamera || roomSession.screenSharing}
			onclick={() =>
				roomSession.flipCamera().catch((err) => {
					console.error('flipCamera failed', err);
					// The browser's error name (e.g. NotReadableError) says why.
					const reason = err instanceof Error ? `${err.name}: ${err.message}` : String(err);
					alert(`Could not switch camera.\n${reason}`);
				})}
			aria-label="Switch camera"
		>
			<Icon name="camera-flip" size={18} />
		</button>
	{/if}
	<button class="icon-btn" onclick={() => roomSession.toggleMic()} aria-label="Toggle mic">
		<Icon name={roomSession.micEnabled ? 'mic' : 'mic-off'} size={18} />
	</button>
	<button
		class="icon-btn"
		class:icon-btn--active={roomSession.screenSharing}
		onclick={() => roomSession.toggleScreenShare()}
		aria-label="Share screen"
	>
		<Icon name="monitor" size={18} />
	</button>
	{#if roomSession.isAdmin}
		<button
			class="btn btn--pill admin-btn"
			style="margin-left: auto;"
			onclick={() => (editing = true)}
		>
			<Icon name="edit" size={16} />
			Edit Stage
		</button>
	{/if}
	<button
		class="btn btn--pill"
		style="width: auto;"
		style:margin-left={roomSession.isAdmin ? null : 'auto'}
		onclick={shareLink}
	>
		<Icon name="share" size={16} />
		Share Link Room
	</button>
{/snippet}

{#if selfFull}
	<div class="self-full screen--dark">
		<video
			bind:this={selfVideo}
			class:mirrored={roomSession.mirrorSelf && !roomSession.screenSharing}
			autoplay
			playsinline
			muted
		></video>

		<div class="self-full__top">
			<span class="badge badge--live"><span class="dot"></span>LIVE</span>
			{#if roomSession.screenSharing}
				<span class="badge badge--screen" role="status">
					<Icon name="monitor" size={12} />
					Sharing screen
				</span>
			{/if}
			<span class="self-full__count">{participantCount} People</span>
			<button class="icon-btn" onclick={leaveRoom} aria-label="Leave room">
				<Icon name="x" size={18} />
			</button>
		</div>

		<div class="bottom-bar self-full__bar">
			{@render publisherControls()}
		</div>
		{#if roomSession.isAdmin}
			<button class="link end self-full__end" disabled={ending} onclick={endRoom}>
				{ending ? 'Ending…' : 'End room for everyone'}
			</button>
		{/if}
	</div>
{:else}
	<Screen theme="dark">
		<div class="screen__topbar" style="justify-content: space-between;">
			<div>
				<div class="screen__topbar-title">
					Live Room : <span class="muted">[{roomTitle}]</span>
				</div>
				<div style="display: flex; align-items: center; gap: 8px; margin-top: 4px;">
					<span class="badge badge--live"><span class="dot"></span>LIVE</span>
					<span class="muted" style="font-size: 12px;">{participantCount} People</span>
				</div>
			</div>
			<button class="icon-btn" onclick={leaveRoom} aria-label="Leave room">
				<Icon name="x" size={18} />
			</button>
		</div>

		<div class="screen__body">
			{#if roomSession.status === 'not_found' || roomSession.status === 'closed'}
				<div class="center-col ended">
					<Icon name="x" size={28} />
					<p>{roomSession.errorMessage}</p>
					<Button variant="filled" icon="arrow-left" onclick={() => goto(resolve('/home'))}>
						Back to home
					</Button>
				</div>
			{:else}
				{#if roomSession.errorMessage}
					<p style="color: var(--live); font-size: 13px;">{roomSession.errorMessage}</p>
				{/if}

				{#if stageView && template}
					<StageMap
						{template}
						pins={split.pins.map((pin) => ({
							...pin,
							live: pin.live && livePeers.has(pin.peerId)
						}))}
						activeId={watchingPin?.streamerId ?? null}
						onPinClick={openPin}
					/>
					{#if notice}<p class="notice">{notice}</p>{/if}
					{#if moreStreams.length > 0}
						<div class="more">
							<span class="muted">More streams</span>
							<div class="chips">
								{#each moreStreams as peerId (peerId)}
									<button type="button" class="chip" onclick={() => watch(peerId)}>
										<Icon name="user" size={12} />
										{nameOf(peerId)}
									</button>
								{/each}
							</div>
						</div>
					{/if}
				{:else}
					<div class="grid">
						{#if role === 'publisher'}
							<VideoTile
								stream={roomSession.previewStream}
								label={roomSession.displayName || 'You'}
								highlighted
								muted
								mirrored={roomSession.mirrorSelf && !roomSession.screenSharing}
							/>
						{/if}
						{#each roomSession.remotePublisherIds as peerId (peerId)}
							<VideoTile
								stream={roomSession.remoteStreams.get(peerId) ?? null}
								label={nameOf(peerId)}
								quality={roomSession.remoteQuality.get(peerId) ?? null}
								onPickLayer={(layer) => roomSession.pickLayer(peerId, layer)}
								onFocus={() => watch(peerId)}
							/>
						{/each}
					</div>
				{/if}

				<div style="flex: 1;"></div>

				<div class="bottom-bar">
					{#if role === 'publisher'}
						{@render publisherControls()}
					{:else}
						{#if roomSession.isAdmin}
							<button class="btn btn--pill admin-btn" onclick={() => (editing = true)}>
								<Icon name="edit" size={16} />
								Edit Stage
							</button>
						{/if}
						<button class="btn btn--pill" onclick={rename}>
							<Icon name="edit" size={16} />
							Rename
						</button>
						<button class="btn btn--pill" onclick={shareLink}>
							<Icon name="share" size={16} />
							Share Link Room
						</button>
					{/if}
				</div>
				{#if roomSession.isAdmin}
					<button class="link end" disabled={ending} onclick={endRoom}>
						{ending ? 'Ending…' : 'End room for everyone'}
					</button>
				{/if}
			{/if}
		</div>
	</Screen>
{/if}

{#if watching}
	<FullStream
		name={watchingName}
		spot={template && watchingPin ? describeSpot(template, watchingPin.y, watchingPin.x) : null}
		stream={roomSession.remoteStreams.get(watching) ?? null}
		quality={roomSession.remoteQuality.get(watching) ?? null}
		{template}
		pins={split.pins}
		activeId={watchingPin?.streamerId ?? null}
		onPickLayer={(layer) => watching && roomSession.pickLayer(watching, layer)}
		onBack={() => (watching = null)}
		onPrev={watchable.length > 1 ? () => step(-1) : undefined}
		onNext={watchable.length > 1 ? () => step(1) : undefined}
	/>
{/if}

{#if editing && roomSession.stage}
	<div class="editor-sheet">
		<Screen>
			<div class="screen__body">
				<StageEditor
					{roomId}
					adminKey={getAdminKey(roomId) ?? ''}
					stage={roomSession.stage}
					onSaved={() => (editing = false)}
					onCancel={() => (editing = false)}
				/>
			</div>
		</Screen>
	</div>
{/if}

<style>
	.self-full {
		position: fixed;
		inset: 0;
		z-index: 20;
		background: #111;
		color: #fff;
	}

	.self-full video {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.self-full video.mirrored {
		transform: scaleX(-1);
	}

	.self-full__top {
		position: absolute;
		top: 16px;
		left: 12px;
		right: 12px;
		display: flex;
		align-items: center;
		gap: 10px;
	}

	.self-full__top .icon-btn {
		margin-left: auto;
		background: rgba(0, 0, 0, 0.45);
		border-color: transparent;
		color: #fff;
	}

	.self-full__count {
		font-size: 12px;
		text-shadow: 0 1px 2px rgba(0, 0, 0, 0.6);
	}

	.self-full__bar {
		position: absolute;
		left: 12px;
		right: 12px;
		bottom: 48px;
		background: rgba(0, 0, 0, 0.45);
	}

	.self-full__end {
		position: absolute;
		left: 50%;
		transform: translateX(-50%);
		bottom: 16px;
		color: rgba(255, 255, 255, 0.75);
	}

	.grid {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: 10px;
	}

	/* Desktop: as many tiles per row as fit, each capped so a lone tile doesn't fill the window. */
	@media (min-width: 768px) {
		.grid {
			grid-template-columns: repeat(auto-fill, minmax(220px, 320px));
			justify-content: center;
			gap: 16px;
		}
	}

	.bottom-bar {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px;
		border-radius: 999px;
		background: var(--bg-elevated);
		min-width: 0;
		overflow: hidden;
	}

	.ended {
		margin: auto 0;
		gap: 16px;
	}

	.ended p {
		margin: 0;
		max-width: 280px;
	}

	.notice {
		margin: 10px 0 0;
		padding: 10px 12px;
		border-radius: var(--radius-sm);
		background: var(--bg-elevated);
		font-size: 13px;
	}

	.more {
		margin-top: 12px;
		font-size: 12px;
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
		cursor: pointer;
	}

	.admin-btn {
		border-color: #7dd3fc;
	}

	.link {
		align-self: center;
		margin-top: 10px;
		padding: 0;
		border: none;
		background: none;
		color: var(--text-muted);
		font-size: 12px;
		text-decoration: underline;
		cursor: pointer;
	}

	.editor-sheet {
		position: fixed;
		inset: 0;
		z-index: 30;
		overflow-y: auto;
		background: rgba(0, 0, 0, 0.5);
	}

	.icon-btn--active {
		background: #7dd3fc;
		color: #1c1d21;
	}
</style>
