package usecase

import (
	"errors"

	"capstonewebserver/internal/domain"

	"github.com/golang-jwt/jwt/v5"
)

var ErrMalformedToken = errors.New("malformed admin token")
var ErrInvalidSignature = errors.New("admin token signature is invalid")

// SignAdminToken mints the admin_token returned by POST /api/rooms and
// POST /api/rooms/:room_id/admin/reclaim.
func SignAdminToken(secret []byte, roomID string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, domain.AdminClaims{Admin: true, RoomID: roomID})
	return token.SignedString(secret)
}

// VerifyAdminToken checks the signature on tokenString and returns its
// claims. It does not check the claims against the currently open room;
// callers compare RoomID themselves.
func VerifyAdminToken(tokenString string, secret []byte) (domain.AdminClaims, error) {
	var claims domain.AdminClaims
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(t *jwt.Token) (any, error) {
		return secret, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, jwt.ErrTokenMalformed):
			return domain.AdminClaims{}, ErrMalformedToken
		case errors.Is(err, jwt.ErrTokenSignatureInvalid):
			return domain.AdminClaims{}, ErrInvalidSignature
		default:
			return domain.AdminClaims{}, err
		}
	}
	if !token.Valid {
		return domain.AdminClaims{}, ErrMalformedToken
	}

	return claims, nil
}
