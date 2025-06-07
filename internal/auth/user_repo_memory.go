package auth

import (
	"strings"
	"sync"
	"time"
)

// MemoryUserRepo is a threadsafe in-memory storage useful for tests & single-instance servers.
// NOT suitable for production without persistence.
// It also handles incremental ID assignment.
// ID counter starts from 1.
type MemoryUserRepo struct {
	mu     sync.RWMutex
	users  map[string]*User // key = lowercase(username)
	nextID uint64
}

// NewMemoryUserRepo returns repository pre-populated with a single test user
// (username: test, password: test, non-admin).
func NewMemoryUserRepo() (*MemoryUserRepo, error) {
	repo := &MemoryUserRepo{
		users:  make(map[string]*User),
		nextID: 1,
	}

	// Create default test user (username=test, password=test)
	passwordHash, err := HashPassword("test")
	if err != nil {
		return nil, err
	}
	_, err = repo.CreateUser("test", passwordHash, false)
	if err != nil {
		return nil, err
	}

	// Also create admin user (username=admin, password=admin)
	adminHash, err := HashPassword("admin")
	if err != nil {
		return nil, err
	}
	_, _ = repo.CreateUser("admin", adminHash, true)

	return repo, nil
}

// GetUserByUsername retrieves user by case-insensitive username.
func (r *MemoryUserRepo) GetUserByUsername(username string) (*User, error) {
	key := normalize(username)
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[key]
	if !ok {
		return nil, ErrUserNotFound
	}
	return user, nil
}

// CreateUser inserts a new user if username not present.
func (r *MemoryUserRepo) CreateUser(username string, passwordHash string, isAdmin bool) (*User, error) {
	key := normalize(username)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[key]; exists {
		return nil, ErrUserExists
	}

	user := &User{
		ID:           r.nextID,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now(),
		LastLogin:    time.Now(),
		IsAdmin:      isAdmin,
	}
	r.nextID++
	r.users[key] = user
	return user, nil
}

// Helper to normalise usernames.
func normalize(username string) string {
	return strings.ToLower(username)
}
