<script lang="ts">
	import { onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Icon from '$lib/icons/Icon.svelte';
	import { extractRoomIdFromScan } from '$lib/qr';
	import { normalizeRoomCode } from '$lib/roomcode';

	let role = $derived(page.params.role === 'watch' ? 'watch' : 'streamer');
	let videoEl: HTMLVideoElement | undefined = $state();
	let errorMessage = $state<string | null>(null);
	let scanner: import('qr-scanner').default | null = null;

	$effect(() => {
		if (!videoEl) return;
		let cancelled = false;

		void import('qr-scanner').then(({ default: QrScanner }) => {
			if (cancelled || !videoEl) return;
			scanner = new QrScanner(
				videoEl,
				(result) => {
					const roomId = normalizeRoomCode(extractRoomIdFromScan(result.data));
					if (!roomId) return;
					scanner?.stop();
					if (role === 'watch') goto(resolve('/room/[id]?role=subscriber', { id: roomId }));
					else goto(resolve('/room/[id]/join?role=publisher', { id: roomId }));
				},
				{ highlightScanRegion: true, highlightCodeOutline: true }
			);
			scanner.start().catch(() => {
				errorMessage = 'Camera access is required to scan a QR code.';
			});
		});

		return () => {
			cancelled = true;
			scanner?.stop();
			scanner?.destroy();
			scanner = null;
		};
	});

	onDestroy(() => {
		scanner?.stop();
		scanner?.destroy();
	});
</script>

<Screen theme="dark">
	<div class="screen__topbar">
		<button
			class="icon-btn"
			onclick={() => goto(resolve('/join/[role]', { role }))}
			aria-label="Back"
		>
			<Icon name="arrow-left" size={18} />
		</button>
	</div>

	<div class="screen__body" style="padding: 0; position: relative; flex: 1;">
		<video bind:this={videoEl} class="scan-video"></video>

		<div class="scan-frame">
			<span class="corner corner--tl"></span>
			<span class="corner corner--tr"></span>
			<span class="corner corner--bl"></span>
			<span class="corner corner--br"></span>
		</div>

		{#if errorMessage}
			<p
				class="muted center-col"
				style="position: absolute; bottom: 40px; width: 100%; padding: 0 20px;"
			>
				{errorMessage}
			</p>
		{/if}
	</div>
</Screen>

<style>
	.scan-video {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		background: #000;
	}

	.scan-frame {
		position: absolute;
		top: 50%;
		left: 50%;
		width: 220px;
		height: 220px;
		transform: translate(-50%, -50%);
		pointer-events: none;
	}

	.corner {
		position: absolute;
		width: 28px;
		height: 28px;
		border: 3px solid #fff;
	}

	.corner--tl {
		top: 0;
		left: 0;
		border-right: none;
		border-bottom: none;
	}

	.corner--tr {
		top: 0;
		right: 0;
		border-left: none;
		border-bottom: none;
	}

	.corner--bl {
		bottom: 0;
		left: 0;
		border-right: none;
		border-top: none;
	}

	.corner--br {
		bottom: 0;
		right: 0;
		border-left: none;
		border-top: none;
	}
</style>
