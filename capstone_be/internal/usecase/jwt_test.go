package usecase_test

import (
	"capstonewebserver/internal/domain"
	"capstonewebserver/internal/usecase"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignAndVerifyAdminToken(t *testing.T) {
	tests := []struct {
		name        string
		roomId      string
		checkResult func(t *testing.T, token string)
	}{
		{
			"round trips the room id",
			"ABC-123",
			func(t *testing.T, token string) {
				claims, err := usecase.VerifyAdminToken(token, []byte("secret"))
				require.NoError(t, err)
				assert.True(t, claims.Admin)
				assert.Equal(t, "ABC-123", claims.RoomID)
			},
		},
		{
			"produces three dot-separated segments",
			"ABC-123",
			func(t *testing.T, token string) {
				assert.Regexp(t, `^[^.]+\.[^.]+\.[^.]+$`, token)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := usecase.SignAdminToken([]byte("secret"), tt.roomId)
			require.NoError(t, err)
			tt.checkResult(t, token)
		})
	}
}

func TestVerifyAdminToken(t *testing.T) {
	validToken, err := usecase.SignAdminToken([]byte("secret"), "ABC-123")
	require.NoError(t, err)

	tests := []struct {
		name        string
		token       string
		secret      []byte
		checkResult func(t *testing.T, claims domain.AdminClaims, err error)
	}{
		{
			"valid token",
			validToken,
			[]byte("secret"),
			func(t *testing.T, claims domain.AdminClaims, err error) {
				require.NoError(t, err)
				assert.Equal(t, domain.AdminClaims{Admin: true, RoomID: "ABC-123"}, claims)
			},
		},
		{
			"wrong secret",
			validToken,
			[]byte("other-secret"),
			func(t *testing.T, claims domain.AdminClaims, err error) {
				assert.ErrorIs(t, err, usecase.ErrInvalidSignature)
			},
		},
		{
			"malformed token",
			"not-a-jwt",
			[]byte("secret"),
			func(t *testing.T, claims domain.AdminClaims, err error) {
				assert.ErrorIs(t, err, usecase.ErrMalformedToken)
			},
		},
		{
			"empty token",
			"",
			[]byte("secret"),
			func(t *testing.T, claims domain.AdminClaims, err error) {
				assert.ErrorIs(t, err, usecase.ErrMalformedToken)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := usecase.VerifyAdminToken(tt.token, tt.secret)
			tt.checkResult(t, claims, err)
		})
	}
}
