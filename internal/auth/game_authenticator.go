package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GameAuthenticator handles game authentication with JWT support
type GameAuthenticator struct {
	userRepo  UserRepository
	jwtSecret []byte
}

// AuthResult represents the result of authentication
type AuthResult struct {
	Success  bool
	UserID   uint64
	Username string
	Token    string
	Message  string
	Roles    []string
}

// NewGameAuthenticator creates a new game authenticator
func NewGameAuthenticator(userRepo UserRepository, jwtSecret []byte) *GameAuthenticator {
	return &GameAuthenticator{
		userRepo:  userRepo,
		jwtSecret: jwtSecret,
	}
}

// AuthenticateUser authenticates a user with username and password
func (ga *GameAuthenticator) AuthenticateUser(username, password string) (*AuthResult, error) {
	// Find user by username
	user, err := ga.userRepo.GetUserByUsername(username)
	if err != nil {
		return &AuthResult{
			Success: false,
			Message: "User not found",
		}, nil
	}

	// Verify password
	if !CheckPassword(user.PasswordHash, password) {
		return &AuthResult{
			Success: false,
			Message: "Invalid credentials",
		}, nil
	}

	// Generate JWT token
	token, err := ga.generateJWT(user.ID, user.Username)
	if err != nil {
		return &AuthResult{
			Success: false,
			Message: "Failed to generate token",
		}, err
	}

	return &AuthResult{
		Success:  true,
		UserID:   user.ID,
		Username: user.Username,
		Token:    token,
		Message:  "Authentication successful",
		Roles:    []string{user.GetRole()},
	}, nil
}

// ValidateToken validates a JWT token and returns user info
func (ga *GameAuthenticator) ValidateToken(tokenString string) (*AuthResult, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return ga.jwtSecret, nil
	})

	if err != nil {
		return &AuthResult{
			Success: false,
			Message: "Invalid token",
		}, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID := uint64(claims["user_id"].(float64))
		username := claims["username"].(string)

		return &AuthResult{
			Success:  true,
			UserID:   userID,
			Username: username,
			Token:    tokenString,
			Message:  "Token valid",
		}, nil
	}

	return &AuthResult{
		Success: false,
		Message: "Invalid token claims",
	}, nil
}

// generateJWT generates a JWT token for a user
func (ga *GameAuthenticator) generateJWT(userID uint64, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // 24 hours
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(ga.jwtSecret)
}
