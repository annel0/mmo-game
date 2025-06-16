package storage

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/block"
	"github.com/dgraph-io/badger/v3"
)

// WorldStorage представляет собой хранилище данных мира
type WorldStorage struct {
	db      *badger.DB
	dbPath  string
	mutex   sync.RWMutex
	isReady bool
}

// ChunkDelta содержит изменения в чанке
type ChunkDelta struct {
	Coords      vec.Vec2              `json:"coords"`
	BlockDeltas map[string]BlockDelta `json:"blocks"` // Ключ - упакованные координаты "x:y"
}

// BlockDelta содержит изменения блока
type BlockDelta struct {
	ID      block.BlockID          `json:"id"`
	Payload map[string]interface{} `json:"payload,omitempty"`
}

// EntityDelta содержит данные о сущности для сохранения
type EntityDelta struct {
	ID       uint64                 `json:"id"`       // Уникальный ID сущности
	Type     uint16                 `json:"type"`     // Тип сущности
	Position vec.Vec2               `json:"position"` // Позиция в мире
	Payload  map[string]interface{} `json:"payload"`  // Метаданные сущности
}

// BigChunkEntities содержит информацию о сущностях в BigChunk
type BigChunkEntities struct {
	Coords   vec.Vec2               `json:"coords"`   // Координаты BigChunk
	Entities map[uint64]EntityDelta `json:"entities"` // Карта сущностей по ID
}

// NewWorldStorage создает новое хранилище мира
func NewWorldStorage(dataPath string) (*WorldStorage, error) {
	dbPath := filepath.Join(dataPath, "world")
	opts := badger.DefaultOptions(dbPath)
	opts.Logger = nil // Отключаем логирование BadgerDB

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть BadgerDB: %w", err)
	}

	return &WorldStorage{
		db:      db,
		dbPath:  dbPath,
		isReady: true,
	}, nil
}

// Close закрывает хранилище данных
func (ws *WorldStorage) Close() error {
	ws.mutex.Lock()
	defer ws.mutex.Unlock()

	if !ws.isReady {
		return nil
	}

	ws.isReady = false
	return ws.db.Close()
}

// SaveChunk сохраняет изменения в чанке
func (ws *WorldStorage) SaveChunk(chunk *world.Chunk) error {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	if !ws.isReady {
		return fmt.Errorf("хранилище не готово")
	}

	// Если нет изменений, пропускаем
	chunk.Mu.RLock()
	if chunk.ChangeCounter == 0 {
		chunk.Mu.RUnlock()
		return nil
	}

	// Создаем дельту изменений
	delta := ChunkDelta{
		Coords:      chunk.Coords,
		BlockDeltas: make(map[string]BlockDelta),
	}

	// Добавляем изменения блоков из Changes3D
	for coord := range chunk.Changes3D {
		// Обрабатываем только изменения в слое ACTIVE
		if coord.Layer == world.LayerActive {
			blockID := chunk.GetBlockLayer(coord.Layer, coord.Pos)
			metadata := chunk.GetBlockMetadataLayer(coord.Layer, coord.Pos)

			key := fmt.Sprintf("%d:%d", coord.Pos.X, coord.Pos.Y)
			delta.BlockDeltas[key] = BlockDelta{
				ID:      blockID,
				Payload: metadata,
			}
		}
	}
	chunk.Mu.RUnlock()

	// Сериализуем дельту в JSON
	data, err := json.Marshal(delta)
	if err != nil {
		return fmt.Errorf("ошибка сериализации дельты: %w", err)
	}

	// Создаем ключ для BadgerDB
	key := fmt.Sprintf("chunk:%d:%d", delta.Coords.X, delta.Coords.Y)

	// Сохраняем в BadgerDB
	err = ws.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})

	if err != nil {
		return fmt.Errorf("ошибка сохранения в BadgerDB: %w", err)
	}

	// Очищаем список изменений в чанке
	chunk.ClearChanges()

	return nil
}

// LoadChunk загружает дельту чанка
func (ws *WorldStorage) LoadChunk(coords vec.Vec2) (*ChunkDelta, error) {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	if !ws.isReady {
		return nil, fmt.Errorf("хранилище не готово")
	}

	key := fmt.Sprintf("chunk:%d:%d", coords.X, coords.Y)
	var data []byte

	// Читаем данные из BadgerDB
	err := ws.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			data = append([]byte{}, val...)
			return nil
		})
	})

	// Если чанк не найден, возвращаем пустую дельту
	if err == badger.ErrKeyNotFound {
		return &ChunkDelta{
			Coords:      coords,
			BlockDeltas: make(map[string]BlockDelta),
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка чтения из BadgerDB: %w", err)
	}

	// Десериализуем данные
	var delta ChunkDelta
	if err := json.Unmarshal(data, &delta); err != nil {
		return nil, fmt.Errorf("ошибка десериализации дельты: %w", err)
	}

	return &delta, nil
}

// ApplyDeltaToChunk применяет дельту к чанку
func (ws *WorldStorage) ApplyDeltaToChunk(chunk *world.Chunk, delta *ChunkDelta) error {
	if delta == nil || len(delta.BlockDeltas) == 0 {
		return nil
	}

	chunk.Mu.Lock()
	defer chunk.Mu.Unlock()

	// Применяем изменения к чанку
	for key, blockDelta := range delta.BlockDeltas {
		var x, y int
		if _, err := fmt.Sscanf(key, "%d:%d", &x, &y); err != nil {
			log.Printf("Ошибка парсинга ключа '%s': %v", key, err)
			continue
		}

		// Проверяем корректность координат
		if x < 0 || x >= 16 || y < 0 || y >= 16 {
			log.Printf("Некорректные координаты: %d,%d", x, y)
			continue
		}

		// Устанавливаем блок
		pos := vec.Vec2{X: x, Y: y}
		chunk.SetBlock(pos, blockDelta.ID)

		// Устанавливаем метаданные, если они есть
		if len(blockDelta.Payload) > 0 {
			chunk.SetBlockMetadataMap(pos, blockDelta.Payload)
		}
	}

	return nil
}

// SaveBigChunk сохраняет все чанки в BigChunk
func (ws *WorldStorage) SaveBigChunk(bigChunk *world.BigChunk) error {
	// Так как поля BigChunk приватные, нам нужно использовать его методы
	// или экспортировать метод для получения всех чанков

	// Временное решение - сейчас нет публичного метода для получения чанков
	// Поэтому эту функцию нужно будет вызывать изнутри самого BigChunk
	// в реализации метода saveState()
	return nil
}

// LoadAndApplyChunk загружает и применяет дельту чанка
func (ws *WorldStorage) LoadAndApplyChunk(chunk *world.Chunk) error {
	delta, err := ws.LoadChunk(chunk.Coords)
	if err != nil {
		return err
	}

	return ws.ApplyDeltaToChunk(chunk, delta)
}

// SaveEntities сохраняет данные о сущностях из BigChunk
func (ws *WorldStorage) SaveEntities(bigChunkCoords vec.Vec2, entities map[uint64]interface{}) error {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	if !ws.isReady {
		return fmt.Errorf("хранилище не готово")
	}

	// Создаем структуру для сохранения
	delta := BigChunkEntities{
		Coords:   bigChunkCoords,
		Entities: make(map[uint64]EntityDelta),
	}

	// Преобразуем сущности в формат для сохранения
	for id, entity := range entities {
		// Обрабатываем разные типы EntityData
		if entityData, ok := entity.(world.EntityData); ok {
			delta.Entities[id] = EntityDelta{
				ID:       entityData.ID,
				Type:     entityData.Type,
				Position: entityData.Position,
				Payload:  entityData.Metadata,
			}
		} else if entityMap, ok := entity.(map[string]interface{}); ok {
			// Альтернативный формат из карты
			position := vec.Vec2{X: 0, Y: 0}
			if pos, ok := entityMap["position"].(vec.Vec2); ok {
				position = pos
			}

			entityType := uint16(0)
			if typeVal, ok := entityMap["type"].(uint16); ok {
				entityType = typeVal
			}

			payload := make(map[string]interface{})
			if pl, ok := entityMap["payload"].(map[string]interface{}); ok {
				payload = pl
			}

			delta.Entities[id] = EntityDelta{
				ID:       id,
				Type:     entityType,
				Position: position,
				Payload:  payload,
			}
		}
	}

	// Если нет сущностей для сохранения, пропускаем
	if len(delta.Entities) == 0 {
		return nil
	}

	// Сериализуем в JSON
	data, err := json.Marshal(delta)
	if err != nil {
		return fmt.Errorf("ошибка сериализации сущностей: %w", err)
	}

	// Создаем ключ для BadgerDB
	key := fmt.Sprintf("entities:%d:%d", bigChunkCoords.X, bigChunkCoords.Y)

	// Сохраняем в BadgerDB
	err = ws.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})

	if err != nil {
		return fmt.Errorf("ошибка сохранения сущностей в BadgerDB: %w", err)
	}

	return nil
}

// LoadEntities загружает данные о сущностях для BigChunk
func (ws *WorldStorage) LoadEntities(bigChunkCoords vec.Vec2) (*BigChunkEntities, error) {
	ws.mutex.RLock()
	defer ws.mutex.RUnlock()

	if !ws.isReady {
		return nil, fmt.Errorf("хранилище не готово")
	}

	key := fmt.Sprintf("entities:%d:%d", bigChunkCoords.X, bigChunkCoords.Y)
	var data []byte

	// Читаем данные из BadgerDB
	err := ws.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}

		return item.Value(func(val []byte) error {
			data = append([]byte{}, val...)
			return nil
		})
	})

	// Если сущности не найдены, возвращаем пустую структуру
	if err == badger.ErrKeyNotFound {
		return &BigChunkEntities{
			Coords:   bigChunkCoords,
			Entities: make(map[uint64]EntityDelta),
		}, nil
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка чтения сущностей из BadgerDB: %w", err)
	}

	// Десериализуем данные
	var delta BigChunkEntities
	if err := json.Unmarshal(data, &delta); err != nil {
		return nil, fmt.Errorf("ошибка десериализации сущностей: %w", err)
	}

	return &delta, nil
}

// ApplyEntityDeltaToBigChunk применяет загруженные данные о сущностях к BigChunk
func (ws *WorldStorage) ApplyEntityDeltaToBigChunk(entities map[uint64]interface{}, delta *BigChunkEntities) {
	if delta == nil || len(delta.Entities) == 0 {
		return
	}

	// Применяем данные к карте сущностей
	for id, entityDelta := range delta.Entities {
		// Создаем экземпляр EntityData
		entityData := world.EntityData{
			ID:       entityDelta.ID,
			Type:     entityDelta.Type,
			Position: entityDelta.Position,
			Metadata: entityDelta.Payload,
		}

		// Добавляем в карту сущностей
		entities[id] = entityData
	}
}
