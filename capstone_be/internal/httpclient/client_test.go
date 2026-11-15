package httpclient

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/dto"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHTTPClient struct {
	resp *http.Response
	err  error
}

func (c *fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return c.resp, c.err
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestHttpClient_GetPublishers(t *testing.T) {
	tests := []struct {
		name        string
		client      Client
		checkResult func(t *testing.T, got []dto.Publisher, err error)
	}{
		{
			"decodes the publisher list",
			&fakeHTTPClient{resp: jsonResponse(http.StatusOK, `{"room_id":"ABC-123","publishers":[{"peer_id":"pub-abc123"}]}`)},
			func(t *testing.T, got []dto.Publisher, err error) {
				require.NoError(t, err)
				assert.Equal(t, []dto.Publisher{{PeerID: "pub-abc123"}}, got)
			},
		},
		{
			"non-2xx status is an error",
			&fakeHTTPClient{resp: jsonResponse(http.StatusInternalServerError, `{}`)},
			func(t *testing.T, got []dto.Publisher, err error) {
				assert.Error(t, err)
			},
		},
		{
			"transport failure is an error",
			&fakeHTTPClient{err: errors.New("connection refused")},
			func(t *testing.T, got []dto.Publisher, err error) {
				assert.Error(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHttpClient("http://sfu.internal", tt.client)
			got, err := c.GetPublishers(t.Context(), "ABC-123")
			tt.checkResult(t, got, err)
		})
	}
}

func TestHttpClient_GetPeerIP(t *testing.T) {
	tests := []struct {
		name        string
		client      Client
		checkResult func(t *testing.T, got string, err error)
	}{
		{
			"decodes the ip",
			&fakeHTTPClient{resp: jsonResponse(http.StatusOK, `{"room_id":"ABC-123","peer_id":"pub-abc123","ip":"203.0.113.7"}`)},
			func(t *testing.T, got string, err error) {
				require.NoError(t, err)
				assert.Equal(t, "203.0.113.7", got)
			},
		},
		{
			"404 becomes ErrPeerNotFound",
			&fakeHTTPClient{resp: jsonResponse(http.StatusNotFound, `{}`)},
			func(t *testing.T, got string, err error) {
				assert.ErrorIs(t, err, domain.ErrPeerNotFound)
			},
		},
		{
			"other non-2xx is a plain error",
			&fakeHTTPClient{resp: jsonResponse(http.StatusBadGateway, `{}`)},
			func(t *testing.T, got string, err error) {
				assert.Error(t, err)
				assert.NotErrorIs(t, err, domain.ErrPeerNotFound)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHttpClient("http://sfu.internal", tt.client)
			got, err := c.GetPeerIP(t.Context(), "ABC-123", "pub-abc123")
			tt.checkResult(t, got, err)
		})
	}
}

// recordingHTTPClient remembers the request so a test can check what was
// sent to the SFU.
type recordingHTTPClient struct {
	resp *http.Response
	req  *http.Request
}

func (c *recordingHTTPClient) Do(req *http.Request) (*http.Response, error) {
	c.req = req
	return c.resp, nil
}

func TestHttpClient_DisconnectPeer(t *testing.T) {
	tests := []struct {
		name        string
		client      Client
		checkResult func(t *testing.T, err error)
	}{
		{
			"204 is success",
			&fakeHTTPClient{resp: jsonResponse(http.StatusNoContent, ``)},
			func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			"404 becomes ErrPeerNotFound",
			&fakeHTTPClient{resp: jsonResponse(http.StatusNotFound, `{}`)},
			func(t *testing.T, err error) {
				assert.ErrorIs(t, err, domain.ErrPeerNotFound)
			},
		},
		{
			"other non-2xx is a plain error",
			&fakeHTTPClient{resp: jsonResponse(http.StatusInternalServerError, ``)},
			func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.NotErrorIs(t, err, domain.ErrPeerNotFound)
			},
		},
		{
			"transport failure is an error",
			&fakeHTTPClient{err: errors.New("connection refused")},
			func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHttpClient("http://sfu.internal", tt.client)
			err := c.DisconnectPeer(t.Context(), "ABC-123", "pub-abc123")
			tt.checkResult(t, err)
		})
	}
}

func TestHttpClient_DisconnectPeer_request(t *testing.T) {
	rec := &recordingHTTPClient{resp: jsonResponse(http.StatusNoContent, ``)}
	c := NewHttpClient("http://sfu.internal", rec)

	require.NoError(t, c.DisconnectPeer(t.Context(), "ABC-123", "pub-abc123"))

	assert.Equal(t, http.MethodPost, rec.req.Method)
	assert.Equal(t, "http://sfu.internal/internal/rooms/ABC-123/peers/pub-abc123/disconnect", rec.req.URL.String())
}

func TestHttpClient_CloseRoom(t *testing.T) {
	tests := []struct {
		name        string
		client      Client
		checkResult func(t *testing.T, err error)
	}{
		{
			"204 is success",
			&fakeHTTPClient{resp: jsonResponse(http.StatusNoContent, ``)},
			func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			"succeeds even if the SFU reports no such room",
			&fakeHTTPClient{resp: jsonResponse(http.StatusNoContent, ``)},
			func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			"non-2xx is an error",
			&fakeHTTPClient{resp: jsonResponse(http.StatusInternalServerError, ``)},
			func(t *testing.T, err error) {
				assert.Error(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewHttpClient("http://sfu.internal", tt.client)
			err := c.CloseRoom(t.Context(), "ABC-123")
			tt.checkResult(t, err)
		})
	}
}
