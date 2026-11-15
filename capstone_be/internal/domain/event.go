package domain

// Names of the events pushed on GET /api/rooms/:room_id/events
// (contracts.md "events").
const (
	EventStageUpdated = "stage_updated"
	EventRoomClosed   = "room_closed"
)

// Event is one message on a room's events stream. Data is serialized to
// JSON as the event's data field: a dto.RoomLayoutQueryResponse for
// stage_updated, a dto.RoomClosedEvent for room_closed.
type Event struct {
	Name string
	Data any
}
