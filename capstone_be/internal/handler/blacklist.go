package handler

import (
	"net/http"

	"capstonewebserver/internal/dto"

	"github.com/gin-gonic/gin"
)

var ipParam = "ip"

// AddBlacklist handles POST /api/blacklist (admin).
func (h WebServerHandler) AddBlacklist(c *gin.Context) {
	var req dto.BlacklistAddRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody("invalid_request", err.Error()))
		return
	}

	res, err := h.buc.AddBlacklist(c.Request.Context(), req)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

// ListBlacklist handles GET /api/blacklist (admin).
func (h WebServerHandler) ListBlacklist(c *gin.Context) {
	res, err := h.buc.ListBlacklist(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

// RemoveBlacklist handles DELETE /api/blacklist/:ip (admin).
func (h WebServerHandler) RemoveBlacklist(c *gin.Context) {
	ip := c.Param(ipParam)

	if err := h.buc.RemoveBlacklist(c.Request.Context(), ip); err != nil {
		respondError(c, err)
		return
	}

	c.AbortWithStatus(http.StatusNoContent)
}
