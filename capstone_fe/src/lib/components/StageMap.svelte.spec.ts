import { page } from 'vitest/browser';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import StageMap from './StageMap.svelte';

const pins = [
	{ streamerId: 's1', peerId: 'pub-1', name: 'Hana A', live: true, x: 0.3, y: 0.23 },
	{ streamerId: 's2', peerId: 'pub-2', name: 'User C', live: false, x: 0.75, y: 0.45 }
];

describe('StageMap', () => {
	it('puts pins at their stage position and reports taps', async () => {
		const onPinClick = vi.fn();
		render(StageMap, { template: 'concert', pins, onPinClick });

		const hana = page.getByRole('button', { name: 'Watch Hana A' });
		await expect.element(hana).toHaveStyle({ left: '30%', top: '23%' });
		await expect
			.element(page.getByRole('button', { name: 'User C (offline)' }))
			.toBeInTheDocument();

		await hana.click();
		expect(onPinClick).toHaveBeenCalledWith(pins[0]);
	});

	it('reports a tap on an empty spot as a 0-1 position', async () => {
		const onMapClick = vi.fn();
		const screen = render(StageMap, { template: 'blank', pins: [], onMapClick });
		const map = screen.container.querySelector('.stage-map') as HTMLElement;
		const rect = map.getBoundingClientRect();
		map.dispatchEvent(
			new MouseEvent('click', {
				bubbles: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.top + rect.height / 4
			})
		);
		expect(onMapClick).toHaveBeenCalledWith(0.5, 0.25);
	});
});
