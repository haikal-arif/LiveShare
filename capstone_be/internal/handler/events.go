package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"capstonewebserver/internal/domain"

	"github.com/gin-gonic/gin"
)

// keepaliveInterval is how often an idle stream gets a comment line, so
// proxies don't close it (contracts.md: "every 25 s").
var keepaliveInterval = 25 * time.Second

// Events handles GET /api/rooms/:room_id/events: a Server-Sent Events stream
// that starts with the current stage and then carries stage_updated and
// room_closed until the client leaves or the room closes.
func (h WebServerHandler) Events(c *gin.Context) {
	roomId := c.Param(roomIdParam)

	initial, events, cancel, err := h.euc.Subscribe(c.Request.Context(), roomId)
	if err != nil {
		respondError(c, err)
		return
	}
	defer cancel()

	header := c.Writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	// nginx would otherwise buffer the stream and hold events back.
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	if err := writeEvent(c, domain.Event{Name: domain.EventStageUpdated, Data: initial}); err != nil {
		return
	}

	keepalive := time.NewTicker(keepaliveInterval)
	defer keepalive.Stop()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if err := writeEvent(c, ev); err != nil {
				return
			}
			// The server ends the stream after room_closed.
			if ev.Name == domain.EventRoomClosed {
				return
			}
		case <-keepalive.C:
			if _, err := fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

// writeEvent writes ev in the text/event-stream format and flushes it.
func writeEvent(c *gin.Context, ev domain.Event) error {
	data, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", ev.Name, data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}
