package dto

// POST /api/rooms (request). Creating a room is gated by the server's
// passcode (set via .env).
type RoomCreationRequest struct {
	Passcode string `json:"passcode"`
}

// POST /api/rooms (201)
type RoomCreationResponse struct {
	RoomID     string `json:"room_id"`
	AdminToken string `json:"admin_token"`
}

// GET /api/rooms/:room_id (200). Publishers must be non-nil so an empty room
// serializes as [] rather than null.
type RoomQueryResponse struct {
	RoomID     string      `json:"room_id"`
	Publishers []Publisher `json:"publishers"`
	SfuWsURL   string      `json:"sfu_ws_url"`
	ICEServers []ICEServer `json:"ice_servers"`
}

// GET /api/rooms/:room_id/publishers (200)
type RoomPublisherQueryResponse struct {
	RoomID     string      `json:"room_id"`
	Publishers []Publisher `json:"publishers"`
}

type Publisher struct {
	PeerID string `json:"peer_id"`
}

type ICEServer struct {
	URLs []string `json:"urls"`
	// Only for TURN: the long-term username and password.
	Username   string `json:"username,omitempty"`
	Credential string `json:"credential,omitempty"`
}

// POST /api/rooms/:room_id/admin/reclaim (200). The request has no body.
type RoomReclaimRequest struct {
}

type RoomReclaimResponse struct {
	RoomID     string `json:"room_id"`
	AdminToken string `json:"admin_token"`
}

// PUT /api/rooms/:room_id/streamers/:streamer_id (200)
type StreamerRegistrationRequest struct {
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name"`
}

type StreamerRegistrationResponse = Streamer

// Streamer is one entry in the registry. It appears in the registration
// response and in the stage's streamers list.
type Streamer struct {
	StreamerID  string `json:"streamer_id"`
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name"`
	Live        bool   `json:"live"`
}

// GET /api/rooms/:room_id/stage (200). PUT /api/rooms/:room_id/stage returns
// the same body.
type RoomLayoutQueryResponse struct {
	RoomID     string      `json:"room_id"`
	Version    int64       `json:"version"`
	Template   *string     `json:"template"`
	Placements []Placement `json:"placements"`
	Streamers  []Streamer  `json:"streamers"`
}

// PUT /api/rooms/:room_id/stage (admin). A null template with empty
// placements removes the stage.
type RoomLayoutEditRequest struct {
	Template   *string     `json:"template"`
	Placements []Placement `json:"placements"`
}

// X and Y are fractions of the template's width and height, from 0.0 to 1.0.
type Placement struct {
	StreamerID string  `json:"streamer_id"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
}

// POST /api/blacklist (request). The admin names the peer by its current
// peer_id; Go resolves it to an IP through the SFU.
type BlacklistAddRequest struct {
	PeerID string `json:"peer_id"`
}

// POST /api/blacklist (200)
type BlacklistAddResponse struct {
	RoomID string `json:"room_id"`
	IP     string `json:"ip"`
	PeerID string `json:"peer_id"`
}

// One entry in GET /api/blacklist's list.
type BlacklistEntry struct {
	IP     string `json:"ip"`
	PeerID string `json:"peer_id"`
}

// GET /api/blacklist (200). Blacklist must be non-nil so an empty list
// serializes as [] rather than null.
type BlacklistListResponse struct {
	RoomID    string           `json:"room_id"`
	Blacklist []BlacklistEntry `json:"blacklist"`
}

// room_closed event data on GET /api/rooms/:room_id/events. A stage_updated
// event carries the same body as GET /api/rooms/:room_id/stage
// (RoomLayoutQueryResponse).
type RoomClosedEvent struct {
	RoomID string `json:"room_id"`
}
