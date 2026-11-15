package repository

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"capstonewebserver/internal/dto"
)

// GatewayBlocklist wraps a BlacklistStore and mirrors every change into a
// file the gateway (nginx) includes. Each blocked IP becomes one
// "deny <ip>;" line, so the gateway refuses the address before it reaches
// Go or the SFU, including the SFU's WebSocket at /ws.
//
// The gateway watches the file and reloads itself when it changes, so the
// backend only has to write it. A failed write rolls the change back, so
// the in-memory list and the gateway's copy never disagree on success.
type GatewayBlocklist struct {
	mu    sync.Mutex // serialises mutate + write, so the file never goes stale
	store *BlacklistStore
	path  string
}

// NewGatewayBlocklist writes the current contents of store to path straight
// away. The gateway includes the file at startup, so it has to exist before
// the gateway starts reading it.
func NewGatewayBlocklist(store *BlacklistStore, path string) (*GatewayBlocklist, error) {
	g := &GatewayBlocklist{store: store, path: path}
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.syncLocked(context.Background()); err != nil {
		return nil, err
	}
	return g, nil
}

// IsBlocked reports whether ip is currently blacklisted.
func (g *GatewayBlocklist) IsBlocked(ctx context.Context, ip string) bool {
	return g.store.IsBlocked(ctx, ip)
}

// Add blocks ip for peerId and returns the peer_id now on record for it.
// Blocking an IP that's already blocked changes nothing and writes nothing.
func (g *GatewayBlocklist) Add(ctx context.Context, ip, peerId string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	wasBlocked := g.store.IsBlocked(ctx, ip)
	recorded, err := g.store.Add(ctx, ip, peerId)
	if err != nil {
		return "", err
	}
	if wasBlocked {
		return recorded, nil
	}
	if err := g.syncLocked(ctx); err != nil {
		_, _ = g.store.Remove(ctx, ip)
		return "", err
	}
	return recorded, nil
}

// Remove unblocks ip. removed is false if ip wasn't blocked.
func (g *GatewayBlocklist) Remove(ctx context.Context, ip string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	prev, err := g.peerFor(ctx, ip)
	if err != nil {
		return false, err
	}
	removed, err := g.store.Remove(ctx, ip)
	if err != nil || !removed {
		return removed, err
	}
	if err := g.syncLocked(ctx); err != nil {
		_, _ = g.store.Add(ctx, ip, prev)
		return false, err
	}
	return true, nil
}

// List returns every blocked {ip, peer_id} pair.
func (g *GatewayBlocklist) List(ctx context.Context) ([]dto.BlacklistEntry, error) {
	return g.store.List(ctx)
}

// Clear drops every entry and empties the gateway's file.
func (g *GatewayBlocklist) Clear(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	prev, err := g.store.List(ctx)
	if err != nil {
		return err
	}
	if err := g.store.Clear(ctx); err != nil {
		return err
	}
	if err := g.syncLocked(ctx); err != nil {
		for _, e := range prev {
			_, _ = g.store.Add(ctx, e.IP, e.PeerID)
		}
		return err
	}
	return nil
}

// peerFor returns the peer_id recorded for ip, or "" if ip isn't blocked.
func (g *GatewayBlocklist) peerFor(ctx context.Context, ip string) (string, error) {
	entries, err := g.store.List(ctx)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if e.IP == ip {
			return e.PeerID, nil
		}
	}
	return "", nil
}

// syncLocked writes the store's current IPs to the gateway file. Callers
// hold g.mu.
func (g *GatewayBlocklist) syncLocked(ctx context.Context) error {
	entries, err := g.store.List(ctx)
	if err != nil {
		return err
	}
	ips := make([]string, 0, len(entries))
	for _, e := range entries {
		ips = append(ips, e.IP)
	}
	return WriteBlocklistFile(g.path, ips)
}

// WriteBlocklistFile replaces path with one "deny <ip>;" line per ip. The
// file is written to a temp file in the same directory and renamed over
// path, so the gateway never reads a half-written list. If any ip isn't a
// valid address, nothing is written and the existing file is kept: the
// output is nginx config, so only parsed addresses may go in.
func WriteBlocklistFile(path string, ips []string) error {
	var b strings.Builder
	b.WriteString("# Written by the webserver from the room blacklist. Do not edit.\n")
	for _, raw := range ips {
		parsed := net.ParseIP(raw)
		if parsed == nil {
			return fmt.Errorf("blocklist: invalid ip %q", raw)
		}
		fmt.Fprintf(&b, "deny %s;\n", parsed.String())
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".blocklist-*.tmp")
	if err != nil {
		return fmt.Errorf("blocklist: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("blocklist: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("blocklist: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("blocklist: close: %w", err)
	}
	// CreateTemp makes the file 0600; the gateway runs as another user and
	// has to read it.
	if err := os.Chmod(tmpName, 0o644); err != nil {
		cleanup()
		return fmt.Errorf("blocklist: chmod: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("blocklist: rename: %w", err)
	}
	return nil
}
