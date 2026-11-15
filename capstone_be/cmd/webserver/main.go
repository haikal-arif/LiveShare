package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"capstonewebserver/internal/dto"
)

// config is the server's static configuration, read from the environment
// (.env in development).
type config struct {
	Addr           string
	Passcode       string
	JWTSecret      string
	SfuBaseURL     string
	SfuWsURL       string
	SfuTimeout     time.Duration
	ICEServers     []dto.ICEServer
	AllowedOrigins []string
	TrustedProxies []string
	// BlocklistFile is where blocked IPs are written for the gateway to
	// enforce. Empty means no gateway file: blocks are enforced by Go only.
	BlocklistFile string
}

func loadConfig() (config, error) {
	cfg := config{
		Addr:           getenv("ADDR", ":8080"),
		Passcode:       os.Getenv("ROOM_PASSCODE"),
		JWTSecret:      os.Getenv("JWT_SECRET"),
		SfuBaseURL:     strings.TrimRight(os.Getenv("SFU_BASE_URL"), "/"),
		SfuWsURL:       os.Getenv("SFU_WS_URL"),
		SfuTimeout:     5 * time.Second,
		AllowedOrigins: splitList(os.Getenv("ALLOWED_ORIGINS")),
		TrustedProxies: splitList(getenv("TRUSTED_PROXIES", "127.0.0.1,::1")),
		BlocklistFile:  os.Getenv("BLOCKLIST_FILE"),
	}
	if urls := splitList(os.Getenv("ICE_SERVERS")); len(urls) > 0 {
		cfg.ICEServers = []dto.ICEServer{{URLs: urls}}
	}
	cfg.ICEServers = append(cfg.ICEServers, turnServers(
		os.Getenv("TURN_URLS"), os.Getenv("TURN_USERNAME"), os.Getenv("TURN_CREDENTIAL"),
	)...)

	var missing []string
	for name, v := range map[string]string{
		"ROOM_PASSCODE": cfg.Passcode,
		"JWT_SECRET":    cfg.JWTSecret,
		"SFU_BASE_URL":  cfg.SfuBaseURL,
	} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	r, pollLiveness, err := newRouter(cfg)
	if err != nil {
		log.Fatal(err)
	}
	go pollLiveness(context.Background())

	log.Printf("webserver listening on %s", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// turnServers is the TURN entry for TURN_URLS (comma-separated) with its
// credentials, or nothing when no URLs are set.
func turnServers(urls, username, credential string) []dto.ICEServer {
	list := splitList(urls)
	if len(list) == 0 {
		return nil
	}
	return []dto.ICEServer{{URLs: list, Username: username, Credential: credential}}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
