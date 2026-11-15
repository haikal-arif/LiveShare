<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon, { type IconName } from '../icons/Icon.svelte';

	let {
		variant = 'outline',
		icon,
		disabled = false,
		type = 'button',
		onclick,
		children
	}: {
		variant?: 'outline' | 'filled' | 'live' | 'pill' | 'ghost';
		icon?: IconName;
		disabled?: boolean;
		type?: 'button' | 'submit';
		onclick?: () => void;
		children: Snippet;
	} = $props();

	const variantClass: Record<string, string> = {
		outline: '',
		filled: 'btn--filled',
		live: 'btn--filled btn--live',
		pill: 'btn--pill',
		ghost: 'btn--ghost btn--pill'
	};
</script>

<button class="btn {variantClass[variant]}" {type} {disabled} {onclick}>
	{#if icon}<Icon name={icon} size={18} />{/if}
	{@render children()}
</button>
