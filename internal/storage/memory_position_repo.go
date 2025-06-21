package storage

import (
	"context"
	"fmt"
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
)

// MemoryPositionRepo реализует PositionRepo в памяти.
// Используется как fallback, когда MariaDB недоступна,
// или для CI/локальной разработки без БД.
// ВНИМАНИЕ: Данные теряются при перезапуске сервера!
type MemoryPositionRepo struct {
	mu   sync.RWMutex
	data map[uint64]vec.Vec3 // userID -> позиция
}

// NewMemoryPositionRepo создает новый репозиторий позиций в памяти.
//
// Возвращает:
//
//	*MemoryPositionRepo - экземпляр репозитория
func NewMemoryPositionRepo() *MemoryPositionRepo {
	return &MemoryPositionRepo{
		data: make(map[uint64]vec.Vec3),
	}
}

// Save сохраняет позицию игрока в памяти.
func (r *MemoryPositionRepo) Save(ctx context.Context, userID uint64, pos vec.Vec3) error {
	// Валидация входных данных
	if userID == 0 {
		return fmt.Errorf("недействительный userID: %d", userID)
	}

	// Проверяем корректность layer (слоя)
	if pos.Z < 0 || pos.Z > 255 {
		return fmt.Errorf("недействительный layer: %d (должен быть 0-255)", pos.Z)
	}

	// Проверяем контекст на отмену
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.data[userID] = pos
	return nil
}

// Load загружает позицию игрока из памяти.
func (r *MemoryPositionRepo) Load(ctx context.Context, userID uint64) (vec.Vec3, bool, error) {
	// Валидация входных данных
	if userID == 0 {
		return vec.Vec3{}, false, fmt.Errorf("недействительный userID: %d", userID)
	}

	// Проверяем контекст на отмену
	select {
	case <-ctx.Done():
		return vec.Vec3{}, false, ctx.Err()
	default:
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	pos, exists := r.data[userID]
	return pos, exists, nil
}

// Delete удаляет сохраненную позицию игрока из памяти.
func (r *MemoryPositionRepo) Delete(ctx context.Context, userID uint64) error {
	// Валидация входных данных
	if userID == 0 {
		return fmt.Errorf("недействительный userID: %d", userID)
	}

	// Проверяем контекст на отмену
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.data[userID]; !exists {
		return fmt.Errorf("позиция для пользователя %d не найдена", userID)
	}

	delete(r.data, userID)
	return nil
}

// BatchSave сохраняет позиции нескольких игроков в памяти.
func (r *MemoryPositionRepo) BatchSave(ctx context.Context, positions map[uint64]vec.Vec3) error {
	if len(positions) == 0 {
		return nil // Нечего сохранять
	}

	// Проверяем контекст на отмену
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Валидация всех записей перед сохранением
	for userID, pos := range positions {
		if userID == 0 {
			return fmt.Errorf("недействительный userID в batch: %d", userID)
		}
		if pos.Z < 0 || pos.Z > 255 {
			return fmt.Errorf("недействительный layer для пользователя %d: %d", userID, pos.Z)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Сохраняем все позиции
	for userID, pos := range positions {
		r.data[userID] = pos
	}

	return nil
}

// GetAllPositions возвращает все сохраненные позиции (для отладки).
// Этот метод не входит в интерфейс PositionRepo, но полезен для тестирования.
func (r *MemoryPositionRepo) GetAllPositions() map[uint64]vec.Vec3 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Создаем копию карты для безопасности
	result := make(map[uint64]vec.Vec3, len(r.data))
	for userID, pos := range r.data {
		result[userID] = pos
	}

	return result
}

// Count возвращает количество сохраненных позиций (для отладки).
func (r *MemoryPositionRepo) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.data)
}

// Clear очищает все сохраненные позиции (для тестов).
func (r *MemoryPositionRepo) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = make(map[uint64]vec.Vec3)
}
