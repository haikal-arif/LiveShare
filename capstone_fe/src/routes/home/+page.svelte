<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon from '$lib/icons/Icon.svelte';

	let sharedRoom = $derived(page.url.searchParams.get('room'));

	function joinAsStreamer() {
		if (sharedRoom) goto(resolve('/room/[id]/join?role=publisher', { id: sharedRoom }));
		else goto(resolve('/join/[role]', { role: 'streamer' }));
	}

	function watchLiveStreaming() {
		if (sharedRoom) goto(resolve('/room/[id]?role=subscriber', { id: sharedRoom }));
		else goto(resolve('/join/[role]', { role: 'watch' }));
	}
</script>

<Screen>
	<div class="screen__body">
		<div class="center-col" style="margin-top: 48px; margin-bottom: 40px;">
			<div class="brand-mark">
				<Icon name="camera" size={30} />
			</div>
			<h1 style="margin: 14px 0 0; font-size: 20px;">FlashLive</h1>
		</div>

		<div class="stack">
			{#if sharedRoom}
				<p class="muted" style="margin: 0 0 4px; font-size: 13px;">
					Joining shared room <strong>{sharedRoom}</strong> — creating a new room is disabled while you
					have an invite open.
				</p>
			{/if}
			<Button
				variant="outline"
				icon="grid"
				disabled={!!sharedRoom}
				onclick={() => goto(resolve('/create'))}
			>
				Create Room
			</Button>
			<Button variant="outline" icon="login" onclick={joinAsStreamer}>Join as Streamer</Button>
			<Button variant="outline" icon="cast" onclick={watchLiveStreaming}>
				Watch Live Streaming
			</Button>
		</div>

		<div style="flex: 1;"></div>

		<div
			class="muted"
			style="display: flex; justify-content: center; gap: 18px; font-size: 13px; padding-top: 24px;"
		>
			<span>About FlashLive</span>
			<span style="display: inline-flex; align-items: center; gap: 4px;">
				<Icon name="globe" size={14} /> English <Icon name="chevron-up" size={12} />
			</span>
		</div>
	</div>
</Screen>
