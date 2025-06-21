package world

import (
	"log"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// chunkBlockAPI реализует block.BlockAPI для конкретного чанка
type chunkBlockAPI struct {
	chunk    *Chunk
	bigChunk *BigChunk
	world    *WorldManager
}

// bigChunkBlockAPI реализует интерфейс block.BlockAPI для BigChunk
type bigChunkBlockAPI struct {
	bigChunk *BigChunk
	world    *WorldManager
}

// newChunkBlockAPI создает новый API для блоков в чанке
func newChunkBlockAPI(chunk *Chunk, bigChunk *BigChunk, world *WorldManager) block.BlockAPI {
	return &chunkBlockAPI{
		chunk:    chunk,
		bigChunk: bigChunk,
		world:    world,
	}
}

// GetBlockID возвращает ID блока по глобальным координатам
func (api *chunkBlockAPI) GetBlockID(pos vec.Vec2) block.BlockID {
	// Конвертируем глобальные координаты в локальные
	local := api.chunk.toLocalCoords(pos)
	return api.chunk.GetBlockID(local)
}

// SetBlock устанавливает блок по глобальным координатам
func (api *chunkBlockAPI) SetBlock(pos vec.Vec2, id block.BlockID) {
	// Перенаправляем запрос в WorldManager
	api.world.SetBlock(pos, Block{ID: id})
}

// GetBlockMetadata возвращает метаданные блока
func (api *chunkBlockAPI) GetBlockMetadata(pos vec.Vec2, key string) interface{} {
	local := api.chunk.toLocalCoords(pos)
	value, exists := api.chunk.GetBlockMetadataValue(local, key)
	if exists {
		return value
	}
	// Если метаданные не найдены, возвращаем nil
	return nil
}

// SetBlockMetadata устанавливает метаданные блока
func (api *chunkBlockAPI) SetBlockMetadata(pos vec.Vec2, key string, value interface{}) {
	local := api.chunk.toLocalCoords(pos)
	api.chunk.SetBlockMetadata(local, key, value)
}

// Реализация методов BlockAPI для BigChunk

// GetBlockID возвращает ID блока по координатам
func (api *bigChunkBlockAPI) GetBlockID(pos vec.Vec2) block.BlockID {
	chunkCoords := pos.ToChunkCoords()
	localPos := pos.LocalInChunk()

	api.bigChunk.mu.RLock()
	chunk, exists := api.bigChunk.chunks[chunkCoords]
	api.bigChunk.mu.RUnlock()

	if !exists {
		// Если чанк не существует, запрашиваем его у WorldManager
		return api.world.GetBlock(pos).ID
	}

	return chunk.GetBlock(localPos)
}

// SetBlock устанавливает блок по координатам
func (api *bigChunkBlockAPI) SetBlock(pos vec.Vec2, id block.BlockID) {
	// Создаем событие изменения блока
	event := BlockEvent{
		EventType:   EventTypeBlockChange,
		SourceChunk: pos.ToChunkCoords(),
		TargetChunk: pos.ToChunkCoords(),
		Position:    pos,
		Block:       Block{ID: id},
	}

	// Отправляем событие в мировой менеджер
	select {
	case api.bigChunk.eventsOut <- event:
		// Успешно отправлено
	default:
		// Канал переполнен, пропускаем
	}
}

// GetBlockMetadata возвращает метаданные блока по ключу
func (api *bigChunkBlockAPI) GetBlockMetadata(pos vec.Vec2, key string) interface{} {
	chunkCoords := pos.ToChunkCoords()
	localPos := pos.LocalInChunk()

	api.bigChunk.mu.RLock()
	chunk, exists := api.bigChunk.chunks[chunkCoords]
	api.bigChunk.mu.RUnlock()

	if !exists {
		// Если чанк не существует, запрашиваем его у WorldManager
		block := api.world.GetBlock(pos)
		if value, exists := block.Payload[key]; exists {
			return value
		}
		return nil
	}

	// Получаем метаданные из чанка
	value, _ := chunk.GetBlockMetadataValue(localPos, key)
	return value
}

// SetBlockMetadata устанавливает метаданные блока
func (api *bigChunkBlockAPI) SetBlockMetadata(pos vec.Vec2, key string, value interface{}) {
	chunkCoords := pos.ToChunkCoords()
	localPos := pos.LocalInChunk()

	api.bigChunk.mu.RLock()
	chunk, exists := api.bigChunk.chunks[chunkCoords]
	api.bigChunk.mu.RUnlock()

	if !exists {
		// Если чанк не существует, создаём его
		api.bigChunk.mu.Lock()
		chunk, exists = api.bigChunk.chunks[chunkCoords]
		if !exists {
			// Создаём новый чанк
			chunk = NewChunk(chunkCoords)
			api.bigChunk.chunks[chunkCoords] = chunk
		}
		api.bigChunk.mu.Unlock()
	}

	// Устанавливаем метаданные
	chunk.SetBlockMetadata(localPos, key, value)

	// Получаем текущий блок для отправки обновления
	blockID := chunk.GetBlock(localPos)
	blockPayload := chunk.GetBlockMetadata(localPos)

	// Создаем событие изменения блока для уведомления клиентов
	event := BlockEvent{
		EventType:   EventTypeBlockChange,
		SourceChunk: pos.ToChunkCoords(),
		TargetChunk: pos.ToChunkCoords(),
		Position:    pos,
		Block:       Block{ID: blockID, Payload: blockPayload},
		Data:        map[string]interface{}{key: value},
	}

	// Отправляем событие в мировой менеджер
	select {
	case api.bigChunk.eventsOut <- event:
		// Успешно отправлено
	default:
		// Канал переполнен, логируем
		log.Printf("Канал событий переполнен, событие обновления метаданных блока отброшено")
	}
}

// SendEvent отправляет событие блока
func (api *bigChunkBlockAPI) SendEvent(eventType EventType, data interface{}, targetPos vec.Vec2) {
	// Создаём событие для блока
	event := BlockEvent{
		EventType:   eventType,
		SourceChunk: api.bigChunk.coords,
		TargetChunk: targetPos.ToChunkCoords(),
		Position:    targetPos,
		Data:        data,
	}

	// Проверяем, находится ли целевой блок в этом же BigChunk
	if targetPos.ToBigChunkCoords() == api.bigChunk.coords {
		// Отправляем событие внутри этого же BigChunk
		select {
		case api.bigChunk.eventsIn <- event:
			// Успешно отправлено
		default:
			// Канал переполнен, пропускаем
		}
	} else {
		// Отправляем событие через WorldManager
		select {
		case api.bigChunk.eventsOut <- event:
			// Успешно отправлено
		default:
			// Канал переполнен, пропускаем
		}
	}
}

// --- Layer-aware helpers ---
func (api *chunkBlockAPI) GetBlockIDLayer(layer uint8, pos vec.Vec2) block.BlockID {
	local := api.chunk.toLocalCoords(pos)
	return api.chunk.GetBlockLayer(BlockLayer(layer), local)
}

func (api *chunkBlockAPI) SetBlockLayer(layer uint8, pos vec.Vec2, id block.BlockID) {
	// Перенаправляем запрос в WorldManager с указанием слоя
	api.world.SetBlockLayer(pos, BlockLayer(layer), Block{ID: id})
}

func (api *bigChunkBlockAPI) GetBlockIDLayer(layer uint8, pos vec.Vec2) block.BlockID {
	return api.world.GetBlockLayer(pos, BlockLayer(layer)).ID
}

func (api *bigChunkBlockAPI) SetBlockLayer(layer uint8, pos vec.Vec2, id block.BlockID) {
	// Создаем событие изменения блока с указанием слоя
	event := BlockEvent{
		EventType:   EventTypeBlockChange,
		SourceChunk: pos.ToChunkCoords(),
		TargetChunk: pos.ToChunkCoords(),
		Position:    pos,
		Block:       Block{ID: id},
		Data: map[string]interface{}{
			"layer": layer,
		},
	}

	// Отправляем событие в мировой менеджер
	select {
	case api.bigChunk.eventsOut <- event:
		// Успешно отправлено
	default:
		// Канал переполнен, пропускаем
		log.Printf("Канал событий переполнен, событие установки блока на слое %d отброшено", layer)
	}
}

// ScheduleUpdateOnce помечает блок для разового обновления в следующем тике
func (api *bigChunkBlockAPI) ScheduleUpdateOnce(pos vec.Vec2) {
	api.bigChunk.AddOnceTickable(pos)
}

// TriggerNeighborUpdates запускает разовое обновление для всех соседних блоков
func (api *bigChunkBlockAPI) TriggerNeighborUpdates(pos vec.Vec2) {
	// Обновляем 4 соседних блока
	neighbors := []vec.Vec2{
		{X: pos.X + 1, Y: pos.Y}, // право
		{X: pos.X - 1, Y: pos.Y}, // лево
		{X: pos.X, Y: pos.Y + 1}, // вниз
		{X: pos.X, Y: pos.Y - 1}, // вверх
	}

	for _, neighbor := range neighbors {
		api.ScheduleUpdateOnce(neighbor)
	}
}

// ScheduleUpdateOnce помечает блок для разового обновления в следующем тике
func (api *chunkBlockAPI) ScheduleUpdateOnce(pos vec.Vec2) {
	// Делегируем вызов к BigChunk
	api.bigChunk.AddOnceTickable(pos)
}

// TriggerNeighborUpdates запускает разовое обновление для всех соседних блоков
func (api *chunkBlockAPI) TriggerNeighborUpdates(pos vec.Vec2) {
	// Обновляем 4 соседних блока
	neighbors := []vec.Vec2{
		{X: pos.X + 1, Y: pos.Y}, // право
		{X: pos.X - 1, Y: pos.Y}, // лево
		{X: pos.X, Y: pos.Y + 1}, // вниз
		{X: pos.X, Y: pos.Y - 1}, // вверх
	}

	for _, neighbor := range neighbors {
		api.ScheduleUpdateOnce(neighbor)
	}
}

// ... другие методы
