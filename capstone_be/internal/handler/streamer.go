package handler

import (
	"net/http"

	"capstonewebserver/internal/dto"

	"github.com/gin-gonic/gin"
)

var streamerIdParam = "streamer_id"

// RegisterStreamer handles PUT /api/rooms/:room_id/streamers/:streamer_id.
func (h WebServerHandler) RegisterStreamer(c *gin.Context) {
	roomId := c.Param(roomIdParam)
	streamerId := c.Param(streamerIdParam)

	var req dto.StreamerRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody("invalid_request", err.Error()))
		return
	}

	res, err := h.suc.RegisterStreamer(c.Request.Context(), roomId, streamerId, req)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}
