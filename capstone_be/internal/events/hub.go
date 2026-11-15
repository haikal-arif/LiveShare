// Package events is the in-memory fan-out behind GET
// /api/rooms/:room_id/events: usecases publish, each open stream subscribes.
// It implements usecase.EventPublisher and usecase.EventSubscriber.
package events

import (
	"context"
	"sync"

	"capstonewebserver/internal/domain"
)

// bufferSize is how many events a slow stream may fall behind by before its
// oldest ones are dropped.
const bufferSize = 16

// Hub delivers events to the streams subscribed to a room.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan domain.Event]struct{} // room_id -> streams
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan domain.Event]struct{})}
}

// Subscribe opens a stream on roomId. cancel closes it; it's safe to call
// more than once.
func (h *Hub) Subscribe(ctx context.Context, roomId string) (<-chan domain.Event, func()) {
	ch := make(chan domain.Event, bufferSize)

	h.mu.Lock()
	if h.subs[roomId] == nil {
		h.subs[roomId] = make(map[chan domain.Event]struct{})
	}
	h.subs[roomId][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			delete(h.subs[roomId], ch)
			if len(h.subs[roomId]) == 0 {
				delete(h.subs, roomId)
			}
			close(ch)
		})
	}
	return ch, cancel
}

// Publish sends ev to every stream open on roomId. It never blocks: a
// stream whose buffer is full loses its oldest event. That's safe for
// stage_updated (every event replaces the whole stage) and keeps
// room_closed from being the one that's dropped.
func (h *Hub) Publish(ctx context.Context, roomId string, ev domain.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[roomId] {
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- ev
		}
	}
}

// Subscribers reports how many streams are open on roomId.
func (h *Hub) Subscribers(ctx context.Context, roomId string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[roomId])
}
