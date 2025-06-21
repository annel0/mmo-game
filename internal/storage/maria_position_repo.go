package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/annel0/mmo-game/internal/vec"
	_ "github.com/go-sql-driver/mysql"
)

// MariaPositionRepo реализует PositionRepo для базы данных MariaDB/MySQL.
// Использует таблицу player_positions для хранения позиций игроков.
type MariaPositionRepo struct {
	db *sql.DB
}

// NewMariaPositionRepo создает новый репозиторий позиций для MariaDB.
// Автоматически создает таблицу, если она не существует.
//
// Параметры:
//
//	dsn - строка подключения к базе данных (user:pass@tcp(host:port)/dbname)
//
// Возвращает:
//
//	*MariaPositionRepo - экземпляр репозитория
//	error - ошибка при подключении или создании таблицы
func NewMariaPositionRepo(dsn string) (*MariaPositionRepo, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к MariaDB: %w", err)
	}

	// Проверяем соединение
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("не удалось проверить соединение с MariaDB: %w", err)
	}

	repo := &MariaPositionRepo{db: db}

	// Создаем таблицу, если она не существует
	if err := repo.createTable(); err != nil {
		db.Close()
		return nil, fmt.Errorf("не удалось создать таблицу: %w", err)
	}

	return repo, nil
}

// createTable создает таблицу player_positions, если она не существует.
func (r *MariaPositionRepo) createTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS player_positions (
			user_id    BIGINT      PRIMARY KEY,
			x          INT         NOT NULL,
			y          INT         NOT NULL,
			layer      TINYINT     NOT NULL DEFAULT 1,
			updated_at TIMESTAMP   DEFAULT CURRENT_TIMESTAMP
			           ON UPDATE   CURRENT_TIMESTAMP,
			INDEX idx_updated_at (updated_at)
		) ENGINE=InnoDB
	`

	_, err := r.db.Exec(query)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы player_positions: %w", err)
	}

	return nil
}

// Save сохраняет позицию игрока в базе данных.
// Использует INSERT ... ON DUPLICATE KEY UPDATE для обновления существующих записей.
func (r *MariaPositionRepo) Save(ctx context.Context, userID uint64, pos vec.Vec3) error {
	// Валидация входных данных
	if userID == 0 {
		return fmt.Errorf("недействительный userID: %d", userID)
	}

	// Проверяем корректность layer (слоя)
	if pos.Z < 0 || pos.Z > 255 {
		return fmt.Errorf("недействительный layer: %d (должен быть 0-255)", pos.Z)
	}

	query := `
		INSERT INTO player_positions (user_id, x, y, layer) 
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE 
			x = VALUES(x), 
			y = VALUES(y), 
			layer = VALUES(layer),
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := r.db.ExecContext(ctx, query, userID, pos.X, pos.Y, pos.Z)
	if err != nil {
		return fmt.Errorf("ошибка сохранения позиции для пользователя %d: %w", userID, err)
	}

	return nil
}

// Load загружает позицию игрока из базы данных.
func (r *MariaPositionRepo) Load(ctx context.Context, userID uint64) (vec.Vec3, bool, error) {
	// Валидация входных данных
	if userID == 0 {
		return vec.Vec3{}, false, fmt.Errorf("недействительный userID: %d", userID)
	}

	query := `SELECT x, y, layer FROM player_positions WHERE user_id = ?`

	var pos vec.Vec3
	err := r.db.QueryRowContext(ctx, query, userID).Scan(&pos.X, &pos.Y, &pos.Z)

	if err == sql.ErrNoRows {
		// Позиция не найдена - первый вход пользователя
		return vec.Vec3{}, false, nil
	}

	if err != nil {
		return vec.Vec3{}, false, fmt.Errorf("ошибка загрузки позиции для пользователя %d: %w", userID, err)
	}

	return pos, true, nil
}

// Delete удаляет сохраненную позицию игрока.
func (r *MariaPositionRepo) Delete(ctx context.Context, userID uint64) error {
	// Валидация входных данных
	if userID == 0 {
		return fmt.Errorf("недействительный userID: %d", userID)
	}

	query := `DELETE FROM player_positions WHERE user_id = ?`

	result, err := r.db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("ошибка удаления позиции для пользователя %d: %w", userID, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("ошибка получения количества затронутых строк: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("позиция для пользователя %d не найдена", userID)
	}

	return nil
}

// BatchSave сохраняет позиции нескольких игроков в одной транзакции.
// Это оптимизация для автосохранения всех онлайн игроков.
func (r *MariaPositionRepo) BatchSave(ctx context.Context, positions map[uint64]vec.Vec3) error {
	if len(positions) == 0 {
		return nil // Нечего сохранять
	}

	// Начинаем транзакцию
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback() // Откат в случае ошибки

	// Подготавливаем запрос
	query := `
		INSERT INTO player_positions (user_id, x, y, layer) 
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE 
			x = VALUES(x), 
			y = VALUES(y), 
			layer = VALUES(layer),
			updated_at = CURRENT_TIMESTAMP
	`

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("ошибка подготовки запроса: %w", err)
	}
	defer stmt.Close()

	// Выполняем запросы для каждой позиции
	for userID, pos := range positions {
		// Валидация каждой записи
		if userID == 0 {
			return fmt.Errorf("недействительный userID в batch: %d", userID)
		}
		if pos.Z < 0 || pos.Z > 255 {
			return fmt.Errorf("недействительный layer для пользователя %d: %d", userID, pos.Z)
		}

		_, err = stmt.ExecContext(ctx, userID, pos.X, pos.Y, pos.Z)
		if err != nil {
			return fmt.Errorf("ошибка сохранения позиции для пользователя %d в batch: %w", userID, err)
		}
	}

	// Фиксируем транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ошибка фиксации транзакции: %w", err)
	}

	return nil
}

// Close закрывает соединение с базой данных.
func (r *MariaPositionRepo) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}
