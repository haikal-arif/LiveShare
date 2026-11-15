package domain

import "errors"

// RoomExistsError is returned when a room is already open on the server.
// RoomID is the open room, so the client can join it instead.
type RoomExistsError struct {
	RoomID string
}

func (e *RoomExistsError) Error() string {
	return "a room is already open on this server"
}

var (
	ErrRoomNotFound       = errors.New("room not found")
	ErrAdminTokenRequired = errors.New("admin token required")
	ErrNotAdmin           = errors.New("admin token does not match this room")
	ErrSFUUnreachable     = errors.New("sfu unreachable")
	ErrInvalidPasscode    = errors.New("passcode is missing or wrong")
	ErrIPMismatch         = errors.New("request does not come from the room's creator IP")

	// Streamer registration (PUT /api/rooms/:room_id/streamers/:streamer_id).
	ErrInvalidDisplayName = errors.New("display_name must be 1-40 characters after trimming whitespace")
	ErrPeerNotLive        = errors.New("peer_id is not a live publisher in this room")

	// Stage layout (PUT /api/rooms/:room_id/stage).
	ErrInvalidTemplate           = errors.New("template is not one of the known venue templates")
	ErrInvalidPosition           = errors.New("placement x/y must be between 0.0 and 1.0")
	ErrUnknownStreamer           = errors.New("streamer_id is not registered in this room")
	ErrDuplicateStreamer         = errors.New("streamer_id is placed more than once")
	ErrPlacementsWithoutTemplate = errors.New("placements sent with a null template")

	// Blacklist.
	ErrBlacklisted    = errors.New("caller ip is blacklisted from this room")
	ErrPeerNotFound   = errors.New("peer_id is not connected to this room right now")
	ErrNotBlacklisted = errors.New("ip is not on the blacklist")
)

type RoomDetail struct {
	id string
}
