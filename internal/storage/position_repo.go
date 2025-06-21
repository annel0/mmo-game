package storage

import (
	"context"

	"github.com/annel0/mmo-game/internal/vec"
)

// PositionRepo определяет интерфейс для сохранения и загрузки позиций игроков.
// Позиции привязаны к UserID (постоянный идентификатор аккаунта), а не к EntityID.
// Это позволяет сохранять позицию между сессиями игры.
type PositionRepo interface {
	// Save сохраняет позицию игрока в хранилище.
	// Параметры:
	//   ctx - контекст для отмены операции
	//   userID - уникальный идентификатор пользователя
	//   pos - позиция в 3D пространстве (x, y, layer)
	// Возвращает:
	//   error - ошибка при сохранении
	Save(ctx context.Context, userID uint64, pos vec.Vec3) error

	// Load загружает позицию игрока из хранилища.
	// Параметры:
	//   ctx - контекст для отмены операции
	//   userID - уникальный идентификатор пользователя
	// Возвращает:
	//   vec.Vec3 - позиция игрока
	//   bool - true если позиция найдена, false если первый вход
	//   error - ошибка при загрузке
	Load(ctx context.Context, userID uint64) (vec.Vec3, bool, error)

	// Delete удаляет сохраненную позицию игрока (для тестов или сброса).
	// Параметры:
	//   ctx - контекст для отмены операции
	//   userID - уникальный идентификатор пользователя
	// Возвращает:
	//   error - ошибка при удалении
	Delete(ctx context.Context, userID uint64) error

	// BatchSave сохраняет позиции нескольких игроков одновременно (для автосохранения).
	// Параметры:
	//   ctx - контекст для отмены операции
	//   positions - карта userID -> позиция
	// Возвращает:
	//   error - ошибка при сохранении
	BatchSave(ctx context.Context, positions map[uint64]vec.Vec3) error
}
