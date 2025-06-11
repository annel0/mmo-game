package auth

import "errors"

// UserRepository defines operations for user persistence and retrieval.
// For MVP we provide an in-memory implementation, but this interface allows
// swapping to a database-backed repository without touching the rest of the code.
type UserRepository interface {
	// GetUserByUsername returns a user by username (case-insensitive). If the user
	// is not found, (nil, ErrUserNotFound) should be returned.
	GetUserByUsername(username string) (*User, error)

	// CreateUser creates a new user with the supplied data and returns the stored
	// user instance. Caller is expected to pass a bcrypt-hashed password.
	// Implementations must enforce unique usernames and return ErrUserExists on
	// conflict.
	CreateUser(username string, passwordHash string, isAdmin bool) (*User, error)

	// === НОВЫЕ МЕТОДЫ ДЛЯ JWT ===
	// GetUserByID returns a user by ID. If the user is not found, (nil, ErrUserNotFound) should be returned.
	GetUserByID(id uint64) (*User, error)

	// ValidateCredentials validates username and password, returns user if valid
	ValidateCredentials(username, password string) (*User, error)
}

// Domain-level errors returned by the repository.
var (
	ErrUserNotFound = errors.New("user not found")
	ErrUserExists   = errors.New("user already exists")
)
