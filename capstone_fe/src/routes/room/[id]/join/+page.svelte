<script lang="ts">
	import { onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon from '$lib/icons/Icon.svelte';
	import { roomSession } from '$lib/room.svelte';

	let roomId = $derived(page.params.id as string);
	let videoEl: HTMLVideoElement | undefined = $state();
	let name = $state('');
	let cameraError = $state<string | null>(null);
	let starting = $state(false);
	let localStream: MediaStream | null = $state(null);

	$effect(() => {
		let cancelled = false;
		navigator.mediaDevices
			// Browsers only send all three simulcast layers from ~720p up.
			.getUserMedia({ video: { width: { ideal: 1280 }, height: { ideal: 720 } }, audio: true })
			.then((stream) => {
				if (cancelled) {
					stream.getTracks().forEach((t) => t.stop());
					return;
				}
				localStream = stream;
				if (videoEl) videoEl.srcObject = stream;
			})
			.catch(() => {
				cameraError = 'Camera access is required to stream. Please allow camera permissions.';
			});

		return () => {
			cancelled = true;
		};
	});

	onDestroy(() => {
		if (roomSession.status !== 'connected') {
			localStream?.getTracks().forEach((t) => t.stop());
		}
	});

	async function startLiveStreaming() {
		if (!localStream) return;
		starting = true;
		roomSession.setLocalPreview(localStream, name.trim() || 'You');
		try {
			await roomSession.startPublishing(roomId);
			goto(resolve('/room/[id]?role=publisher', { id: roomId }));
		} catch {
			starting = false;
		}
	}
</script>

<Screen theme="dark">
	<div class="screen__topbar" style="justify-content: space-between;">
		<div style="display: flex; align-items: center; gap: 10px;">
			<Icon name="camera" size={18} />
			<div>
				<div class="screen__topbar-title">Live Room</div>
				<div class="muted" style="font-size: 12px;">ID Room : {roomId}</div>
			</div>
		</div>
		<span class="icon-btn"><Icon name="mic" size={16} /></span>
	</div>

	<div class="screen__body" style="justify-content: center; gap: 24px;">
		<div class="center-col">
			<div class="preview-circle">
				{#if localStream}
					<video bind:this={videoEl} autoplay playsinline muted></video>
				{:else}
					<Icon name="user" size={40} />
				{/if}
			</div>
			{#if cameraError}
				<p class="muted" style="font-size: 13px; margin-top: 12px; max-width: 260px;">
					{cameraError}
				</p>
			{/if}
			{#if roomSession.errorMessage}
				<p style="color: var(--live); font-size: 13px; margin-top: 12px;">
					{roomSession.errorMessage}
				</p>
			{/if}
		</div>

		<input class="field" placeholder="Fill your name…" bind:value={name} />

		<Button
			variant="filled"
			icon="video"
			disabled={!localStream || starting}
			onclick={startLiveStreaming}
		>
			{starting ? 'Connecting…' : 'Start Live Streaming'}
		</Button>
	</div>
</Screen>

<style>
	.preview-circle {
		width: 140px;
		height: 140px;
		border-radius: 999px;
		overflow: hidden;
		background: rgba(255, 255, 255, 0.08);
		display: flex;
		align-items: center;
		justify-content: center;
		color: rgba(255, 255, 255, 0.6);
	}

	.preview-circle video {
		width: 100%;
		height: 100%;
		object-fit: cover;
		transform: scaleX(-1);
	}
</style>
