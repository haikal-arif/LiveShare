import { page } from 'vitest/browser';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import VideoTile from './VideoTile.svelte';

// Any MediaStream works; the tile only hands it to the <video> element.
const stream = () => new MediaStream();

describe('VideoTile quality picker', () => {
	it('shows the forwarded layer, tagged auto after a downgrade, and reports a pick', async () => {
		const onPickLayer = vi.fn();
		render(VideoTile, {
			stream: stream(),
			label: 'User pub-1',
			quality: { picked: 'high', layer: 'mid', quality: 'poor' },
			onPickLayer
		});

		const picker = page.getByRole('combobox', { name: 'Stream quality' });
		await expect.element(picker).toHaveValue('mid');
		await expect.element(page.getByRole('option', { name: 'Mid (auto)' })).toBeInTheDocument();
		await expect.element(picker).toHaveAttribute('title', expect.stringContaining('retry'));

		await picker.selectOptions('high');
		expect(onPickLayer).toHaveBeenCalledExactlyOnceWith('high');
	});

	it('has no picker for the local preview', async () => {
		render(VideoTile, { stream: stream(), label: 'You', muted: true });
		await expect.element(page.getByText('You')).toBeInTheDocument();
		expect(page.getByRole('combobox').elements()).toHaveLength(0);
	});
});
