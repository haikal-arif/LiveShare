<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Screen from '$lib/components/Screen.svelte';
	import Button from '$lib/components/Button.svelte';
	import Icon from '$lib/icons/Icon.svelte';
	import { normalizeRoomCode } from '$lib/roomcode';

	let role = $derived(page.params.role === 'watch' ? 'watch' : 'streamer');
	let title = $derived(role === 'watch' ? 'Join Live Stream' : 'Join as Streamer');
	let roomInput = $state('');

	function goToRoom(roomId: string) {
		const id = normalizeRoomCode(roomId);
		if (!id) return;
		if (role === 'watch') goto(resolve('/room/[id]?role=subscriber', { id }));
		else goto(resolve('/room/[id]/join?role=publisher', { id }));
	}

	function onSubmit(e: SubmitEvent) {
		e.preventDefault();
		goToRoom(roomInput);
	}
</script>

<Screen theme="dark">
	<div class="screen__topbar">
		<button class="icon-btn" onclick={() => goto(resolve('/home'))} aria-label="Back">
			<Icon name="arrow-left" size={18} />
		</button>
	</div>

	<div class="screen__body">
		<h1 class="center-col" style="font-size: 20px; margin: 12px 0 28px;">{title}</h1>

		<form class="stack" onsubmit={onSubmit}>
			<span class="field-label">Enter ID Room</span>
			<input class="field" placeholder="ABC 123" bind:value={roomInput} />
			<Button variant="pill" type="submit">Join</Button>
		</form>

		<div style="margin: 28px 0;" class="divider-label">Or Scan QR</div>

		<button
			class="card"
			style="border-style: dashed; padding: 24px; cursor: pointer; width: 100%; background: transparent;"
			onclick={() => goto(resolve('/scan/[role]', { role }))}
		>
			<div class="center-col" style="gap: 8px;">
				<Icon name="qr" size={40} />
			</div>
		</button>
		<div style="margin-top: 12px;">
			<Button variant="pill" onclick={() => goto(resolve('/scan/[role]', { role }))}>
				Scan QR
			</Button>
		</div>
	</div>
</Screen>
