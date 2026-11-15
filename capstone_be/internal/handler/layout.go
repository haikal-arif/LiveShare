package handler

import (
	"net/http"

	"capstonewebserver/internal/dto"

	"github.com/gin-gonic/gin"
)

// GetStage handles GET /api/rooms/:room_id/stage.
func (h WebServerHandler) GetStage(c *gin.Context) {
	roomId := c.Param(roomIdParam)

	res, err := h.luc.GetStage(c.Request.Context(), roomId)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

// PutStage handles PUT /api/rooms/:room_id/stage (admin).
func (h WebServerHandler) PutStage(c *gin.Context) {
	roomId := c.Param(roomIdParam)

	var req dto.RoomLayoutEditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody("invalid_request", err.Error()))
		return
	}

	res, err := h.luc.PutStage(c.Request.Context(), roomId, req)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}
