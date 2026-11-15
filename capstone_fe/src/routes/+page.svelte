<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import Screen from '$lib/components/Screen.svelte';
	import Icon from '$lib/icons/Icon.svelte';

	function toHome() {
		goto(resolve('/home'));
	}

	$effect(() => {
		const timer = setTimeout(toHome, 1400);
		return () => clearTimeout(timer);
	});
</script>

<Screen>
	<div
		class="screen__body center-col"
		style="justify-content: center; flex: 1; cursor: pointer;"
		onclick={toHome}
		role="button"
		tabindex="0"
		onkeydown={(e) => e.key === 'Enter' && toHome()}
	>
		<div class="brand-mark" style="width: 88px; height: 88px;">
			<Icon name="camera" size={36} />
		</div>
		<h1 style="margin: 16px 0 4px; font-size: 22px;">FlashLive</h1>
		<p class="muted" style="margin: 0; font-size: 14px;">Live Streams, Fast via QR</p>
	</div>
</Screen>
