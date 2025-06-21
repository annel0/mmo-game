package world

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// RegionManager управляет регионами мира для параллельной обработки
type RegionManager struct {
	regionSize   int                   // Размер региона в чанках
	regions      map[regionKey]*Region // Карта регионов
	regionsMu    sync.RWMutex          // Мьютекс для карты регионов
	entityRegion map[string]regionKey  // Карта entity ID -> region key
	entityMu     sync.RWMutex          // Мьютекс для карты сущностей
	workerCount  int                   // Количество воркеров
	updateChan   chan *Region          // Канал для обновления регионов
	shutdownChan chan struct{}         // Канал для остановки
	wg           sync.WaitGroup        // WaitGroup для воркеров
	stats        RegionManagerStats    // Статистика
}

// regionKey представляет ключ региона
type regionKey struct {
	x, z int
}

// Region представляет регион мира
type Region struct {
	key          regionKey
	entities     map[string]*entity.Entity
	chunks       map[chunkKey]*Chunk
	spatialIndex *SpatialIndex
	mu           sync.RWMutex
	lastUpdate   time.Time
	dirty        atomic.Bool
}

// chunkKey представляет ключ чанка
type chunkKey struct {
	x, z int
}

// RegionManagerStats содержит статистику менеджера регионов
type RegionManagerStats struct {
	regionCount      atomic.Int32
	entityCount      atomic.Int64
	updatesPerSecond atomic.Int64
	updateDuration   atomic.Int64 // в наносекундах
}

// NewRegionManager создаёт новый менеджер регионов
func NewRegionManager(regionSize int, workerCount int) *RegionManager {
	if regionSize <= 0 {
		regionSize = 4 // 4x4 чанка по умолчанию
	}

	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}

	rm := &RegionManager{
		regionSize:   regionSize,
		regions:      make(map[regionKey]*Region),
		entityRegion: make(map[string]regionKey),
		workerCount:  workerCount,
		updateChan:   make(chan *Region, workerCount*2),
		shutdownChan: make(chan struct{}),
	}

	// Запускаем воркеров
	for i := 0; i < workerCount; i++ {
		rm.wg.Add(1)
		go rm.worker(i)
	}

	// Запускаем сборщик статистики
	go rm.statsCollector()

	return rm
}

// Start запускает обработку регионов
func (rm *RegionManager) Start() {
	go rm.updateLoop()
}

// Stop останавливает менеджер регионов
func (rm *RegionManager) Stop() {
	close(rm.shutdownChan)
	rm.wg.Wait()
}

// AddEntity добавляет сущность в соответствующий регион
func (rm *RegionManager) AddEntity(e *entity.Entity) {
	// Определяем регион
	rKey := rm.getRegionKey(e.Position)

	// Получаем или создаём регион
	rm.regionsMu.Lock()
	region, exists := rm.regions[rKey]
	if !exists {
		region = rm.createRegion(rKey)
		rm.regions[rKey] = region
		rm.stats.regionCount.Add(1)
	}
	rm.regionsMu.Unlock()

	// Добавляем сущность в регион
	region.mu.Lock()
	entityIDStr := fmt.Sprintf("%d", e.ID)
	region.entities[entityIDStr] = e
	region.spatialIndex.Insert(e)
	region.dirty.Store(true)
	region.mu.Unlock()

	// Обновляем маппинг
	rm.entityMu.Lock()
	rm.entityRegion[entityIDStr] = rKey
	rm.entityMu.Unlock()

	rm.stats.entityCount.Add(1)
}

// RemoveEntity удаляет сущность из региона
func (rm *RegionManager) RemoveEntity(entityID uint64) {
	entityIDStr := fmt.Sprintf("%d", entityID)

	// Находим регион сущности
	rm.entityMu.RLock()
	rKey, exists := rm.entityRegion[entityIDStr]
	rm.entityMu.RUnlock()

	if !exists {
		return
	}

	// Удаляем из региона
	rm.regionsMu.RLock()
	region, exists := rm.regions[rKey]
	rm.regionsMu.RUnlock()

	if exists {
		region.mu.Lock()
		delete(region.entities, entityIDStr)
		region.spatialIndex.Remove(entityID)
		region.dirty.Store(true)
		region.mu.Unlock()
	}

	// Удаляем маппинг
	rm.entityMu.Lock()
	delete(rm.entityRegion, entityIDStr)
	rm.entityMu.Unlock()

	rm.stats.entityCount.Add(-1)
}

// MoveEntity перемещает сущность между регионами при необходимости
func (rm *RegionManager) MoveEntity(e *entity.Entity) {
	entityIDStr := fmt.Sprintf("%d", e.ID)
	newRKey := rm.getRegionKey(e.Position)

	// Проверяем, изменился ли регион
	rm.entityMu.RLock()
	oldRKey, exists := rm.entityRegion[entityIDStr]
	rm.entityMu.RUnlock()

	if !exists {
		// Сущность не была зарегистрирована, добавляем
		rm.AddEntity(e)
		return
	}

	if oldRKey == newRKey {
		// Регион не изменился, обновляем пространственный индекс
		rm.regionsMu.RLock()
		region, exists := rm.regions[oldRKey]
		rm.regionsMu.RUnlock()

		if exists {
			region.mu.Lock()
			region.spatialIndex.Update(e)
			region.dirty.Store(true)
			region.mu.Unlock()
		}
		return
	}

	// Регион изменился, перемещаем сущность

	// Удаляем из старого региона
	rm.regionsMu.RLock()
	oldRegion, exists := rm.regions[oldRKey]
	rm.regionsMu.RUnlock()

	if exists {
		oldRegion.mu.Lock()
		delete(oldRegion.entities, entityIDStr)
		oldRegion.spatialIndex.Remove(e.ID)
		oldRegion.dirty.Store(true)
		oldRegion.mu.Unlock()
	}

	// Добавляем в новый регион
	rm.regionsMu.Lock()
	newRegion, exists := rm.regions[newRKey]
	if !exists {
		newRegion = rm.createRegion(newRKey)
		rm.regions[newRKey] = newRegion
		rm.stats.regionCount.Add(1)
	}
	rm.regionsMu.Unlock()

	newRegion.mu.Lock()
	newRegion.entities[entityIDStr] = e
	newRegion.spatialIndex.Insert(e)
	newRegion.dirty.Store(true)
	newRegion.mu.Unlock()

	// Обновляем маппинг
	rm.entityMu.Lock()
	rm.entityRegion[entityIDStr] = newRKey
	rm.entityMu.Unlock()
}

// GetEntitiesInRange возвращает сущности в радиусе от точки
func (rm *RegionManager) GetEntitiesInRange(center vec.Vec2Float, radius float64) []*entity.Entity {
	// Определяем регионы, которые могут содержать сущности
	minX := center.X - radius
	maxX := center.X + radius
	minZ := center.Y - radius
	maxZ := center.Y + radius

	// Конвертируем в координаты регионов
	chunkSize := 16.0
	regionSizeWorld := float64(rm.regionSize) * chunkSize

	minRegionX := int(minX / regionSizeWorld)
	maxRegionX := int(maxX / regionSizeWorld)
	minRegionZ := int(minZ / regionSizeWorld)
	maxRegionZ := int(maxZ / regionSizeWorld)

	// Корректируем для отрицательных координат
	if minX < 0 && minX != float64(minRegionX)*regionSizeWorld {
		minRegionX--
	}
	if minZ < 0 && minZ != float64(minRegionZ)*regionSizeWorld {
		minRegionZ--
	}

	result := make([]*entity.Entity, 0)
	seen := make(map[string]struct{})

	rm.regionsMu.RLock()
	defer rm.regionsMu.RUnlock()

	// Проверяем все релевантные регионы
	for rx := minRegionX; rx <= maxRegionX; rx++ {
		for rz := minRegionZ; rz <= maxRegionZ; rz++ {
			if region, exists := rm.regions[regionKey{x: rx, z: rz}]; exists {
				region.mu.RLock()
				entities := region.spatialIndex.QueryRange(center, radius)
				region.mu.RUnlock()

				// Добавляем уникальные сущности
				for _, e := range entities {
					entityIDStr := fmt.Sprintf("%d", e.ID)
					if _, wasSeen := seen[entityIDStr]; !wasSeen {
						result = append(result, e)
						seen[entityIDStr] = struct{}{}
					}
				}
			}
		}
	}

	return result
}

// UpdateRegions запускает обновление всех активных регионов
func (rm *RegionManager) UpdateRegions(dt float64) {
	rm.regionsMu.RLock()
	defer rm.regionsMu.RUnlock()

	// Отправляем регионы на обновление
	for _, region := range rm.regions {
		if region.dirty.Load() {
			select {
			case rm.updateChan <- region:
				// Регион отправлен на обновление
			default:
				// Канал полон, пропускаем этот тик
			}
		}
	}
}

// GetStats возвращает статистику менеджера
func (rm *RegionManager) GetStats() string {
	regionCount := rm.stats.regionCount.Load()
	entityCount := rm.stats.entityCount.Load()
	updatesPerSecond := rm.stats.updatesPerSecond.Load()
	avgUpdateMs := float64(rm.stats.updateDuration.Load()) / 1e6 / float64(updatesPerSecond+1)

	return fmt.Sprintf("RegionManager: %d regions, %d entities, %d updates/s, %.2fms avg update",
		regionCount, entityCount, updatesPerSecond, avgUpdateMs)
}

// Внутренние методы

// getRegionKey возвращает ключ региона для позиции
func (rm *RegionManager) getRegionKey(pos vec.Vec2) regionKey {
	chunkX := pos.X >> 4 // Деление на 16
	chunkZ := pos.Y >> 4

	regionX := chunkX / rm.regionSize
	regionZ := chunkZ / rm.regionSize

	// Корректируем для отрицательных координат
	if chunkX < 0 && chunkX%rm.regionSize != 0 {
		regionX--
	}
	if chunkZ < 0 && chunkZ%rm.regionSize != 0 {
		regionZ--
	}

	return regionKey{x: regionX, z: regionZ}
}

// createRegion создаёт новый регион
func (rm *RegionManager) createRegion(key regionKey) *Region {
	return &Region{
		key:          key,
		entities:     make(map[string]*entity.Entity),
		chunks:       make(map[chunkKey]*Chunk),
		spatialIndex: NewSpatialIndex(16.0), // Размер ячейки = размер чанка
		lastUpdate:   time.Now(),
	}
}

// worker обрабатывает регионы
func (rm *RegionManager) worker(id int) {
	defer rm.wg.Done()

	for {
		select {
		case <-rm.shutdownChan:
			return
		case region := <-rm.updateChan:
			rm.updateRegion(region)
		}
	}
}

// updateRegion обновляет один регион
func (rm *RegionManager) updateRegion(region *Region) {
	start := time.Now()

	region.mu.Lock()
	defer region.mu.Unlock()

	// Обновляем сущности в регионе
	dt := time.Since(region.lastUpdate).Seconds()
	region.lastUpdate = time.Now()

	// Обновляем сущности в регионе
	for _, e := range region.entities {
		rm.updateEntityBehavior(e, dt)
	}

	region.dirty.Store(false)

	// Обновляем статистику
	duration := time.Since(start).Nanoseconds()
	rm.stats.updateDuration.Add(duration)
	rm.stats.updatesPerSecond.Add(1)
}

// updateLoop основной цикл обновления
func (rm *RegionManager) updateLoop() {
	ticker := time.NewTicker(50 * time.Millisecond) // 20 Hz
	defer ticker.Stop()

	for {
		select {
		case <-rm.shutdownChan:
			return
		case <-ticker.C:
			rm.UpdateRegions(0.05) // 50ms
		}
	}
}

// statsCollector собирает статистику
func (rm *RegionManager) statsCollector() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rm.shutdownChan:
			return
		case <-ticker.C:
			// Сбрасываем счётчик обновлений в секунду
			updates := rm.stats.updatesPerSecond.Load()
			rm.stats.updatesPerSecond.Store(0)
			rm.stats.updateDuration.Store(0)

			// Логируем статистику
			if updates > 0 {
				log.Printf("📊 %s", rm.GetStats())
			}
		}
	}
}

// updateEntityBehavior обновляет поведение сущности
func (rm *RegionManager) updateEntityBehavior(e *entity.Entity, dt float64) {
	// Проверяем, есть ли поведение для этого типа сущности
	// В реальной реализации здесь будет вызов EntityBehavior.Update()
	
	// Обновляем базовые параметры сущности
	if e.Velocity.X != 0 || e.Velocity.Y != 0 {
		// Обновляем точную позицию на основе скорости
		e.PrecisePos.X += e.Velocity.X * dt
		e.PrecisePos.Y += e.Velocity.Y * dt

		// Обновляем позицию блока
		e.Position.X = int(e.PrecisePos.X)
		e.Position.Y = int(e.PrecisePos.Y)

		// Применяем трение (если нужно)
		friction := 0.95
		e.Velocity.X *= friction
		e.Velocity.Y *= friction

		// Останавливаем очень медленное движение
		if abs(e.Velocity.X) < 0.01 {
			e.Velocity.X = 0
		}
		if abs(e.Velocity.Y) < 0.01 {
			e.Velocity.Y = 0
		}
	}

	// Обновляем дополнительные данные сущности
	if e.Payload != nil {
		// Обновляем таймеры эффектов из Payload
		if effects, ok := e.Payload["effects"].(map[string]interface{}); ok {
			for name, effectData := range effects {
				if effectMap, ok := effectData.(map[string]interface{}); ok {
					if duration, ok := effectMap["duration"].(float64); ok && duration > 0 {
						duration -= dt
						if duration <= 0 {
							// Эффект закончился
							delete(effects, name)
						} else {
							effectMap["duration"] = duration
						}
					}
				}
			}
		}

		// Обновляем состояние здоровья из Payload
		if health, ok := e.Payload["health"].(float64); ok {
			if health <= 0 && e.Active {
				// Сущность умерла
				e.Active = false
				e.Payload["death_time"] = dt
				// Здесь можно добавить логику смерти (дроп предметов, респавн и т.д.)
			}
		}
	}
}

// abs возвращает абсолютное значение float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
