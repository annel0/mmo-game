package storage_adapter

import (
	"github.com/annel0/mmo-game/internal/storage"
	"github.com/annel0/mmo-game/internal/storage_interface"
	"github.com/annel0/mmo-game/internal/vec"
)

// WorldStorageAdapter адаптирует WorldStorage для использования интерфейса StorageProvider
type WorldStorageAdapter struct {
	storage *storage.WorldStorage
}

// NewStorageProvider создает новый адаптер для WorldStorage
func NewStorageProvider(dataPath string) (storage_interface.StorageProvider, error) {
	ws, err := storage.NewWorldStorage(dataPath)
	if err != nil {
		return nil, err
	}

	return &WorldStorageAdapter{
		storage: ws,
	}, nil
}

// SaveEntities сохраняет данные о сущностях из BigChunk
func (a *WorldStorageAdapter) SaveEntities(bigChunkCoords vec.Vec2, entities map[uint64]interface{}) error {
	return a.storage.SaveEntities(bigChunkCoords, entities)
}

// LoadEntities загружает данные о сущностях для BigChunk и адаптирует их к интерфейсу
func (a *WorldStorageAdapter) LoadEntities(bigChunkCoords vec.Vec2) (*storage_interface.EntitiesData, error) {
	// Вызываем исходную функцию из WorldStorage
	entities, err := a.storage.LoadEntities(bigChunkCoords)
	if err != nil {
		return nil, err
	}

	// Адаптируем результат к формату EntitiesData
	result := &storage_interface.EntitiesData{
		Coords:   bigChunkCoords,
		Entities: make(map[uint64]storage_interface.EntityStorageData),
	}

	// Копируем данные сущностей
	for id, entity := range entities.Entities {
		result.Entities[id] = storage_interface.EntityStorageData{
			ID:       entity.ID,
			Type:     entity.Type,
			Position: entity.Position,
			Payload:  entity.Payload,
		}
	}

	return result, nil
}

// ApplyEntitiesToBigChunk применяет загруженные данные к карте сущностей BigChunk
func (a *WorldStorageAdapter) ApplyEntitiesToBigChunk(entities map[uint64]interface{}, data *storage_interface.EntitiesData) {
	if data == nil || len(data.Entities) == 0 {
		return
	}

	// Применяем данные к карте сущностей
	for id, entityData := range data.Entities {
		mapped := map[string]interface{}{
			"ID":       entityData.ID,
			"Type":     entityData.Type,
			"Position": entityData.Position,
			"Metadata": entityData.Payload,
		}

		// Добавляем в карту сущностей
		entities[id] = mapped
	}
}

// Close закрывает хранилище
func (a *WorldStorageAdapter) Close() error {
	return a.storage.Close()
}
