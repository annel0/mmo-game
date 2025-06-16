package world

import (
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// Chunk представляет участок мира размером 16x16 блоков
type Chunk struct {
	Coords vec.Vec2 // Координаты чанка в мире

	// Blocks3D[layer][x][y]
	Blocks3D [MaxLayers][16][16]block.BlockID

	// Карты, индексируемые по BlockCoord (layer+pos)
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
		Metadata3D:    make(map[BlockCoord]map[string]interface{}),
		Changes3D:     make(map[BlockCoord]struct{}),
		Tickable3D:    make(map[BlockCoord]struct{}),
		Mu:            sync.RWMutex{},
		ChangeCounter: 0,
	}
}

// SetBlockMetadata устанавливает метаданные для блока (слой ACTIVE)
func (c *Chunk) SetBlockMetadata(local vec.Vec2, key string, value interface{}) {
	c.SetBlockMetadataLayer(LayerActive, local, key, value)
}

// SetBlockMetadataMap устанавливает несколько метаданных для блока (слой ACTIVE)
func (c *Chunk) SetBlockMetadataMap(local vec.Vec2, metadata map[string]interface{}) {
	for key, value := range metadata {
		c.SetBlockMetadataLayer(LayerActive, local, key, value)
	}
}

// GetBlockMetadata возвращает метаданные для блока (слой ACTIVE)
func (c *Chunk) GetBlockMetadata(local vec.Vec2) map[string]interface{} {
	return c.GetBlockMetadataLayer(LayerActive, local)
}

// GetBlockMetadataValue возвращает конкретное значение метаданных (слой ACTIVE)
func (c *Chunk) GetBlockMetadataValue(local vec.Vec2, key string) (interface{}, bool) {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if metadata, exists := c.Metadata3D[BlockCoord{Layer: LayerActive, Pos: local}]; exists {
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

// GetBlockID возвращает ID блока по локальным координатам (слой ACTIVE)
func (c *Chunk) GetBlockID(local vec.Vec2) block.BlockID {
	return c.GetBlock(local)
}

// SetBlock устанавливает блок по локальным координатам (слой ACTIVE)
func (c *Chunk) SetBlock(local vec.Vec2, blockID block.BlockID) {
	c.SetBlockLayer(LayerActive, local, blockID)
}

// SetBlockWithBehavior устанавливает блок по локальным координатам с поведением (слой ACTIVE)
func (c *Chunk) SetBlockWithBehavior(local vec.Vec2, behavior block.BlockBehavior) {
	c.Mu.Lock()
	defer c.Mu.Unlock()

	// Обновляем блок
	c.Blocks3D[LayerActive][local.X][local.Y] = behavior.ID()
	c.Changes3D[BlockCoord{Layer: LayerActive, Pos: local}] = struct{}{}
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

// GetBlock возвращает ID блока по локальным координатам (слой ACTIVE)
func (c *Chunk) GetBlock(local vec.Vec2) block.BlockID {
	return c.GetBlockLayer(LayerActive, local)
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

	c.Changes3D = make(map[BlockCoord]struct{})
	c.ChangeCounter = 0
}

// GetBlockLayer возвращает ID блока на конкретном слое.
func (c *Chunk) GetBlockLayer(layer BlockLayer, local vec.Vec2) block.BlockID {
	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if layer >= MaxLayers {
		return 0
	}
	return c.Blocks3D[layer][local.X][local.Y]
}

// SetBlockLayer устанавливает блок на указанном слое.
func (c *Chunk) SetBlockLayer(layer BlockLayer, local vec.Vec2, blockID block.BlockID) {
	if layer >= MaxLayers {
		return
	}

	c.Mu.Lock()
	defer c.Mu.Unlock()

	c.Blocks3D[layer][local.X][local.Y] = blockID
	c.Changes3D[BlockCoord{Layer: layer, Pos: local}] = struct{}{}
	c.ChangeCounter++

	// Обновляем тикаемые блоки для активного слоя
	if layer == LayerActive {
		behavior, exists := block.Get(blockID)
		if exists && behavior.NeedsTick() {
			c.Tickable3D[BlockCoord{Layer: layer, Pos: local}] = struct{}{}
		} else {
			delete(c.Tickable3D, BlockCoord{Layer: layer, Pos: local})
		}
	}
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
	c.ChangeCounter++
}

// GetBlockMetadataLayer возвращает метаданные блока на указанном слое.
func (c *Chunk) GetBlockMetadataLayer(layer BlockLayer, local vec.Vec2) map[string]interface{} {
	coord := BlockCoord{Layer: layer, Pos: local}

	c.Mu.RLock()
	defer c.Mu.RUnlock()

	if meta, ok := c.Metadata3D[coord]; ok {
		// Создаем копию метаданных
		result := make(map[string]interface{}, len(meta))
		for k, v := range meta {
			result[k] = v
		}
		return result
	}
	return make(map[string]interface{})
}
