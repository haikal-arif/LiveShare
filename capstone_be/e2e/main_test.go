//go:build e2e

// Package e2e black-box tests the webserver binary over real HTTP, the way
// the frontend would. TestMain builds cmd/webserver, starts it against a
// fake SFU (the Rust service's internal REST API, contracts.md section 2)
// and every test talks to it with a plain net/http client.
//
//	go test -tags e2e ./e2e/...
package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	passcode = "the-passcode-from-env"
	secret   = "e2e-jwt-secret"
	sfuWsURL = "wss://sfu.example.com/ws"
	iceURL   = "stun:stun.example.com:3478"

	// creatorIP is the address rooms are created from; reclaim only works
	// from it.
	creatorIP = "203.0.113.10"
)

var (
	baseURL string
	sfu     *fakeSFU
	state   serverState
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	dir, err := os.MkdirTemp("", "webserver-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mkdir temp:", err)
		return 1
	}
	defer os.RemoveAll(dir)

	bin := filepath.Join(dir, "webserver")
	build := exec.Command("go", "build", "-o", bin, "../cmd/webserver")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "build webserver:", err)
		return 1
	}

	sfu = newFakeSFU()
	defer sfu.Close()

	addr, err := freeAddr()
	if err != nil {
		fmt.Fprintln(os.Stderr, "free port:", err)
		return 1
	}
	baseURL = "http://" + addr

	srv := exec.Command(bin)
	srv.Env = append(os.Environ(),
		"ADDR="+addr,
		"ROOM_PASSCODE="+passcode,
		"JWT_SECRET="+secret,
		"SFU_BASE_URL="+sfu.URL(),
		"SFU_WS_URL="+sfuWsURL,
		"ICE_SERVERS="+iceURL,
		"ALLOWED_ORIGINS=https://app.example.com",
		"TRUSTED_PROXIES=127.0.0.1,::1",
		"GIN_MODE=release",
	)
	srv.Stdout, srv.Stderr = os.Stdout, os.Stderr
	if err := srv.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start webserver:", err)
		return 1
	}
	defer func() {
		_ = srv.Process.Signal(syscall.SIGTERM)
		_ = srv.Wait()
	}()

	if err := waitHealthy(baseURL+"/api/health", 10*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "webserver never became healthy:", err)
		return 1
	}

	return m.Run()
}

func freeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer l.Close()
	return l.Addr().String(), nil
}

func waitHealthy(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s (last error: %v)", timeout, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// --- HTTP helpers ---

type request struct {
	method string
	path   string
	body   string
	token  string
	// ip is sent as X-Real-IP, the header the reverse proxy sets; the server
	// trusts it from loopback only.
	ip     string
	origin string
}

type response struct {
	status int
	header http.Header
	body   []byte
}

func (r response) decode(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.body, v); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
}

// send performs req against the server. A successful room creation is
// remembered in state so begin's cleanup can close it again.
func send(t *testing.T, req request) response {
	t.Helper()

	var body io.Reader
	if req.body != "" {
		body = strings.NewReader(req.body)
	}
	httpReq, err := http.NewRequest(req.method, baseURL+req.path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if req.body != "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.token != "" {
		httpReq.Header.Set("X-Admin-Token", req.token)
	}
	if req.ip != "" {
		httpReq.Header.Set("X-Real-IP", req.ip)
	}
	if req.origin != "" {
		httpReq.Header.Set("Origin", req.origin)
	}

	httpRes, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("%s %s: %v", req.method, req.path, err)
	}
	defer httpRes.Body.Close()
	raw, err := io.ReadAll(httpRes.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	res := response{status: httpRes.StatusCode, header: httpRes.Header, body: raw}

	if req.method == http.MethodPost && req.path == "/api/rooms" && res.status == http.StatusCreated {
		var created struct {
			RoomID     string `json:"room_id"`
			AdminToken string `json:"admin_token"`
		}
		res.decode(t, &created)
		state.set(created.RoomID, created.AdminToken)
	}
	return res
}

// env is what a test case works with: the open room and its admin token.
type env struct {
	roomID string
	token  string
}

// serverState remembers the open room so it can be closed between tests; the
// server hosts one room at a time.
type serverState struct {
	mu     sync.Mutex
	roomID string
	token  string
}

func (s *serverState) set(roomID, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.roomID, s.token = roomID, token
}

func (s *serverState) take() (roomID, token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	roomID, token = s.roomID, s.token
	s.roomID, s.token = "", ""
	return roomID, token
}

// begin gives the test a clean server (no room, healthy fake SFU, empty
// peers) and restores that when the test ends. Call it first in every test
// and subtest that shares the server.
func begin(t *testing.T) {
	t.Helper()
	reset := func() {
		sfu.reset()
		if roomID, token := state.take(); roomID != "" {
			res := send(t, request{method: http.MethodDelete, path: "/api/rooms/" + roomID, token: token})
			if res.status != http.StatusNoContent && res.status != http.StatusNotFound && res.status != http.StatusForbidden {
				t.Errorf("cleanup: closing room %s got %d: %s", roomID, res.status, res.body)
			}
		}
	}
	reset()
	t.Cleanup(reset)
}

// openRoom creates the server's room from creatorIP.
func openRoom(t *testing.T) env {
	t.Helper()
	res := send(t, request{method: http.MethodPost, path: "/api/rooms", body: `{"passcode":"` + passcode + `"}`, ip: creatorIP})
	if res.status != http.StatusCreated {
		t.Fatalf("open room: got %d: %s", res.status, res.body)
	}
	var created struct {
		RoomID     string `json:"room_id"`
		AdminToken string `json:"admin_token"`
	}
	res.decode(t, &created)
	return env{roomID: created.RoomID, token: created.AdminToken}
}

// --- fake SFU ---

// fakeSFU stands in for the Rust SFU's internal REST API.
type fakeSFU struct {
	srv *httptest.Server

	mu         sync.Mutex
	down       bool
	publishers map[string][]string // room_id -> peer_ids
	peerIPs    map[string]string   // room_id/peer_id -> ip
	closed     map[string]bool

	failDisconnect bool
	disconnected   map[string]bool // room_id/peer_id
}

func newFakeSFU() *fakeSFU {
	f := &fakeSFU{}
	f.reset()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/rooms/{room_id}/publishers", f.handlePublishers)
	mux.HandleFunc("GET /internal/rooms/{room_id}/peers/{peer_id}", f.handlePeer)
	mux.HandleFunc("POST /internal/rooms/{room_id}/peers/{peer_id}/disconnect", f.handleDisconnect)
	mux.HandleFunc("POST /internal/rooms/{room_id}/close", f.handleClose)
	mux.HandleFunc("GET /internal/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	f.srv = httptest.NewServer(mux)
	return f
}

func (f *fakeSFU) URL() string { return f.srv.URL }
func (f *fakeSFU) Close()      { f.srv.Close() }

func (f *fakeSFU) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = false
	f.publishers = make(map[string][]string)
	f.peerIPs = make(map[string]string)
	f.closed = make(map[string]bool)
	f.failDisconnect = false
	f.disconnected = make(map[string]bool)
}

// setDown makes every SFU call fail with 503.
func (f *fakeSFU) setDown(down bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = down
}

func (f *fakeSFU) setPublishers(roomID string, peerIDs ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishers[roomID] = peerIDs
}

func (f *fakeSFU) setPeerIP(roomID, peerID, ip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.peerIPs[roomID+"/"+peerID] = ip
}

// setFailDisconnect makes only the disconnect call fail, with 500.
func (f *fakeSFU) setFailDisconnect(fail bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failDisconnect = fail
}

func (f *fakeSFU) wasDisconnected(roomID, peerID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.disconnected[roomID+"/"+peerID]
}

func (f *fakeSFU) wasClosed(roomID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed[roomID]
}

func (f *fakeSFU) isDown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.down
}

func (f *fakeSFU) handlePublishers(w http.ResponseWriter, r *http.Request) {
	if f.isDown() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	roomID := r.PathValue("room_id")

	f.mu.Lock()
	peers := f.publishers[roomID]
	f.mu.Unlock()

	type publisher struct {
		PeerID string `json:"peer_id"`
	}
	list := make([]publisher, 0, len(peers))
	for _, p := range peers {
		list = append(list, publisher{PeerID: p})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"room_id": roomID, "publishers": list})
}

func (f *fakeSFU) handlePeer(w http.ResponseWriter, r *http.Request) {
	if f.isDown() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	roomID, peerID := r.PathValue("room_id"), r.PathValue("peer_id")

	f.mu.Lock()
	ip, ok := f.peerIPs[roomID+"/"+peerID]
	f.mu.Unlock()
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "peer_not_found"}})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"room_id": roomID, "peer_id": peerID, "ip": ip})
}

func (f *fakeSFU) handleClose(w http.ResponseWriter, r *http.Request) {
	if f.isDown() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	f.mu.Lock()
	f.closed[r.PathValue("room_id")] = true
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// handleDisconnect drops the peer like the real SFU would: it leaves the
// publisher list and can no longer be resolved.
func (f *fakeSFU) handleDisconnect(w http.ResponseWriter, r *http.Request) {
	if f.isDown() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	roomID, peerID := r.PathValue("room_id"), r.PathValue("peer_id")

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failDisconnect {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	key := roomID + "/" + peerID
	if _, ok := f.peerIPs[key]; !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "peer_not_found"}})
		return
	}
	delete(f.peerIPs, key)
	remaining := f.publishers[roomID][:0:0]
	for _, p := range f.publishers[roomID] {
		if p != peerID {
			remaining = append(remaining, p)
		}
	}
	f.publishers[roomID] = remaining
	f.disconnected[key] = true
	w.WriteHeader(http.StatusNoContent)
}
