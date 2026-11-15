export type FacingMode = 'user' | 'environment';

/**
 * True on phones: a touch screen with no hover, which tells them apart from
 * a laptop or desktop, even one with a touchscreen.
 */
export function isPhone(): boolean {
	return matchMedia('(pointer: coarse) and (hover: none)').matches;
}

/** Which camera a video track came from, or `fallback` when the browser doesn't say. */
export function facingModeOf(track: MediaStreamTrack, fallback: FacingMode): FacingMode {
	const mode = track.getSettings().facingMode;
	return mode === 'user' || mode === 'environment' ? mode : fallback;
}
