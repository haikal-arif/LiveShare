/**
 * Types mirroring capstone-docs/API-Contract.md. Kept in one place so the
 * REST client, signaling client, and webrtc helpers all agree on the wire
 * shapes.
 */

export type PeerId = string;
export type RoomId = string;

export interface IceServer {
	urls: string[];
	/** TURN only: long-term username and password. */
	username?: string;
	credential?: string;
}

export interface PublisherRef {
	peer_id: PeerId;
}

export interface RoomDetails {
	room_id: RoomId;
	publishers: PublisherRef[];
	sfu_ws_url: string;
	ice_servers: IceServer[];
}

export interface RoomPublishers {
	room_id: RoomId;
	publishers: PublisherRef[];
}

export interface ApiError {
	error: {
		code: string;
		message: string;
	};
	/** Only on 409 room_exists: the room that's already open. */
	room_id?: RoomId;
}

export interface CreatedRoom {
	room_id: RoomId;
	admin_token: string;
}

/** A publisher with a stable identity; peer_id changes on every reconnect. */
export interface Streamer {
	streamer_id: string;
	peer_id: PeerId;
	display_name: string;
	live: boolean;
}

export type StageTemplate = 'concert' | 'conference' | 'theater' | 'blank';

/** A streamer's spot on the stage, as 0-1 fractions of its width/height. */
export interface Placement {
	streamer_id: string;
	x: number;
	y: number;
}

export interface Stage {
	room_id: RoomId;
	version: number;
	/** null = no stage set, watchers get the plain grid. */
	template: StageTemplate | null;
	placements: Placement[];
	streamers: Streamer[];
}

// ---- WebSocket signaling (browser/mock-client <-> SFU) ----

/** How well a subscriber<->publisher pairing's connection is keeping up. */
export type ConnectionQuality = 'good' | 'poor' | 'bad';
/** The publisher simulcast layer the SFU is forwarding (rids h / m / l). */
export type SimulcastLayer = 'high' | 'mid' | 'low';

export interface ConnectionStats {
	target_peer: PeerId;
	/** Optional in the contract; this client never sends it (see connection-stats.ts). */
	available_incoming_bitrate_bps?: number;
	packet_loss_pct: number;
	round_trip_time_ms?: number;
	jitter_ms?: number;
}

/**
 * Machine-readable reason on a WebSocket `error` message. Treat a code you
 * don't know as a generic error; `reason` is the human-readable text.
 */
export type ErrorCode =
	| 'invalid_message'
	| 'not_joined'
	| 'already_joined'
	| 'peer_not_found'
	| 'wrong_role'
	| 'invalid_sdp'
	| 'invalid_state'
	| 'internal';

export type ClientMessage =
	| { type: 'join_as_publisher'; room: RoomId }
	| { type: 'join_as_subscriber'; room: RoomId }
	| { type: 'sdp_offer'; target_peer: PeerId | null; sdp: string }
	| { type: 'sdp_answer'; target_peer: PeerId; sdp: string }
	| { type: 'ice_candidate'; target_peer: PeerId | null; candidate: string }
	| { type: 'request_track'; target_peer: PeerId }
	| { type: 'set_preferred_layer'; target_peer: PeerId; layer: SimulcastLayer }
	| ({ type: 'connection_stats' } & ConnectionStats)
	| { type: 'leave' };

export type ServerMessage =
	| { type: 'joined'; peer_id: PeerId; ice_servers: IceServer[]; publishers: PeerId[] }
	| { type: 'publisher_joined'; peer_id: PeerId }
	| { type: 'publisher_left'; peer_id: PeerId }
	| { type: 'sdp_offer'; from_peer: PeerId | null; sdp: string }
	| { type: 'sdp_answer'; from_peer: PeerId | null; sdp: string }
	| { type: 'ice_candidate'; from_peer: PeerId | null; candidate: string }
	| {
			type: 'connection_quality';
			from_peer: PeerId;
			quality: ConnectionQuality;
			layer: SimulcastLayer;
	  }
	| { type: 'room_closed' }
	| { type: 'error'; code: ErrorCode; reason: string };
