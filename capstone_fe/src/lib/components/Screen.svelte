<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		theme = 'light',
		children
	}: {
		theme?: 'light' | 'dark';
		children: Snippet;
	} = $props();
</script>

<div class="screen" class:screen--dark={theme === 'dark'}>
	{@render children()}
</div>
