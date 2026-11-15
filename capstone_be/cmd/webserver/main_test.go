package main

import (
	"testing"

	"capstonewebserver/internal/dto"
)

func TestTurnServers(t *testing.T) {
	cases := []struct {
		name        string
		setup       func() (urls, username, credential string)
		checkResult func(t *testing.T, got []dto.ICEServer)
	}{
		{
			name:  "no urls means no turn entry",
			setup: func() (string, string, string) { return "", "u", "p" },
			checkResult: func(t *testing.T, got []dto.ICEServer) {
				if got != nil {
					t.Fatalf("got %+v, want nil", got)
				}
			},
		},
		{
			name: "urls are split and carry the credentials",
			setup: func() (string, string, string) {
				return "turn:a.example:3478?transport=udp, turn:a.example:3478?transport=tcp", "u", "p"
			},
			checkResult: func(t *testing.T, got []dto.ICEServer) {
				if len(got) != 1 {
					t.Fatalf("got %d entries, want 1", len(got))
				}
				want := dto.ICEServer{
					URLs:       []string{"turn:a.example:3478?transport=udp", "turn:a.example:3478?transport=tcp"},
					Username:   "u",
					Credential: "p",
				}
				if got[0].Username != want.Username || got[0].Credential != want.Credential || len(got[0].URLs) != 2 || got[0].URLs[0] != want.URLs[0] || got[0].URLs[1] != want.URLs[1] {
					t.Fatalf("got %+v, want %+v", got[0], want)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			urls, username, credential := tc.setup()
			tc.checkResult(t, turnServers(urls, username, credential))
		})
	}
}
