package world

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// SpatialIndex представляет пространственный индекс для быстрого поиска сущностей
type SpatialIndex struct {
	cellSize float64
	cells    map[cellKey]*cellData
	cellsMu  sync.RWMutex
	entities map[string]*indexedEntity // Используем string ID
	entityMu sync.RWMutex
}

// cellKey представляет ключ ячейки в пространственной сетке
type cellKey struct {
	x, z int
}

// cellData хранит данные ячейки
type cellData struct {
	entities map[string]*indexedEntity
	mu       sync.RWMutex
}

// indexedEntity представляет индексированную сущность
type indexedEntity struct {
	entity  *entity.Entity
	cells   map[cellKey]struct{}
	cellsMu sync.RWMutex
	bounds  entityBounds
}

// entityBounds представляет границы сущности
type entityBounds struct {
	minX, minZ float64
	maxX, maxZ float64
}

// NewSpatialIndex создаёт новый пространственный индекс
func NewSpatialIndex(cellSize float64) *SpatialIndex {
	if cellSize <= 0 {
		cellSize = 16.0 // Размер чанка по умолчанию
	}

	return &SpatialIndex{
		cellSize: cellSize,
		cells:    make(map[cellKey]*cellData),
		entities: make(map[string]*indexedEntity),
	}
}

// entityIDToString конвертирует uint64 ID в string
func entityIDToString(id uint64) string {
	return strconv.FormatUint(id, 10)
}

// Insert добавляет сущность в индекс
func (si *SpatialIndex) Insert(e *entity.Entity) {
	si.entityMu.Lock()
	defer si.entityMu.Unlock()

	entityIDStr := entityIDToString(e.ID)

	// Вычисляем границы сущности на основе PrecisePos и Size
	halfWidth := e.Size.X / 2.0
	halfHeight := e.Size.Y / 2.0

	bounds := entityBounds{
		minX: e.PrecisePos.X - halfWidth,
		minZ: e.PrecisePos.Y - halfHeight,
		maxX: e.PrecisePos.X + halfWidth,
		maxZ: e.PrecisePos.Y + halfHeight,
	}

	// Создаём индексированную сущность
	indexed := &indexedEntity{
		entity: e,
		cells:  make(map[cellKey]struct{}),
		bounds: bounds,
	}

	// Вычисляем ячейки, которые покрывает сущность
	cells := si.getCellsForBounds(bounds)

	// Добавляем сущность в ячейки
	si.cellsMu.Lock()
	for _, key := range cells {
		cell := si.getOrCreateCell(key)
		cell.mu.Lock()
		cell.entities[entityIDStr] = indexed
		cell.mu.Unlock()

		indexed.cellsMu.Lock()
		indexed.cells[key] = struct{}{}
		indexed.cellsMu.Unlock()
	}
	si.cellsMu.Unlock()

	// Сохраняем индексированную сущность
	si.entities[entityIDStr] = indexed
}

// Update обновляет позицию сущности в индексе
func (si *SpatialIndex) Update(e *entity.Entity) {
	entityIDStr := entityIDToString(e.ID)

	si.entityMu.RLock()
	indexed, exists := si.entities[entityIDStr]
	si.entityMu.RUnlock()

	if !exists {
		// Сущность не была в индексе, добавляем
		si.Insert(e)
		return
	}

	// Вычисляем новые границы
	halfWidth := e.Size.X / 2.0
	halfHeight := e.Size.Y / 2.0

	newBounds := entityBounds{
		minX: e.PrecisePos.X - halfWidth,
		minZ: e.PrecisePos.Y - halfHeight,
		maxX: e.PrecisePos.X + halfWidth,
		maxZ: e.PrecisePos.Y + halfHeight,
	}

	// Проверяем, изменились ли занимаемые ячейки
	newCells := si.getCellsForBounds(newBounds)

	indexed.cellsMu.RLock()
	oldCells := make(map[cellKey]struct{})
	for key := range indexed.cells {
		oldCells[key] = struct{}{}
	}
	indexed.cellsMu.RUnlock()

	// Определяем добавленные и удалённые ячейки
	toAdd := make([]cellKey, 0)
	toRemove := make([]cellKey, 0)

	for _, key := range newCells {
		if _, exists := oldCells[key]; !exists {
			toAdd = append(toAdd, key)
		}
	}

	for key := range oldCells {
		found := false
		for _, newKey := range newCells {
			if key == newKey {
				found = true
				break
			}
		}
		if !found {
			toRemove = append(toRemove, key)
		}
	}

	// Обновляем ячейки
	si.cellsMu.Lock()

	// Удаляем из старых ячеек
	for _, key := range toRemove {
		if cell, exists := si.cells[key]; exists {
			cell.mu.Lock()
			delete(cell.entities, entityIDStr)
			if len(cell.entities) == 0 {
				delete(si.cells, key)
			}
			cell.mu.Unlock()
		}
	}

	// Добавляем в новые ячейки
	for _, key := range toAdd {
		cell := si.getOrCreateCell(key)
		cell.mu.Lock()
		cell.entities[entityIDStr] = indexed
		cell.mu.Unlock()
	}

	si.cellsMu.Unlock()

	// Обновляем список ячеек в сущности
	indexed.cellsMu.Lock()
	indexed.cells = make(map[cellKey]struct{})
	for _, key := range newCells {
		indexed.cells[key] = struct{}{}
	}
	indexed.cellsMu.Unlock()

	// Обновляем границы
	indexed.bounds = newBounds
}

// Remove удаляет сущность из индекса
func (si *SpatialIndex) Remove(entityID uint64) {
	entityIDStr := entityIDToString(entityID)

	si.entityMu.Lock()
	indexed, exists := si.entities[entityIDStr]
	if !exists {
		si.entityMu.Unlock()
		return
	}
	delete(si.entities, entityIDStr)
	si.entityMu.Unlock()

	// Удаляем из всех ячеек
	indexed.cellsMu.RLock()
	cells := make([]cellKey, 0, len(indexed.cells))
	for key := range indexed.cells {
		cells = append(cells, key)
	}
	indexed.cellsMu.RUnlock()

	si.cellsMu.Lock()
	for _, key := range cells {
		if cell, exists := si.cells[key]; exists {
			cell.mu.Lock()
			delete(cell.entities, entityIDStr)
			if len(cell.entities) == 0 {
				delete(si.cells, key)
			}
			cell.mu.Unlock()
		}
	}
	si.cellsMu.Unlock()
}

// QueryRange возвращает все сущности в заданном радиусе от точки
func (si *SpatialIndex) QueryRange(center vec.Vec2Float, radius float64) []*entity.Entity {
	// Вычисляем границы поиска
	bounds := entityBounds{
		minX: center.X - radius,
		minZ: center.Y - radius,
		maxX: center.X + radius,
		maxZ: center.Y + radius,
	}

	// Получаем ячейки в границах
	cells := si.getCellsForBounds(bounds)

	// Собираем уникальные сущности
	seen := make(map[string]struct{})
	result := make([]*entity.Entity, 0)

	si.cellsMu.RLock()
	for _, key := range cells {
		if cell, exists := si.cells[key]; exists {
			cell.mu.RLock()
			for entityID, indexed := range cell.entities {
				if _, wasSeen := seen[entityID]; !wasSeen {
					// Проверяем, действительно ли сущность в радиусе
					dx := indexed.entity.PrecisePos.X - center.X
					dz := indexed.entity.PrecisePos.Y - center.Y
					distSq := dx*dx + dz*dz

					if distSq <= radius*radius {
						result = append(result, indexed.entity)
						seen[entityID] = struct{}{}
					}
				}
			}
			cell.mu.RUnlock()
		}
	}
	si.cellsMu.RUnlock()

	return result
}

// QueryRect возвращает все сущности в прямоугольной области
func (si *SpatialIndex) QueryRect(minX, minZ, maxX, maxZ float64) []*entity.Entity {
	bounds := entityBounds{
		minX: minX,
		minZ: minZ,
		maxX: maxX,
		maxZ: maxZ,
	}

	cells := si.getCellsForBounds(bounds)

	seen := make(map[string]struct{})
	result := make([]*entity.Entity, 0)

	si.cellsMu.RLock()
	for _, key := range cells {
		if cell, exists := si.cells[key]; exists {
			cell.mu.RLock()
			for entityID, indexed := range cell.entities {
				if _, wasSeen := seen[entityID]; !wasSeen {
					// Проверяем, что центр сущности в прямоугольнике
					if indexed.entity.PrecisePos.X >= minX && indexed.entity.PrecisePos.X <= maxX &&
						indexed.entity.PrecisePos.Y >= minZ && indexed.entity.PrecisePos.Y <= maxZ {
						result = append(result, indexed.entity)
						seen[entityID] = struct{}{}
					}
				}
			}
			cell.mu.RUnlock()
		}
	}
	si.cellsMu.RUnlock()

	return result
}

// GetNearbyEntities возвращает сущности вблизи заданной сущности
func (si *SpatialIndex) GetNearbyEntities(e *entity.Entity, radius float64) []*entity.Entity {
	return si.QueryRange(e.PrecisePos, radius)
}

// GetCellCount возвращает количество активных ячеек
func (si *SpatialIndex) GetCellCount() int {
	si.cellsMu.RLock()
	defer si.cellsMu.RUnlock()
	return len(si.cells)
}

// GetEntityCount возвращает количество индексированных сущностей
func (si *SpatialIndex) GetEntityCount() int {
	si.entityMu.RLock()
	defer si.entityMu.RUnlock()
	return len(si.entities)
}

// GetStats возвращает статистику индекса
func (si *SpatialIndex) GetStats() string {
	si.cellsMu.RLock()
	cellCount := len(si.cells)
	totalEntitiesInCells := 0
	maxEntitiesPerCell := 0

	for _, cell := range si.cells {
		cell.mu.RLock()
		count := len(cell.entities)
		cell.mu.RUnlock()

		totalEntitiesInCells += count
		if count > maxEntitiesPerCell {
			maxEntitiesPerCell = count
		}
	}
	si.cellsMu.RUnlock()

	si.entityMu.RLock()
	entityCount := len(si.entities)
	si.entityMu.RUnlock()

	avgEntitiesPerCell := 0.0
	if cellCount > 0 {
		avgEntitiesPerCell = float64(totalEntitiesInCells) / float64(cellCount)
	}

	return fmt.Sprintf("SpatialIndex Stats: %d entities, %d cells, avg %.2f entities/cell, max %d entities/cell",
		entityCount, cellCount, avgEntitiesPerCell, maxEntitiesPerCell)
}

// Вспомогательные методы

// getCellsForBounds возвращает ключи ячеек, которые пересекаются с границами
func (si *SpatialIndex) getCellsForBounds(bounds entityBounds) []cellKey {
	minCellX := int(bounds.minX / si.cellSize)
	minCellZ := int(bounds.minZ / si.cellSize)
	maxCellX := int(bounds.maxX / si.cellSize)
	maxCellZ := int(bounds.maxZ / si.cellSize)

	// Корректируем для отрицательных координат
	if bounds.minX < 0 && bounds.minX != float64(minCellX)*si.cellSize {
		minCellX--
	}
	if bounds.minZ < 0 && bounds.minZ != float64(minCellZ)*si.cellSize {
		minCellZ--
	}

	cells := make([]cellKey, 0, (maxCellX-minCellX+1)*(maxCellZ-minCellZ+1))

	for x := minCellX; x <= maxCellX; x++ {
		for z := minCellZ; z <= maxCellZ; z++ {
			cells = append(cells, cellKey{x: x, z: z})
		}
	}

	return cells
}

// getOrCreateCell возвращает ячейку или создаёт новую
func (si *SpatialIndex) getOrCreateCell(key cellKey) *cellData {
	if cell, exists := si.cells[key]; exists {
		return cell
	}

	cell := &cellData{
		entities: make(map[string]*indexedEntity),
	}
	si.cells[key] = cell
	return cell
}
