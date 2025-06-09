package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWT secret key - in production should be loaded from environment variable
var jwtSecret []byte

func init() {
	// Generate a secure random secret key
	jwtSecret = make([]byte, 32)
	if _, err := rand.Read(jwtSecret); err != nil {
		// Fallback to a hardcoded key only for development
		jwtSecret = []byte("development-secret-key-change-in-production")
	}
}

// Claims represents JWT claims
type Claims struct {
	PlayerID uint64 `json:"player_id"`
	Username string `json:"username"`
	IsAdmin  bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// GenerateJWT creates a secure JWT token for the given user
func GenerateJWT(user *User) (string, error) {
	claims := &Claims{
		PlayerID: user.ID,
		Username: user.Username,
		IsAdmin:  user.IsAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "mmo-game",
			Subject:   user.Username,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

// ValidateJWT checks token validity and returns associated user info
func ValidateJWT(tokenString string) (playerID uint64, isValid bool, isAdmin bool) {
	claims := &Claims{}
	
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret, nil
	})

	if err != nil || !token.Valid {
		return 0, false, false
	}

	return claims.PlayerID, true, claims.IsAdmin
}

// GenerateSecureSecret generates a new secure secret key
func GenerateSecureSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// SetJWTSecret allows setting a custom secret key (for production use)
func SetJWTSecret(secret string) error {
	decoded, err := base64.StdEncoding.DecodeString(secret)
	if err != nil {
		return err
	}
	if len(decoded) < 32 {
		return errors.New("secret key must be at least 32 bytes")
	}
	jwtSecret = decoded
	return nil
}