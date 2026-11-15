<script module lang="ts">
	export type IconName =
		| 'camera'
		| 'camera-flip'
		| 'grid'
		| 'login'
		| 'cast'
		| 'copy'
		| 'share'
		| 'mic'
		| 'mic-off'
		| 'video'
		| 'video-off'
		| 'monitor'
		| 'x'
		| 'edit'
		| 'user'
		| 'chevron-up'
		| 'chevron-down'
		| 'globe'
		| 'arrow-left'
		| 'qr'
		| 'mirror';
</script>

<script lang="ts">
	let { name, size = 20 }: { name: IconName; size?: number } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.8"
	stroke-linecap="round"
	stroke-linejoin="round"
	class="icon"
	aria-hidden="true"
>
	{#if name === 'camera'}
		<rect x="3" y="7" width="18" height="13" rx="2" />
		<path d="M8 7l1.5-2.5h5L16 7" />
		<circle cx="12" cy="13.5" r="3.5" />
	{:else if name === 'grid'}
		<rect x="3" y="3" width="7" height="7" rx="1" />
		<rect x="14" y="3" width="7" height="7" rx="1" />
		<rect x="3" y="14" width="7" height="7" rx="1" />
		<rect x="14" y="14" width="7" height="7" rx="1" />
	{:else if name === 'login'}
		<rect x="3" y="4" width="12" height="16" rx="2" />
		<line x1="21" y1="12" x2="10" y2="12" />
		<polyline points="15 8 21 12 15 16" />
	{:else if name === 'cast'}
		<rect x="2" y="4" width="20" height="13" rx="2" />
		<path d="M6 21a6 6 0 0 0-6-6" transform="translate(0 0)" />
		<path d="M2 17a5 5 0 0 1 5 5" />
		<path d="M2 21a1 1 0 0 1 1-1" />
	{:else if name === 'copy'}
		<rect x="9" y="9" width="12" height="12" rx="2" />
		<path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1" />
	{:else if name === 'share'}
		<circle cx="18" cy="5" r="2.5" />
		<circle cx="6" cy="12" r="2.5" />
		<circle cx="18" cy="19" r="2.5" />
		<line x1="8.2" y1="10.7" x2="15.8" y2="6.3" />
		<line x1="8.2" y1="13.3" x2="15.8" y2="17.7" />
	{:else if name === 'mic'}
		<rect x="9" y="2" width="6" height="12" rx="3" />
		<path d="M5 11a7 7 0 0 0 14 0" />
		<line x1="12" y1="18" x2="12" y2="22" />
		<line x1="8" y1="22" x2="16" y2="22" />
	{:else if name === 'mic-off'}
		<rect x="9" y="2" width="6" height="12" rx="3" />
		<path d="M5 11a7 7 0 0 0 14 0" />
		<line x1="12" y1="18" x2="12" y2="22" />
		<line x1="8" y1="22" x2="16" y2="22" />
		<line x1="3" y1="3" x2="21" y2="21" />
	{:else if name === 'video'}
		<rect x="2" y="6" width="14" height="12" rx="2" />
		<polygon points="16 10 22 7 22 17 16 14" />
	{:else if name === 'video-off'}
		<rect x="2" y="6" width="14" height="12" rx="2" />
		<polygon points="16 10 22 7 22 17 16 14" />
		<line x1="1" y1="2" x2="23" y2="22" />
	{:else if name === 'monitor'}
		<rect x="3" y="4" width="18" height="12" rx="2" />
		<line x1="8" y1="20" x2="16" y2="20" />
		<line x1="12" y1="16" x2="12" y2="20" />
	{:else if name === 'x'}
		<line x1="6" y1="6" x2="18" y2="18" />
		<line x1="18" y1="6" x2="6" y2="18" />
	{:else if name === 'edit'}
		<path d="M4 17.25V20h2.75L14.6 12.15l-2.75-2.75L4 17.25z" />
		<path d="M16 7.5l1.5-1.5a1.5 1.5 0 0 1 2.12 2.12L18 9.62" />
	{:else if name === 'user'}
		<circle cx="12" cy="8" r="4" />
		<path d="M4 20c0-4 3.6-6 8-6s8 2 8 6" />
	{:else if name === 'chevron-up'}
		<polyline points="6 15 12 9 18 15" />
	{:else if name === 'chevron-down'}
		<polyline points="6 9 12 15 18 9" />
	{:else if name === 'globe'}
		<circle cx="12" cy="12" r="9" />
		<ellipse cx="12" cy="12" rx="4" ry="9" />
		<line x1="3" y1="12" x2="21" y2="12" />
	{:else if name === 'arrow-left'}
		<line x1="19" y1="12" x2="5" y2="12" />
		<polyline points="11 6 5 12 11 18" />
	{:else if name === 'qr'}
		<rect x="3" y="3" width="7" height="7" rx="1" />
		<rect x="14" y="3" width="7" height="7" rx="1" />
		<rect x="3" y="14" width="7" height="7" rx="1" />
		<line x1="14" y1="14" x2="14" y2="21" />
		<line x1="21" y1="14" x2="21" y2="21" />
		<line x1="17.5" y1="14" x2="17.5" y2="17.5" />
		<line x1="14" y1="17.5" x2="21" y2="17.5" />
	{:else if name === 'camera-flip'}
		<path d="M3 12a9 9 0 0 1 15.5-6.2L21 8" />
		<polyline points="21 3 21 8 16 8" />
		<path d="M21 12a9 9 0 0 1-15.5 6.2L3 16" />
		<polyline points="3 21 3 16 8 16" />
	{:else if name === 'mirror'}
		<line x1="12" y1="3" x2="12" y2="21" stroke-dasharray="2 2.5" />
		<path d="M9 7L3 12l6 5z" />
		<path d="M15 7l6 5-6 5z" />
	{/if}
</svg>

<style>
	.icon {
		display: block;
		flex-shrink: 0;
	}
</style>
