package app

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/dgraph-io/badger/v3"
)

// EntityData содержит данные о сущности для хранения
type EntityData struct {
	ID       uint64                 `json:"id"`       // Уникальный ID сущности
	Type     uint16                 `json:"type"`     // Тип сущности
	Position vec.Vec2               `json:"position"` // Позиция в мире
	Payload  map[string]interface{} `json:"payload"`  // Метаданные сущности
}

// BigChunkEntities содержит информацию о сущностях в BigChunk
type BigChunkEntities struct {
	Coords   vec.Vec2              `json:"coords"`   // Координаты BigChunk
	Entities map[uint64]EntityData `json:"entities"` // Карта сущностей по ID
}

// EntityStorage управляет хранением сущностей в BadgerDB
type EntityStorage struct {
	db     *badger.DB
	dbPath string
	mutex  sync.RWMutex
	ready  bool
}

// NewEntityStorage создает новое хранилище сущностей
func NewEntityStorage(dataPath string) (*EntityStorage, error) {
	storage := &EntityStorage{
		dbPath: filepath.Join(dataPath, "entities"),
	}

	opts := badger.DefaultOptions(storage.dbPath)
	opts.Logger = nil // Отключаем логирование BadgerDB

	var err error
	storage.db, err = badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть BadgerDB: %w", err)
	}

	storage.ready = true
	return storage, nil
}

// SaveEntities сохраняет данные о сущностях из BigChunk
func (s *EntityStorage) SaveEntities(bigChunkCoords vec.Vec2, entities map[uint64]interface{}) error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if !s.ready {
		return fmt.Errorf("хранилище не готово")
	}

	// Создаем структуру для сохранения
	delta := BigChunkEntities{
		Coords:   bigChunkCoords,
		Entities: make(map[uint64]EntityData),
	}

	// Преобразуем сущности в формат для сохранения
	for id, entity := range entities {
		if entityMap, ok := entity.(map[string]interface{}); ok {
			// Получаем значения из карты
			position := vec.Vec2{X: 0, Y: 0}
			if posMap, ok := entityMap["Position"].(map[string]interface{}); ok {
				if x, ok := posMap["X"].(int); ok {
					position.X = x
				}
				if y, ok := posMap["Y"].(int); ok {
					position.Y = y
				}
			}

			entityType := uint16(0)
			if typeVal, ok := entityMap["Type"].(uint16); ok {
				entityType = typeVal
			}

			payload := make(map[string]interface{})
			if pl, ok := entityMap["Metadata"].(map[string]interface{}); ok {
				payload = pl
			}

			delta.Entities[id] = EntityData{
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
	err = s.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), data)
	})

	if err != nil {
		return fmt.Errorf("ошибка сохранения сущностей в BadgerDB: %w", err)
	}

	return nil
}

// LoadEntities загружает данные о сущностях для BigChunk
func (s *EntityStorage) LoadEntities(bigChunkCoords vec.Vec2) (*BigChunkEntities, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	if !s.ready {
		return nil, fmt.Errorf("хранилище не готово")
	}

	key := fmt.Sprintf("entities:%d:%d", bigChunkCoords.X, bigChunkCoords.Y)
	var data []byte

	// Читаем данные из BadgerDB
	err := s.db.View(func(txn *badger.Txn) error {
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
			Entities: make(map[uint64]EntityData),
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

// ApplyEntityDataToMap применяет загруженные данные о сущностях к карте сущностей
func (s *EntityStorage) ApplyEntityDataToMap(entities map[uint64]interface{}, data *BigChunkEntities) {
	if data == nil || len(data.Entities) == 0 {
		return
	}

	// Применяем данные к карте сущностей
	for id, entityDelta := range data.Entities {
		// Создаем карту данных для сущности
		entityData := map[string]interface{}{
			"ID":       entityDelta.ID,
			"Type":     entityDelta.Type,
			"Position": entityDelta.Position,
			"Metadata": entityDelta.Payload,
		}

		// Добавляем в карту сущностей
		entities[id] = entityData
	}

	log.Printf("Загружено %d сущностей для BigChunk %v", len(data.Entities), data.Coords)
}

// Close закрывает хранилище
func (s *EntityStorage) Close() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.ready {
		return nil
	}

	s.ready = false
	return s.db.Close()
}
