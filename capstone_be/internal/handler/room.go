package handler

import (
	"errors"
	"net/http"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"capstonewebserver/internal/middleware"

	"github.com/gin-gonic/gin"
)

var (
	roomIdParam = "room_id"
)

func respondError(c *gin.Context, err error) {
	var exists *domain.RoomExistsError
	switch {
	case errors.As(err, &exists):
		c.JSON(http.StatusConflict, dto.ErrorResponse{
			Error:  dto.ErrorDetail{Code: "room_exists", Message: exists.Error()},
			RoomID: exists.RoomID,
		})
	case errors.Is(err, domain.ErrRoomNotFound):
		c.JSON(http.StatusNotFound, errorBody("room_not_found", "room does not exist"))
	case errors.Is(err, domain.ErrInvalidPasscode):
		c.JSON(http.StatusForbidden, errorBody("invalid_passcode", "the passcode is missing or wrong"))
	case errors.Is(err, domain.ErrAdminTokenRequired):
		c.JSON(http.StatusUnauthorized, errorBody("admin_token_required", "X-Admin-Token header is required"))
	case errors.Is(err, domain.ErrNotAdmin):
		c.JSON(http.StatusForbidden, errorBody("not_admin", "admin token does not match this room"))
	case errors.Is(err, domain.ErrIPMismatch):
		c.JSON(http.StatusForbidden, errorBody("ip_mismatch", "request does not come from the room's creator IP"))
	case errors.Is(err, domain.ErrSFUUnreachable):
		c.JSON(http.StatusBadGateway, errorBody("sfu_unreachable", "SFU is unreachable"))
	case errors.Is(err, domain.ErrInvalidDisplayName):
		c.JSON(http.StatusUnprocessableEntity, errorBody("invalid_display_name", "display_name must be 1-40 characters after trimming whitespace"))
	case errors.Is(err, domain.ErrPeerNotLive):
		c.JSON(http.StatusUnprocessableEntity, errorBody("peer_not_live", "peer_id is not a live publisher in this room"))
	case errors.Is(err, domain.ErrInvalidTemplate):
		c.JSON(http.StatusUnprocessableEntity, errorBody("invalid_template", "template is not one of the known venue templates"))
	case errors.Is(err, domain.ErrInvalidPosition):
		c.JSON(http.StatusUnprocessableEntity, errorBody("invalid_position", "placement x/y must be between 0.0 and 1.0"))
	case errors.Is(err, domain.ErrUnknownStreamer):
		c.JSON(http.StatusUnprocessableEntity, errorBody("unknown_streamer", "streamer_id is not registered in this room"))
	case errors.Is(err, domain.ErrDuplicateStreamer):
		c.JSON(http.StatusUnprocessableEntity, errorBody("duplicate_streamer", "streamer_id is placed more than once"))
	case errors.Is(err, domain.ErrPlacementsWithoutTemplate):
		c.JSON(http.StatusUnprocessableEntity, errorBody("placements_without_template", "placements sent with a null template"))
	case errors.Is(err, domain.ErrBlacklisted):
		c.JSON(http.StatusForbidden, errorBody("blacklisted", "this IP is blocked from the room"))
	case errors.Is(err, domain.ErrPeerNotFound):
		c.JSON(http.StatusNotFound, errorBody("peer_not_found", "peer_id is not connected to this room right now"))
	case errors.Is(err, domain.ErrNotBlacklisted):
		c.JSON(http.StatusNotFound, errorBody("not_blacklisted", "ip is not on the blacklist"))
	default:
		c.JSON(http.StatusInternalServerError, errorBody("internal_error", "unexpected error"))
	}
}

func errorBody(code, message string) dto.ErrorResponse {
	return dto.ErrorResponse{Error: dto.ErrorDetail{Code: code, Message: message}}
}

func (h WebServerHandler) CreateRoom(c *gin.Context) {
	var req dto.RoomCreationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorBody("invalid_request", err.Error()))
		return
	}

	res, err := h.ruc.CreateRoom(c.Request.Context(), req.Passcode)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusCreated, res)
}

func (h WebServerHandler) GetRoomDetails(c *gin.Context) {
	roomId := c.Param(roomIdParam)
	res, err := h.ruc.RoomDetail(c.Request.Context(), roomId)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h WebServerHandler) GetRoomPublishers(c *gin.Context) {
	roomId := c.Param(roomIdParam)
	res, err := h.ruc.RoomPublishers(c.Request.Context(), roomId)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}

func (h WebServerHandler) CloseRoom(c *gin.Context) {
	roomId := c.Param(roomIdParam)
	if err := h.ruc.CloseRoom(c.Request.Context(), roomId); err != nil {
		respondError(c, err)
		return
	}

	c.AbortWithStatus(http.StatusNoContent)
}

func (h WebServerHandler) ReclaimRoomAdmin(c *gin.Context) {
	roomId := c.Param(roomIdParam)

	ip, _ := middleware.GetIp(c.Request.Context())
	var ipValue string
	if ip != nil {
		ipValue = *ip
	}

	res, err := h.ruc.ReclaimRoom(c.Request.Context(), roomId, ipValue)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, res)
}
