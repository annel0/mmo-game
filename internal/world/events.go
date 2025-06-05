package world

import (
	"github.com/annel0/mmo-game/internal/vec"
)

// EventType определяет тип события
type EventType uint8

const (
	EventTypeBlockSet       EventType = iota // Установка блока
	EventTypeBlockGet                        // Получение информации о блоке
	EventTypeBlockChange                     // Изменение блока
	EventTypeBlockInteract                   // Взаимодействие с блоком
	EventTypeEntityInteract                  // Взаимодействие с сущностью
	EventTypeEntitySpawn                     // Создание сущности
	EventTypeEntityMove                      // Перемещение сущности
	EventTypeEntityAction                    // Действие сущности
	EventTypeEntityDespawn                   // Удаление сущности
	EventTypeTick                            // Игровой тик
	EventTypeSave                            // Сохранение
)

// Event представляет собой интерфейс для всех событий
type Event interface {
	GetType() EventType
}

// BlockEvent представляет событие, связанное с блоком
type BlockEvent struct {
	EventType   EventType
	Position    vec.Vec2    // Мировые координаты блока
	Block       Block       // Блок
	SourceChunk vec.Vec2    // Координаты исходного чанка
	TargetChunk vec.Vec2    // Координаты целевого чанка
	Data        interface{} // Дополнительные данные события
}

// GetType возвращает тип события
func (e BlockEvent) GetType() EventType {
	return e.EventType
}

// EntityEvent представляет событие, связанное с сущностью
type EntityEvent struct {
	EventType   EventType
	EntityID    uint64      // Идентификатор сущности
	Position    vec.Vec2    // Мировые координаты сущности
	SourceChunk vec.Vec2    // Координаты исходного чанка
	TargetChunk vec.Vec2    // Координаты целевого чанка
	Data        interface{} // Дополнительные данные события
}

// GetType возвращает тип события
func (e EntityEvent) GetType() EventType {
	return e.EventType
}

// SaveEvent представляет событие сохранения чанка
type SaveEvent struct {
	Forced bool     // Принудительное сохранение
	Chunks []*Chunk // Чанки для сохранения, если есть
}

// GetType возвращает тип события
func (e SaveEvent) GetType() EventType {
	return EventTypeSave
}

// TickEvent представляет событие игрового тика
type TickEvent struct {
	TickID    uint64  // Номер тика
	DeltaTime float64 // Время, прошедшее с предыдущего тика (в секундах)
}

// GetType возвращает тип события
func (e TickEvent) GetType() EventType {
	return EventTypeTick
}

// EntitySaveEvent представляет событие сохранения сущностей
type EntitySaveEvent struct {
	BigChunkCoords vec.Vec2               // Координаты BigChunk
	Entities       map[uint64]interface{} // Сущности для сохранения
}

// GetType возвращает тип события
func (e EntitySaveEvent) GetType() EventType {
	return EventTypeSave
}
