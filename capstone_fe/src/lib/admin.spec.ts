import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const reclaimAdmin = vi.fn();
vi.mock('./api', () => ({ reclaimAdmin: (id: string) => reclaimAdmin(id) }));

const { findAdminKey, getAdminKey, saveAdminKey, streamerId } = await import('./admin');

describe('admin keys', () => {
	beforeEach(() => {
		const store = new Map<string, string>();
		vi.stubGlobal('localStorage', {
			getItem: (k: string) => store.get(k) ?? null,
			setItem: (k: string, v: string) => store.set(k, v),
			removeItem: (k: string) => store.delete(k)
		});
		reclaimAdmin.mockReset();
	});
	afterEach(() => vi.unstubAllGlobals());

	it('uses the stored key without asking the server', async () => {
		saveAdminKey('ABC-123', 'k_1');
		expect(await findAdminKey('ABC-123')).toBe('k_1');
		expect(reclaimAdmin).not.toHaveBeenCalled();
	});

	it('reclaims and remembers the key from the creating IP', async () => {
		reclaimAdmin.mockResolvedValue({ room_id: 'ABC-123', admin_token: 'k_2' });
		expect(await findAdminKey('ABC-123')).toBe('k_2');
		expect(getAdminKey('ABC-123')).toBe('k_2');
	});

	it('is null when this device is not the admin', async () => {
		reclaimAdmin.mockRejectedValue(new Error('ip_mismatch'));
		expect(await findAdminKey('ABC-123')).toBeNull();
	});

	it('keeps one streamer id per browser', () => {
		const first = streamerId();
		expect(first).toMatch(/^[0-9a-f-]{36}$/);
		expect(streamerId()).toBe(first);
	});
});
