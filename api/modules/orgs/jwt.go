package orgs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

// InitJWT reads JWT_SECRET; call once at startup.
func InitJWT() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 characters long")
	}
	jwtSecret = []byte(secret)
}

// Claims carries the authenticated user, their org, and role. org_id is the
// multi-tenancy boundary: middleware puts it in context and every query
// filters by it.
type Claims struct {
	UserID int64  `json:"user_id"`
	OrgID  int64  `json:"org_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

const AccessTokenExpiry = 8 * time.Hour

func CreateJWT(userID, orgID int64, role string) (string, error) {
	claims := Claims{
		UserID: userID,
		OrgID:  orgID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(AccessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func ParseJWT(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || claims.UserID == 0 || claims.OrgID == 0 {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}
