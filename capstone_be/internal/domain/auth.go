package domain

import "github.com/golang-jwt/jwt/v5"

// AdminClaims are the two claims carried by an admin_token, per contracts.md:
// "a JWT signed by the server with two claims: admin: true, and room_id set
// to the room it was issued for."
type AdminClaims struct {
	Admin  bool   `json:"admin"`
	RoomID string `json:"room_id"`
	jwt.RegisteredClaims
}
