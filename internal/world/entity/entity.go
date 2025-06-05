package entity

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// EntityType представляет тип сущности
type EntityType uint16

const (
	EntityTypePlayer EntityType = iota
	EntityTypeNPC
	EntityTypeMonster
	EntityTypeItem
	EntityTypeProjectile

	// Добавляем новые типы сущностей
	EntityTypeAnimal  // Животное
	EntityTypeVehicle // Транспорт
)

// AnimalType представляет подтипы животных
type AnimalType uint8

const (
	AnimalTypeCow AnimalType = iota
	AnimalTypeSheep
	AnimalTypeChicken
	AnimalTypePig
	AnimalTypeHorse
)

// MovementDirection представляет направление движения, отправляемое клиентом
type MovementDirection struct {
	Up    bool
	Right bool
	Down  bool
	Left  bool
}

// Entity представляет базовую сущность в мире
type Entity struct {
	ID         uint64                 // Уникальный идентификатор сущности
	Type       EntityType             // Тип сущности
	Position   vec.Vec2               // Текущая позиция в мире (в координатах блоков)
	PrecisePos vec.Vec2Float          // Точная позиция для плавного движения (субблоковая точность)
	Velocity   vec.Vec2Float          // Текущая скорость
	Size       vec.Vec2Float          // Размер хитбокса сущности
	Payload    map[string]interface{} // Дополнительные данные сущности
	Active     bool                   // Активна ли сущность
	Direction  int                    // Направление взгляда (0-3 или 0-7 для 8 направлений)
}

// NewEntity создаёт новую сущность
func NewEntity(id uint64, entityType EntityType, position vec.Vec2) *Entity {
	return &Entity{
		ID:         id,
		Type:       entityType,
		Position:   position,
		PrecisePos: vec.Vec2Float{X: float64(position.X), Y: float64(position.Y)},
		Velocity:   vec.Vec2Float{X: 0, Y: 0},
		Size:       vec.Vec2Float{X: 0.8, Y: 0.8}, // Стандартный размер для вида сверху
		Payload:    make(map[string]interface{}),
		Active:     true,
		Direction:  0, // По умолчанию смотрит вниз (юг)
	}
}

// EntityBehavior определяет поведение сущности
type EntityBehavior interface {
	// Update обновляет состояние сущности
	Update(api EntityAPI, entity *Entity, dt float64)

	// OnSpawn вызывается при создании сущности
	OnSpawn(api EntityAPI, entity *Entity)

	// OnDespawn вызывается при удалении сущности
	OnDespawn(api EntityAPI, entity *Entity)

	// OnDamage вызывается при получении урона
	OnDamage(api EntityAPI, entity *Entity, damage int, source interface{}) bool

	// OnCollision вызывается при столкновении с другой сущностью или блоком
	OnCollision(api EntityAPI, entity *Entity, other interface{}, collisionPoint vec.Vec2Float)

	// GetMoveSpeed возвращает скорость движения сущности
	GetMoveSpeed() float64
}

// EntityAPI предоставляет интерфейс для взаимодействия сущностей с миром
type EntityAPI interface {
	// GetBlock возвращает блок по координатам
	GetBlock(pos vec.Vec2) block.BlockID

	// SetBlock устанавливает блок по координатам
	SetBlock(pos vec.Vec2, id block.BlockID)

	// GetBlockMetadata получает метаданные блока
	GetBlockMetadata(pos vec.Vec2, key string) interface{}

	// SetBlockMetadata устанавливает метаданные блока
	SetBlockMetadata(pos vec.Vec2, key string, value interface{})

	// GetEntitiesInRange возвращает сущности в указанном радиусе
	GetEntitiesInRange(center vec.Vec2, radius float64) []*Entity

	// SpawnEntity создает новую сущность
	SpawnEntity(entityType EntityType, position vec.Vec2) uint64

	// DespawnEntity удаляет сущность
	DespawnEntity(entityID uint64)

	// MoveEntity перемещает сущность, проверяя коллизии
	MoveEntity(entity *Entity, direction MovementDirection, dt float64) bool

	// SendMessage отправляет сообщение сущности
	SendMessage(entityID uint64, messageType string, data interface{})

	// GetBehavior возвращает поведение для типа сущности
	GetBehavior(entityType EntityType) (EntityBehavior, bool)
}
