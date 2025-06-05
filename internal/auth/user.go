package auth

import "time"

// User represents a player/administrator account.
// NOTE: This is the minimal structure required for the MVP authentication layer.
type User struct {
	ID           uint64    // Unique immutable identifier
	Username     string    // Unique username (case-insensitive)
	PasswordHash string    // bcrypt hashed password (60 chars)
	CreatedAt    time.Time // Account creation timestamp (server time)
	LastLogin    time.Time // Last successful login
	IsAdmin      bool      // Administrative privileges flag
}
