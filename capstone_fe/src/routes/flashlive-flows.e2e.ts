import { expect, test } from '@playwright/test';

test('home shows the three entry points', async ({ page }) => {
	await page.goto('/home');
	await expect(page.getByRole('button', { name: 'Create Room' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Join as Streamer' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Watch Live Streaming' })).toBeVisible();
});

test('splash screen advances to home', async ({ page }) => {
	await page.goto('/');
	await page.getByText('FlashLive').click();
	await expect(page).toHaveURL(/\/home$/);
});

test('landing with a shared room disables Create Room', async ({ page }) => {
	await page.goto('/home?room=ABC-123');
	await expect(page.getByRole('button', { name: 'Create Room' })).toBeDisabled();
	await expect(page.getByText('ABC-123')).toBeVisible();
});

// capstone-be isn't running here, so POST /api/rooms is answered in-page.
test('Create Room asks the backend for a room and shows its code and QR', async ({ page }) => {
	await page.route('**/api/rooms', (route) =>
		route.fulfill({ status: 201, json: { room_id: 'ABC-234', admin_token: 'k_test' } })
	);
	await page.goto('/home');
	await page.getByRole('button', { name: 'Create Room' }).click();
	await page.getByPlaceholder('Passcode').fill('s3cret');
	await page.getByRole('button', { name: 'Create Room' }).click();
	await expect(page).toHaveURL(/\/create\?room=ABC-234$/);
	await expect(page.locator('canvas')).toBeVisible();
	// the room code button doubles as the copy-link control, labeled with the room code
	await expect(page.getByRole('button', { name: 'ABC-234' })).toBeVisible();
	// the creator is the admin, so they can set up the stage
	await expect(page.getByRole('button', { name: 'Set Up Stage Layout' })).toBeVisible();
});

test('Create Room while another room is open offers to join that one', async ({ page }) => {
	await page.route('**/api/rooms', (route) =>
		route.fulfill({
			status: 409,
			json: {
				error: { code: 'room_exists', message: 'a room is already open' },
				room_id: 'XYZ-789'
			}
		})
	);
	await page.route('**/api/rooms/XYZ-789/admin/reclaim', (route) =>
		route.fulfill({ status: 403, json: { error: { code: 'ip_mismatch', message: 'no' } } })
	);
	await page.goto('/create');
	await page.getByPlaceholder('Passcode').fill('s3cret');
	await page.getByRole('button', { name: 'Create Room' }).click();
	await expect(page.getByText('A room is already open on this server.')).toBeVisible();
	await page.getByRole('button', { name: 'Watch Live Streaming' }).click();
	await expect(page).toHaveURL(/\/room\/XYZ-789\?role=subscriber$/);
});

test('joining as a streamer with a manual code goes to the camera preview screen', async ({
	page
}) => {
	await page.goto('/join/streamer');
	await expect(page.getByRole('heading', { name: 'Join as Streamer' })).toBeVisible();

	await page.getByPlaceholder('ABC 123').fill('abc-123');
	await page.getByRole('button', { name: 'Join' }).click();

	await expect(page).toHaveURL(/\/room\/ABC-123\/join\?role=publisher$/);
	await expect(page.getByText('ID Room : ABC-123')).toBeVisible();
});

test('watching with a manual code goes straight into the room, no camera step', async ({
	page
}) => {
	// capstone-be isn't running here; answer as it does for a never-created room.
	await page.route('**/api/rooms/XYZ-999', (route) =>
		route.fulfill({
			status: 404,
			json: { error: { code: 'room_not_found', message: 'room XYZ-999 does not exist' } }
		})
	);
	await page.goto('/join/watch');
	await expect(page.getByRole('heading', { name: 'Join Live Stream' })).toBeVisible();

	await page.getByPlaceholder('ABC 123').fill('xyz-999');
	await page.getByRole('button', { name: 'Join' }).click();

	await expect(page).toHaveURL(/\/room\/XYZ-999\?role=subscriber$/);
	await expect(page.getByText('Live Room :')).toBeVisible();
	await expect(page.getByText("This room doesn't exist")).toBeVisible();
	await expect(page.getByRole('button', { name: 'Back to home' })).toBeVisible();
});

test('scan screen requests the camera and reports when access is unavailable', async ({ page }) => {
	await page.goto('/scan/streamer');
	await expect(page.getByText('Camera access is required to scan a QR code.')).toBeVisible();

	await page.getByRole('button', { name: 'Back' }).click();
	await expect(page).toHaveURL(/\/join\/streamer$/);
});
