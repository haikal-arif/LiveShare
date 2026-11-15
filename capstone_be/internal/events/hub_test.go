package events_test

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/events"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func stageEvent(n int) domain.Event {
	return domain.Event{Name: domain.EventStageUpdated, Data: n}
}

func TestHub_Publish(t *testing.T) {
	tests := []struct {
		name        string
		run         func(t *testing.T, hub *events.Hub)
		checkResult func(t *testing.T, hub *events.Hub)
	}{
		{
			"every stream in the room gets the event",
			func(t *testing.T, hub *events.Hub) {},
			func(t *testing.T, hub *events.Hub) {
				a, cancelA := hub.Subscribe(t.Context(), "ABC-123")
				defer cancelA()
				b, cancelB := hub.Subscribe(t.Context(), "ABC-123")
				defer cancelB()

				hub.Publish(t.Context(), "ABC-123", stageEvent(1))

				assert.Equal(t, stageEvent(1), <-a)
				assert.Equal(t, stageEvent(1), <-b)
			},
		},
		{
			"streams in other rooms don't get it",
			func(t *testing.T, hub *events.Hub) {},
			func(t *testing.T, hub *events.Hub) {
				other, cancel := hub.Subscribe(t.Context(), "ZZZ-999")
				defer cancel()

				hub.Publish(t.Context(), "ABC-123", stageEvent(1))

				assert.Empty(t, other)
			},
		},
		{
			"a cancelled stream is closed and stops receiving",
			func(t *testing.T, hub *events.Hub) {},
			func(t *testing.T, hub *events.Hub) {
				ch, cancel := hub.Subscribe(t.Context(), "ABC-123")
				cancel()
				cancel() // safe to repeat

				hub.Publish(t.Context(), "ABC-123", stageEvent(1))

				_, open := <-ch
				assert.False(t, open)
				assert.Equal(t, 0, hub.Subscribers(t.Context(), "ABC-123"))
			},
		},
		{
			"a stream that falls behind loses its oldest events, never the newest",
			func(t *testing.T, hub *events.Hub) {},
			func(t *testing.T, hub *events.Hub) {
				ch, cancel := hub.Subscribe(t.Context(), "ABC-123")
				defer cancel()

				for i := 0; i < 100; i++ {
					hub.Publish(t.Context(), "ABC-123", stageEvent(i))
				}
				hub.Publish(t.Context(), "ABC-123", domain.Event{Name: domain.EventRoomClosed})

				var last domain.Event
				for len(ch) > 0 {
					last = <-ch
				}
				assert.Equal(t, domain.EventRoomClosed, last.Name)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := events.NewHub()
			tt.run(t, hub)
			tt.checkResult(t, hub)
		})
	}
}

func TestHub_Subscribers(t *testing.T) {
	hub := events.NewHub()
	require.Equal(t, 0, hub.Subscribers(t.Context(), "ABC-123"))

	_, cancelA := hub.Subscribe(t.Context(), "ABC-123")
	_, cancelB := hub.Subscribe(t.Context(), "ABC-123")
	assert.Equal(t, 2, hub.Subscribers(t.Context(), "ABC-123"))

	cancelA()
	assert.Equal(t, 1, hub.Subscribers(t.Context(), "ABC-123"))
	cancelB()
	assert.Equal(t, 0, hub.Subscribers(t.Context(), "ABC-123"))
}
