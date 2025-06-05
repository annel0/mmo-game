package world

import (
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// Chunk представляет участок мира размером 16x16 блоков
type Chunk struct {
	Coords        vec.Vec2                            // Координаты чанка в мире
	Blocks        [16][16]block.BlockID               // Матрица блоков
	Metadata      map[vec.Vec2]map[string]interface{} // Метаданные блоков
	Changes       map[vec.Vec2]struct{}               // Измененные блоки
	Tickable      map[vec.Vec2]struct{}               // Тикаемые блоки
	ChangeCounter int                                 // Счетчик изменений
	Mu            sync.RWMutex                        // Мьютекс для безопасного доступа
}

// NewChunk создаёт новый чанк с указанными координатами
func NewChunk(coords vec.Vec2) *Chunk {
	return &Chunk{
		Coords:        coords,
		Changes:       make(map[vec.Vec2]struct{}),
		Metadata:      make(map[vec.Vec2]map[string]interface{}),
		Blocks:        [16][16]block.BlockID{},
		Mu:            sync.RWMutex{},
		ChangeCounter: 0,
		Tickable:      make(map[vec.Vec2]struct{}),
	}
}

// SetBlockMetadata устанавливает метаданные для блока
func (c *Chunk) SetBlockMetadata(local vec.Vec2, key string, value interface{}) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	if _, exists := c.Metadata[local]; !exists {
		c.Metadata[local] = make(map[string]interface{})
	}

	c.Metadata[local][key] = value
	c.Changes[local] = struct{}{}
	c.ChangeCounter++
}

// SetBlockMetadataMap устанавливает несколько метаданных для блока
func (c *Chunk) SetBlockMetadataMap(local vec.Vec2, metadata map[string]interface{}) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	if _, exists := c.Metadata[local]; !exists {
		c.Metadata[local] = make(map[string]interface{})
	}

	// Копируем все ключи из переданной карты
	for key, value := range metadata {
		c.Metadata[local][key] = value
	}

	c.Changes[local] = struct{}{}
	c.ChangeCounter++
}

// GetBlockMetadata возвращает метаданные для блока
func (c *Chunk) GetBlockMetadata(local vec.Vec2) map[string]interface{} {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if metadata, exists := c.Metadata[local]; exists {
		// Создаем копию метаданных
		result := make(map[string]interface{}, len(metadata))
		for k, v := range metadata {
			result[k] = v
		}
		return result
	}

	return make(map[string]interface{})
}

// GetBlockMetadataValue возвращает конкретное значение метаданных
func (c *Chunk) GetBlockMetadataValue(local vec.Vec2, key string) (interface{}, bool) {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if metadata, exists := c.Metadata[local]; exists {
		if value, ok := metadata[key]; ok {
			return value, true
		}
	}

	return nil, false
}

// toLocalCoords преобразует глобальные координаты в локальные для чанка
func (c *Chunk) toLocalCoords(pos vec.Vec2) vec.Vec2 {
	return pos.LocalInChunk()
}

// GetBlockID возвращает ID блока по локальным координатам
func (c *Chunk) GetBlockID(local vec.Vec2) block.BlockID {
	return c.GetBlock(local)
}

// SetBlock устанавливает блок по локальным координатам
func (c *Chunk) SetBlock(local vec.Vec2, blockID block.BlockID) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	// Получаем поведение блока
	behavior, exists := block.Get(blockID)

	// Обновляем блок
	c.Blocks[local.X][local.Y] = blockID
	c.Changes[local] = struct{}{}
	c.ChangeCounter++

	// Обновляем тикаемые блоки
	if exists && behavior.NeedsTick() {
		c.Tickable[local] = struct{}{}
	} else {
		delete(c.Tickable, local)
	}
}

// SetBlockWithBehavior устанавливает блок по локальным координатам с поведением
func (c *Chunk) SetBlockWithBehavior(local vec.Vec2, behavior block.BlockBehavior) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	// Обновляем блок
	c.Blocks[local.X][local.Y] = behavior.ID()
	c.Changes[local] = struct{}{}
	c.ChangeCounter++

	// Обновляем тикаемые блоки
	if behavior.NeedsTick() {
		c.Tickable[local] = struct{}{}
	} else {
		delete(c.Tickable, local)
	}

	// Создаем метаданные для блока
	if _, exists := c.Metadata[local]; !exists {
		c.Metadata[local] = behavior.CreateMetadata()
	}
}

// GetBlock возвращает ID блока по локальным координатам
func (c *Chunk) GetBlock(local vec.Vec2) block.BlockID {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	return c.Blocks[local.X][local.Y]
}

// HasChanges возвращает true, если в чанке есть изменения
func (c *Chunk) HasChanges() bool {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	return c.ChangeCounter > 0
}

// ClearChanges очищает список изменений
func (c *Chunk) ClearChanges() {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.Changes = make(map[vec.Vec2]struct{})
	c.ChangeCounter = 0
}
