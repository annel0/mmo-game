package world

import (
	"github.com/annel0/mmo-game/internal/vec"
)

// EntityType определяет тип сущности
type EntityType uint16

const (
	EntityTypeUnknown    EntityType = 0   // Неизвестный тип
	EntityTypePlayer     EntityType = 1   // Игрок
	EntityTypeObject     EntityType = 100 // Статический объект
	EntityTypeProjectile EntityType = 200 // Снаряд
	EntityTypeNPC        EntityType = 300 // Неигровой персонаж
	EntityTypeAnimal     EntityType = 301 // Животное
	EntityTypeMonster    EntityType = 302 // Монстр
	EntityTypeEffect     EntityType = 400 // Визуальный эффект
)

// WorldEntityData представляет базовую информацию о сущности мира
type WorldEntityData struct {
	ID       uint64                 // Уникальный ID сущности
	Type     uint16                 // Тип сущности
	Position vec.Vec2               // Позиция в мире
	Metadata map[string]interface{} // Метаданные сущности
}

// Entity представляет собой интерфейс для всех игровых сущностей
type Entity interface {
	GetID() uint64              // Получить уникальный ID
	GetType() EntityType        // Получить тип сущности
	GetPosition() vec.Vec2      // Получить текущую позицию
	SetPosition(vec.Vec2)       // Установить новую позицию
	Update(float64)             // Обновить состояние (delta в секундах)
	Serialize() WorldEntityData // Сериализовать для сохранения/отправки
}
