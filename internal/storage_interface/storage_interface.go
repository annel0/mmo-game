package storage_interface

import (
	"github.com/annel0/mmo-game/internal/vec"
)

// StorageProvider определяет интерфейс для взаимодействия с хранилищем
type StorageProvider interface {
	// SaveEntities сохраняет данные о сущностях из BigChunk
	SaveEntities(bigChunkCoords vec.Vec2, entities map[uint64]interface{}) error

	// LoadEntities загружает данные о сущностях для BigChunk
	LoadEntities(bigChunkCoords vec.Vec2) (*EntitiesData, error)

	// ApplyEntitiesToBigChunk применяет загруженные данные о сущностей к BigChunk
	ApplyEntitiesToBigChunk(entities map[uint64]interface{}, data *EntitiesData)

	// Close закрывает хранилище
	Close() error
}

// EntitiesData содержит информацию о сущностях, загруженных из хранилища
type EntitiesData struct {
	Coords   vec.Vec2
	Entities map[uint64]EntityStorageData
}

// EntityStorageData содержит данные о сущности для хранения
type EntityStorageData struct {
	ID       uint64                 // Уникальный ID сущности
	Type     uint16                 // Тип сущности
	Position vec.Vec2               // Позиция в мире
	Payload  map[string]interface{} // Метаданные сущности
}
