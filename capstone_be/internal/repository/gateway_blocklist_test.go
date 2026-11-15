package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBlocklistFile(t *testing.T) {
	tests := []struct {
		name        string
		ips         []string
		subdir      string // non-empty: write under a directory that doesn't exist
		setup       func(t *testing.T, path string)
		checkResult func(t *testing.T, path string, err error)
	}{
		{
			name: "empty list writes header only",
			ips:  nil,
			checkResult: func(t *testing.T, path string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				if strings.Contains(string(b), "deny ") {
					t.Fatalf("expected no deny lines, got %q", b)
				}
			},
		},
		{
			name: "each ip becomes one deny line",
			ips:  []string{"192.168.1.10", "10.0.0.5"},
			checkResult: func(t *testing.T, path string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				for _, want := range []string{"deny 192.168.1.10;", "deny 10.0.0.5;"} {
					if !strings.Contains(string(b), want) {
						t.Fatalf("missing %q in %q", want, b)
					}
				}
			},
		},
		{
			name: "ipv6 is normalised",
			ips:  []string{"2001:0db8:0000:0000:0000:0000:0000:0001"},
			checkResult: func(t *testing.T, path string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				if !strings.Contains(string(b), "deny 2001:db8::1;") {
					t.Fatalf("expected normalised ipv6 line, got %q", b)
				}
			},
		},
		{
			name: "invalid ip is rejected and existing file is kept",
			ips:  []string{"10.0.0.5", "1.1.1.1; return 200"},
			setup: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte("deny 9.9.9.9;\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			checkResult: func(t *testing.T, path string, err error) {
				if err == nil {
					t.Fatal("expected error for invalid ip")
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				if string(b) != "deny 9.9.9.9;\n" {
					t.Fatalf("existing file changed: %q", b)
				}
				entries, derr := os.ReadDir(filepath.Dir(path))
				if derr != nil {
					t.Fatal(derr)
				}
				for _, e := range entries {
					if strings.HasSuffix(e.Name(), ".tmp") {
						t.Fatalf("leftover temp file %s", e.Name())
					}
				}
			},
		},
		{
			name:   "missing directory returns error",
			ips:    []string{"10.0.0.5"},
			subdir: "does-not-exist",
			checkResult: func(t *testing.T, path string, err error) {
				if err == nil {
					t.Fatal("expected error when directory is missing")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "blocklist.conf")
			if tt.subdir != "" {
				path = filepath.Join(dir, tt.subdir, "blocklist.conf")
			}
			if tt.setup != nil {
				tt.setup(t, path)
			}
			err := WriteBlocklistFile(path, tt.ips)
			tt.checkResult(t, path, err)
		})
	}
}

func TestGatewayBlocklist(t *testing.T) {
	tests := []struct {
		name string
		// run drives the wrapper. dir is the directory holding the file.
		run func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string)
		// checkResult inspects the file and the in-memory store afterwards.
		checkResult func(t *testing.T, path string, store *BlacklistStore)
	}{
		{
			name: "add writes the ip to the file",
			run: func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string) {
				if _, err := g.Add(context.Background(), "10.0.0.5", "peer-1"); err != nil {
					t.Fatal(err)
				}
			},
			checkResult: func(t *testing.T, path string, store *BlacklistStore) {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(b), "deny 10.0.0.5;") {
					t.Fatalf("expected deny line, got %q", b)
				}
			},
		},
		{
			name: "remove drops the ip from the file",
			run: func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string) {
				ctx := context.Background()
				if _, err := g.Add(ctx, "10.0.0.5", "peer-1"); err != nil {
					t.Fatal(err)
				}
				if removed, err := g.Remove(ctx, "10.0.0.5"); err != nil || !removed {
					t.Fatalf("remove: removed=%v err=%v", removed, err)
				}
			},
			checkResult: func(t *testing.T, path string, store *BlacklistStore) {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(b), "10.0.0.5") {
					t.Fatalf("ip still in file: %q", b)
				}
			},
		},
		{
			name: "clear empties the file",
			run: func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string) {
				ctx := context.Background()
				_, _ = g.Add(ctx, "10.0.0.5", "peer-1")
				_, _ = g.Add(ctx, "10.0.0.6", "peer-2")
				if err := g.Clear(ctx); err != nil {
					t.Fatal(err)
				}
			},
			checkResult: func(t *testing.T, path string, store *BlacklistStore) {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(b), "deny ") {
					t.Fatalf("expected no deny lines after clear, got %q", b)
				}
			},
		},
		{
			name: "re-blocking keeps the first peer and one deny line",
			run: func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string) {
				ctx := context.Background()
				_, _ = g.Add(ctx, "10.0.0.5", "peer-1")
				recorded, err := g.Add(ctx, "10.0.0.5", "peer-2")
				if err != nil {
					t.Fatal(err)
				}
				if recorded != "peer-1" {
					t.Fatalf("recorded peer = %q, want peer-1", recorded)
				}
			},
			checkResult: func(t *testing.T, path string, store *BlacklistStore) {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if n := strings.Count(string(b), "deny 10.0.0.5;"); n != 1 {
					t.Fatalf("expected one deny line, got %d", n)
				}
			},
		},
		{
			name: "failed write rolls the add back out of the store",
			run: func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string) {
				// Remove the directory so the next file write fails.
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if _, err := g.Add(context.Background(), "10.0.0.5", "peer-1"); err == nil {
					t.Fatal("expected write error")
				}
			},
			checkResult: func(t *testing.T, path string, store *BlacklistStore) {
				if store.IsBlocked(context.Background(), "10.0.0.5") {
					t.Fatal("store still blocks the ip after a failed write")
				}
			},
		},
		{
			name: "failed write rolls a remove back into the store",
			run: func(t *testing.T, g *GatewayBlocklist, store *BlacklistStore, dir string) {
				ctx := context.Background()
				if _, err := g.Add(ctx, "10.0.0.5", "peer-1"); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				if _, err := g.Remove(ctx, "10.0.0.5"); err == nil {
					t.Fatal("expected write error")
				}
			},
			checkResult: func(t *testing.T, path string, store *BlacklistStore) {
				if !store.IsBlocked(context.Background(), "10.0.0.5") {
					t.Fatal("store lost the ip after a failed remove write")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "blocklist.conf")
			store := NewBlacklistStore()
			g, err := NewGatewayBlocklist(store, path)
			if err != nil {
				t.Fatal(err)
			}
			tt.run(t, g, store, dir)
			tt.checkResult(t, path, store)
		})
	}
}

func TestNewGatewayBlocklistWritesEmptyFileAtStartup(t *testing.T) {
	tests := []struct {
		name        string
		path        func(dir string) string
		checkResult func(t *testing.T, path string, err error)
	}{
		{
			name: "creates the file with only the header",
			path: func(dir string) string { return filepath.Join(dir, "blocklist.conf") },
			checkResult: func(t *testing.T, path string, err error) {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				b, rerr := os.ReadFile(path)
				if rerr != nil {
					t.Fatal(rerr)
				}
				if strings.Contains(string(b), "deny ") {
					t.Fatalf("expected empty blocklist, got %q", b)
				}
			},
		},
		{
			name: "missing directory fails startup",
			path: func(dir string) string { return filepath.Join(dir, "missing", "blocklist.conf") },
			checkResult: func(t *testing.T, path string, err error) {
				if err == nil {
					t.Fatal("expected startup error when the directory is missing")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.path(t.TempDir())
			_, err := NewGatewayBlocklist(NewBlacklistStore(), path)
			tt.checkResult(t, path, err)
		})
	}
}
