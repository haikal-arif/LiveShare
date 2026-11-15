import { page } from 'vitest/browser';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import StageEditor from './StageEditor.svelte';
import type { Stage } from '../contract';

const stage: Stage = {
	room_id: 'ABC-123',
	version: 1,
	template: null,
	placements: [],
	streamers: [{ streamer_id: 's1', peer_id: 'pub-1', display_name: 'Hana A', live: true }]
};

describe('StageEditor', () => {
	afterEach(() => vi.unstubAllGlobals());

	it('picks a template, places a streamer by tap, and saves the layout', async () => {
		const saved = { ...stage, version: 2 };
		const fetchMock = vi.fn(
			async () => ({ ok: true, status: 200, json: async () => saved }) as Response
		);
		vi.stubGlobal('fetch', fetchMock);
		const onSaved = vi.fn();
		const screen = render(StageEditor, {
			roomId: 'ABC-123',
			adminKey: 'k_secret',
			stage,
			onSaved,
			onCancel: () => {}
		});

		await page.getByRole('button', { name: /Theater/ }).click();
		await page.getByRole('button', { name: 'Next' }).click();
		await expect.element(page.getByText('Not on stage yet (1)')).toBeInTheDocument();

		// Tap the chip, then tap the middle of the stage.
		await page.getByRole('button', { name: 'Hana A' }).click();
		const map = screen.container.querySelector('.stage-map') as HTMLElement;
		const rect = map.getBoundingClientRect();
		map.dispatchEvent(
			new MouseEvent('click', {
				bubbles: true,
				clientX: rect.left + rect.width / 2,
				clientY: rect.top + rect.height / 2
			})
		);
		await expect.element(page.getByText('Not on stage yet (0)')).toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Watch Hana A' })).toBeInTheDocument();

		await page.getByRole('button', { name: 'Save Layout' }).click();
		await vi.waitFor(() => expect(onSaved).toHaveBeenCalledWith(saved));
		const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
		expect(url).toMatch(/\/api\/rooms\/ABC-123\/stage$/);
		expect(init.headers).toMatchObject({ 'X-Admin-Token': 'k_secret' });
		expect(JSON.parse(init.body as string)).toEqual({
			template: 'theater',
			placements: [{ streamer_id: 's1', x: 0.5, y: 0.5 }]
		});
	});
});
