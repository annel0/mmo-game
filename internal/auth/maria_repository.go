package auth

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MariaConfig содержит настройки подключения к MariaDB
type MariaConfig struct {
	Host     string // например, localhost
	Port     int    // например, 3306
	Database string // например, blockverse
	Username string // пользователь БД
	Password string // пароль БД
}

// MariaUserRepo реализует UserRepository для MariaDB
type MariaUserRepo struct {
	db *sql.DB
}

// NewMariaUserRepo создает новое подключение к MariaDB и возвращает репозиторий
func NewMariaUserRepo(cfg MariaConfig) (*MariaUserRepo, error) {
	// Устанавливаем значения по умолчанию
	if cfg.Host == "" {
		cfg.Host = "localhost"
	}
	if cfg.Port == 0 {
		cfg.Port = 3306
	}
	if cfg.Database == "" {
		cfg.Database = "blockverse"
	}

	// Формируем DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.Database)

	// Открываем подключение
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть подключение к MariaDB: %w", err)
	}

	// Проверяем подключение
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("не удалось подключиться к MariaDB: %w", err)
	}

	repo := &MariaUserRepo{db: db}

	// Создаем таблицы, если их нет
	if err := repo.createTables(); err != nil {
		return nil, fmt.Errorf("не удалось создать таблицы: %w", err)
	}

	return repo, nil
}

// createTables создает необходимые таблицы в БД
func (m *MariaUserRepo) createTables() error {
	// Таблица пользователей
	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(50) NOT NULL UNIQUE,
		password_hash VARCHAR(255) NOT NULL,
		is_admin BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		last_login TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		INDEX idx_username (username)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	if _, err := m.db.Exec(createUsersTable); err != nil {
		return fmt.Errorf("не удалось создать таблицу users: %w", err)
	}

	// Создаем пользователей по умолчанию
	if err := m.createDefaultUsers(); err != nil {
		return fmt.Errorf("не удалось создать пользователей по умолчанию: %w", err)
	}

	return nil
}

// createDefaultUsers создает пользователей по умолчанию, если они не существуют
func (m *MariaUserRepo) createDefaultUsers() error {
	// Проверяем, есть ли пользователи в системе
	var userCount int
	err := m.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if err != nil {
		return fmt.Errorf("ошибка при проверке количества пользователей: %w", err)
	}

	// Если пользователи уже есть, не создаем по умолчанию
	if userCount > 0 {
		return nil
	}

	// Создаем администратора
	// Пароль: ChangeMe123!
	adminHash, err := HashPassword("ChangeMe123!")
	if err != nil {
		return fmt.Errorf("ошибка хеширования пароля администратора: %w", err)
	}
	_, err = m.CreateUser("admin", adminHash, true)
	if err != nil && err != ErrUserExists {
		return fmt.Errorf("не удалось создать администратора: %w", err)
	}

	// Создаем тестового пользователя
	// Пароль: test123
	testHash, err := HashPassword("test123")
	if err != nil {
		return fmt.Errorf("ошибка хеширования пароля тестового пользователя: %w", err)
	}
	_, err = m.CreateUser("test", testHash, false)
	if err != nil && err != ErrUserExists {
		return fmt.Errorf("не удалось создать тестового пользователя: %w", err)
	}

	return nil
}

// GetUserByUsername получает пользователя по имени
func (m *MariaUserRepo) GetUserByUsername(username string) (*User, error) {
	lower := strings.ToLower(username)

	query := `SELECT id, username, password_hash, is_admin, created_at, last_login 
			  FROM users WHERE username = ?`

	var user User
	err := m.db.QueryRow(query, lower).Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&user.IsAdmin,
		&user.CreatedAt,
		&user.LastLogin,
	)

	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении пользователя: %w", err)
	}

	return &user, nil
}

// CreateUser создает нового пользователя
func (m *MariaUserRepo) CreateUser(username string, passwordHash string, isAdmin bool) (*User, error) {
	lower := strings.ToLower(username)
	now := time.Now()

	query := `INSERT INTO users (username, password_hash, is_admin, created_at, last_login) 
			  VALUES (?, ?, ?, ?, ?)`

	result, err := m.db.Exec(query, lower, passwordHash, isAdmin, now, now)
	if err != nil {
		// Проверяем на дублирование пользователя
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, ErrUserExists
		}
		return nil, fmt.Errorf("ошибка при создании пользователя: %w", err)
	}

	// Получаем ID созданного пользователя
	userID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении ID пользователя: %w", err)
	}

	return &User{
		ID:           uint64(userID),
		Username:     lower,
		PasswordHash: passwordHash,
		IsAdmin:      isAdmin,
		CreatedAt:    now,
		LastLogin:    now,
	}, nil
}

// UpdateUserLastLogin обновляет время последнего входа пользователя
func (m *MariaUserRepo) UpdateUserLastLogin(userID uint64) error {
	query := `UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = ?`

	_, err := m.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("ошибка при обновлении времени входа: %w", err)
	}

	return nil
}

// GetUserStats возвращает статистику пользователей
func (m *MariaUserRepo) GetUserStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Общее количество пользователей
	var totalUsers int
	err := m.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&totalUsers)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении количества пользователей: %w", err)
	}
	stats["total_users"] = totalUsers

	// Количество админов
	var totalAdmins int
	err = m.db.QueryRow("SELECT COUNT(*) FROM users WHERE is_admin = TRUE").Scan(&totalAdmins)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении количества админов: %w", err)
	}
	stats["total_admins"] = totalAdmins

	// Пользователи за последние 24 часа
	var recentUsers int
	err = m.db.QueryRow("SELECT COUNT(*) FROM users WHERE last_login > DATE_SUB(NOW(), INTERVAL 24 HOUR)").Scan(&recentUsers)
	if err != nil {
		return nil, fmt.Errorf("ошибка при получении недавних пользователей: %w", err)
	}
	stats["recent_users_24h"] = recentUsers

	return stats, nil
}

// Close закрывает подключение к БД
func (m *MariaUserRepo) Close() error {
	return m.db.Close()
}
