package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"capstonewebserver/internal/events"
	"capstonewebserver/internal/handler"
	"capstonewebserver/internal/httpclient"
	"capstonewebserver/internal/middleware"
	"capstonewebserver/internal/repository"
	"capstonewebserver/internal/usecase"

	"github.com/gin-gonic/gin"
)

// livenessInterval is how often Go polls the SFU for streamers going live
// or offline while an events stream is open (contracts.md decision 11).
const livenessInterval = 2 * time.Second

// newRouter wires the repositories, usecases and handlers together and
// registers every route in contracts.md section 1. The returned func is the
// streamer liveness poll; run it in its own goroutine for as long as the
// server is up.
func newRouter(cfg config) (*gin.Engine, func(ctx context.Context), error) {
	store := repository.NewStore()
	blacklist := repository.NewBlacklistStore()
	hub := events.NewHub()
	sfu := httpclient.NewHttpClient(cfg.SfuBaseURL, &http.Client{Timeout: cfg.SfuTimeout})

	// With BLOCKLIST_FILE set, every block is also written where the gateway
	// enforces it. Reads still come from the same in-memory store.
	var blacklistStore usecase.BlackListStore = blacklist
	if cfg.BlocklistFile != "" {
		gatewayList, err := repository.NewGatewayBlocklist(blacklist, cfg.BlocklistFile)
		if err != nil {
			return nil, nil, fmt.Errorf("gateway blocklist: %w", err)
		}
		blacklistStore = gatewayList
	}

	blacklistUC := usecase.NewBlacklistUseCase(store, sfu, blacklistStore)
	roomUC := usecase.NewRoomUseCase(store, sfu, blacklistUC, hub, usecase.RoomConfig{
		Secret:     []byte(cfg.JWTSecret),
		Passcode:   cfg.Passcode,
		SfuWsURL:   cfg.SfuWsURL,
		ICEServers: cfg.ICEServers,
	})
	layoutUC := usecase.NewLayoutUseCase(store, store, sfu, hub)
	streamerUC := usecase.NewStreamerUseCase(store, sfu, layoutUC)
	eventsUC := usecase.NewEventsUseCase(store, hub, layoutUC)
	livenessUC := usecase.NewLivenessUseCase(store, hub, layoutUC, hub)

	h := handler.NewWebServerHandler(roomUC, streamerUC, layoutUC, blacklistUC, eventsUC)

	r := gin.New()
	r.Use(gin.Recovery())
	// Behind nginx the client IP comes from X-Real-IP, which only a trusted
	// proxy may set (contracts.md reclaim).
	r.RemoteIPHeaders = []string{"X-Real-IP"}
	if err := r.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, nil, err
	}
	r.Use(middleware.CORSMiddleware(cfg.AllowedOrigins), middleware.IpMiddleware())

	currentRoom := func() string {
		id, _ := store.Current(context.Background())
		return id
	}
	admin := middleware.AuthMiddleware([]byte(cfg.JWTSecret), currentRoom)
	blacklistGuard := middleware.BlackListMiddleware(blacklist)

	api := r.Group("/api")
	api.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })

	api.POST("/rooms", h.CreateRoom)
	api.GET("/rooms/:room_id", blacklistGuard, h.GetRoomDetails)
	api.GET("/rooms/:room_id/publishers", blacklistGuard, h.GetRoomPublishers)
	api.DELETE("/rooms/:room_id", admin, h.CloseRoom)
	api.POST("/rooms/:room_id/admin/reclaim", h.ReclaimRoomAdmin)

	api.PUT("/rooms/:room_id/streamers/:streamer_id", blacklistGuard, h.RegisterStreamer)

	api.GET("/rooms/:room_id/stage", h.GetStage)
	api.PUT("/rooms/:room_id/stage", admin, h.PutStage)
	api.GET("/rooms/:room_id/events", h.Events)

	api.POST("/blacklist", admin, h.AddBlacklist)
	api.GET("/blacklist", admin, h.ListBlacklist)
	api.DELETE("/blacklist/:ip", admin, h.RemoveBlacklist)

	return r, func(ctx context.Context) { livenessUC.Run(ctx, livenessInterval) }, nil
}
