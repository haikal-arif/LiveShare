package domain

import "capstonewebserver/internal/dto"

// RoomLayout is the stage as stored: template, placements and version.
type RoomLayout struct {
	Version    int64
	Placements []dto.Placement
	Template   *string
}
