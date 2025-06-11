package auth

import (
	"log"
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

	// Create first admin user if no users exist (for initial setup)
	// This should be replaced with proper admin creation in production
	adminHash, err := HashPassword("ChangeMe123!")
	if err != nil {
		return nil, err
	}
	_, err = repo.CreateUser("admin", adminHash, true)
	if err != nil {
		return nil, err
	}

	log.Printf("SECURITY WARNING: Default admin user created with password 'ChangeMe123!' - CHANGE IMMEDIATELY!")

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
		Role:         "", // Будет установлено через GetRole()
	}
	r.nextID++
	r.users[key] = user
	return user, nil
}

// === НОВЫЕ МЕТОДЫ ДЛЯ JWT ===

// GetUserByID retrieves user by ID
func (r *MemoryUserRepo) GetUserByID(id uint64) (*User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Ищем пользователя по ID (менее эффективно, но для in-memory репозитория приемлемо)
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}

	return nil, ErrUserNotFound
}

// ValidateCredentials проверяет учетные данные пользователя
func (r *MemoryUserRepo) ValidateCredentials(username, password string) (*User, error) {
	// Получаем пользователя
	user, err := r.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	// Проверяем пароль
	if !CheckPassword(user.PasswordHash, password) {
		return nil, ErrUserNotFound // Возвращаем тот же тип ошибки для безопасности
	}

	// Обновляем время последнего входа
	r.mu.Lock()
	user.LastLogin = time.Now()
	r.mu.Unlock()

	return user, nil
}

// Helper to normalise usernames.
func normalize(username string) string {
	return strings.ToLower(username)
}
