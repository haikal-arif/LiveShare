package dto

// ErrorResponse is the body for every error. RoomID is only set for
// room_exists (409), so the frontend knows which room to join instead.
type ErrorResponse struct {
	Error  ErrorDetail `json:"error"`
	RoomID string      `json:"room_id,omitempty"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
