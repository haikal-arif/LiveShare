// Package httpclient implements usecase.RequestClient against the Rust
// SFU's internal REST API (contracts.md section 2).
package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
)

// Client is the subset of *http.Client HttpClient needs, so tests can fake
// it without a real network call.
type Client interface {
	Do(*http.Request) (*http.Response, error)
}

// HttpClient implements usecase.RequestClient.
type HttpClient struct {
	baseURL string
	client  Client
}

func NewHttpClient(baseURL string, client Client) *HttpClient {
	return &HttpClient{baseURL: baseURL, client: client}
}

func (c *HttpClient) GetPublishers(ctx context.Context, roomId string) ([]dto.Publisher, error) {
	path := fmt.Sprintf("/internal/rooms/%s/publishers", roomId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sfu: unexpected status %d from GET %s", resp.StatusCode, path)
	}

	var body struct {
		Publishers []dto.Publisher `json:"publishers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Publishers, nil
}

func (c *HttpClient) GetPeerIP(ctx context.Context, roomId, peerId string) (string, error) {
	path := fmt.Sprintf("/internal/rooms/%s/peers/%s", roomId, peerId)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", domain.ErrPeerNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sfu: unexpected status %d from GET %s", resp.StatusCode, path)
	}

	var body struct {
		IP string `json:"ip"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.IP, nil
}

func (c *HttpClient) DisconnectPeer(ctx context.Context, roomId, peerId string) error {
	path := fmt.Sprintf("/internal/rooms/%s/peers/%s/disconnect", roomId, peerId)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return domain.ErrPeerNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sfu: unexpected status %d from POST %s", resp.StatusCode, path)
	}
	return nil
}

func (c *HttpClient) CloseRoom(ctx context.Context, roomId string) error {
	path := fmt.Sprintf("/internal/rooms/%s/close", roomId)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// 204 even if the SFU has no such room, so any 2xx here is success.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sfu: unexpected status %d from POST %s", resp.StatusCode, path)
	}
	return nil
}
