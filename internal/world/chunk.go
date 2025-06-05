package world

import (
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// Chunk представляет участок мира размером 16x16 блоков
type Chunk struct {
	Coords vec.Vec2 // Координаты чанка в мире

	// Blocks сохраняет слой ACTIVE для обратной совместимости.
	// Новые функции должны использовать Blocks3D или методы Get/SetBlockLayer.
	Blocks [16][16]block.BlockID // DEPRECATED: однослойная матрица

	// Blocks3D[layer][x][y]
	Blocks3D [MaxLayers][16][16]block.BlockID

	// DEPRECATED поля для совместимости
	Metadata map[vec.Vec2]map[string]interface{} // Метаданные слоя ACTIVE (deprecated)
	Changes  map[vec.Vec2]struct{}               // Измененные блоки в ACTIVE (deprecated)
	Tickable map[vec.Vec2]struct{}               // Тикаемые блоки в ACTIVE (deprecated)

	// Новые карты, индексируемые по BlockCoord (layer+pos)
	Metadata3D map[BlockCoord]map[string]interface{}
	Changes3D  map[BlockCoord]struct{}
	Tickable3D map[BlockCoord]struct{}

	ChangeCounter int          // Счетчик изменений
	Mu            sync.RWMutex // Мьютекс для безопасного доступа
}

// NewChunk создаёт новый чанк с указанными координатами
func NewChunk(coords vec.Vec2) *Chunk {
	return &Chunk{
		Coords:        coords,
		Changes:       make(map[vec.Vec2]struct{}),
		Metadata:      make(map[vec.Vec2]map[string]interface{}),
		Metadata3D:    make(map[BlockCoord]map[string]interface{}),
		Changes3D:     make(map[BlockCoord]struct{}),
		Tickable3D:    make(map[BlockCoord]struct{}),
		Mu:            sync.RWMutex{},
		ChangeCounter: 0,
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

	if metadata, exists := c.Metadata3D[BlockCoord{Layer: LayerActive, Pos: local}]; exists {
		// Создаем копию метаданных
		result := make(map[string]interface{}, len(metadata))
		for k, v := range metadata {
			result[k] = v
		}
		return result
	} else if metadata, exists := c.Metadata[local]; exists {
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

	if metadata, exists := c.Metadata3D[BlockCoord{Layer: LayerActive, Pos: local}]; exists {
		if value, ok := metadata[key]; ok {
			return value, true
		}
	} else if metadata, exists := c.Metadata[local]; exists {
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
	c.Blocks3D[LayerActive][local.X][local.Y] = blockID
	c.Changes[local] = struct{}{}
	c.ChangeCounter++

	// Обновляем тикаемые блоки
	if exists && behavior.NeedsTick() {
		c.Tickable3D[BlockCoord{Layer: LayerActive, Pos: local}] = struct{}{}
	} else {
		delete(c.Tickable3D, BlockCoord{Layer: LayerActive, Pos: local})
	}
}

// SetBlockWithBehavior устанавливает блок по локальным координатам с поведением
func (c *Chunk) SetBlockWithBehavior(local vec.Vec2, behavior block.BlockBehavior) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	// Обновляем блок
	c.Blocks[local.X][local.Y] = behavior.ID()
	c.Blocks3D[LayerActive][local.X][local.Y] = behavior.ID()
	c.Changes[local] = struct{}{}
	c.ChangeCounter++

	// Обновляем тикаемые блоки
	if behavior.NeedsTick() {
		c.Tickable3D[BlockCoord{Layer: LayerActive, Pos: local}] = struct{}{}
	} else {
		delete(c.Tickable3D, BlockCoord{Layer: LayerActive, Pos: local})
	}

	// Создаем метаданные для блока
	if _, exists := c.Metadata3D[BlockCoord{Layer: LayerActive, Pos: local}]; !exists {
		c.Metadata3D[BlockCoord{Layer: LayerActive, Pos: local}] = behavior.CreateMetadata()
	}
}

// GetBlock возвращает ID блока по локальным координатам
func (c *Chunk) GetBlock(local vec.Vec2) block.BlockID {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	id := c.Blocks3D[LayerActive][local.X][local.Y]
	if id == 0 { // fallback для данных, ещё не перенесённых в 3D-массив
		id = c.Blocks[local.X][local.Y]
	}
	return id
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

// GetBlockLayer возвращает ID блока на конкретном слое.
func (c *Chunk) GetBlockLayer(layer BlockLayer, local vec.Vec2) block.BlockID {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if layer >= MaxLayers {
		return 0
	}
	id := c.Blocks3D[layer][local.X][local.Y]
	if layer == LayerActive && id == 0 {
		id = c.Blocks[local.X][local.Y]
	}
	return id
}

// SetBlockLayer устанавливает блок на указанном слое.
func (c *Chunk) SetBlockLayer(layer BlockLayer, local vec.Vec2, blockID block.BlockID) {
	if layer >= MaxLayers {
		return
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.Blocks3D[layer][local.X][local.Y] = blockID

	// Для совместимости: если изменяем ACTIVE, дублируем в 2-D поле/тик-карты.
	if layer == LayerActive {
		c.Blocks[local.X][local.Y] = blockID
	}

	c.Changes[local] = struct{}{}
	c.ChangeCounter++
}

// SetBlockMetadataLayer устанавливает метаданные блока в заданном слое.
func (c *Chunk) SetBlockMetadataLayer(layer BlockLayer, local vec.Vec2, key string, value interface{}) {
	coord := BlockCoord{Layer: layer, Pos: local}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	if _, exists := c.Metadata3D[coord]; !exists {
		c.Metadata3D[coord] = make(map[string]interface{})
	}
	c.Metadata3D[coord][key] = value
	c.Changes3D[coord] = struct{}{}

	// Совместимость: если слой ACTIVE, дублируем
	if layer == LayerActive {
		if _, ok := c.Metadata[local]; !ok {
			c.Metadata[local] = make(map[string]interface{})
		}
		c.Metadata[local][key] = value
		c.Changes[local] = struct{}{}
	}
}

// GetBlockMetadataLayer возвращает метаданные блока на указанном слое.
func (c *Chunk) GetBlockMetadataLayer(layer BlockLayer, local vec.Vec2) map[string]interface{} {
	coord := BlockCoord{Layer: layer, Pos: local}

	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if meta, ok := c.Metadata3D[coord]; ok {
		// copy
		m := make(map[string]interface{}, len(meta))
		for k, v := range meta {
			m[k] = v
		}
		return m
	}

	if layer == LayerActive {
		if meta, ok := c.Metadata[local]; ok {
			m := make(map[string]interface{}, len(meta))
			for k, v := range meta {
				m[k] = v
			}
			return m
		}
	}
	return make(map[string]interface{})
}
