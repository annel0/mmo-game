package world

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/physics"
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// BigChunk представляет собой единицу симуляции, которая содержит 32x32 чанка
type BigChunk struct {
	coords    vec.Vec2               // Координаты BigChunk в мире
	chunks    map[vec.Vec2]*Chunk    // Чанки, принадлежащие этому BigChunk
	eventsIn  chan Event             // Входящие события
	eventsOut chan<- Event           // Исходящие события (в WorldManager)
	tickables map[vec.Vec2]struct{}  // Тикаемые блоки в этом BigChunk
	entities  map[uint64]interface{} // Сущности в этом BigChunk (игроки, NPC)
	world     *WorldManager          // Ссылка на WorldManager
	mu        sync.RWMutex           // Мьютекс для безопасного доступа
	tickID    uint64                 // Текущий номер тика для этого BigChunk
}

// EntityData представляет данные о сущности внутри BigChunk
type EntityData struct {
	ID       uint64                 // Уникальный ID сущности
	Type     uint16                 // Тип сущности
	Position vec.Vec2               // Текущая позиция
	Metadata map[string]interface{} // Дополнительные данные
}

// NewBigChunk создаёт новый BigChunk с указанными координатами
func NewBigChunk(coords vec.Vec2, world *WorldManager, eventsOut chan<- Event) *BigChunk {
	return &BigChunk{
		coords:    coords,
		chunks:    make(map[vec.Vec2]*Chunk),
		eventsIn:  make(chan Event, 1000),
		eventsOut: eventsOut,
		tickables: make(map[vec.Vec2]struct{}),
		entities:  make(map[uint64]interface{}),
		world:     world,
		mu:        sync.RWMutex{},
		tickID:    0,
	}
}

// Run запускает горутину обработки для BigChunk
func (bc *BigChunk) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second / 60) // 60 TPS
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-bc.eventsIn:
			bc.handleEvent(event)
		case <-ticker.C:
			bc.processTick()
		}
	}
}

// processTick обрабатывает один игровой тик для этого BigChunk
func (bc *BigChunk) processTick() {
	bc.mu.Lock()
	bc.tickID++
	// Используем tickID, если нужно
	_ = bc.tickID
	bc.mu.Unlock()

	// 1. Обновление тикаемых блоков
	bc.updateBlocks()

	// 2. Обновление сущностей
	bc.updateEntities()

	// 3. Обработка отложенных событий
	// TODO: Реализовать при необходимости
}

// updateBlocks обновляет все тикаемые блоки в BigChunk
func (bc *BigChunk) updateBlocks() {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Создаем BlockAPI для доступа к миру из блоков
	api := bc.createBlockAPI()

	// Обходим все тикаемые блоки и вызываем их TickUpdate
	for pos := range bc.tickables {
		chunkCoords := pos.ToChunkCoords()
		chunk, exists := bc.chunks[chunkCoords]
		if !exists {
			continue
		}

		localPos := pos.LocalInChunk()
		blockID := chunk.GetBlock(localPos)
		block := Block{ID: blockID, Payload: chunk.GetBlockMetadata(localPos)}

		behavior, exists := block.GetBehavior()
		if !exists || !behavior.NeedsTick() {
			// Блок больше не требует тиков, удаляем из списка тикаемых
			delete(bc.tickables, pos)
			continue
		}

		// Вызываем TickUpdate для блока
		behavior.TickUpdate(api, pos)
	}
}

// updateEntities обновляет все сущности в BigChunk
func (bc *BigChunk) updateEntities() {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Здесь будет логика обновления сущностей
	// Например, вызов AI для NPC, обработка физики и т.д.

	// В полной реализации здесь будет цикл по всем сущностям
	// и вызов соответствующих методов обновления
	for entityID, entityData := range bc.entities {
		if data, ok := entityData.(EntityData); ok {
			// Обработка в зависимости от типа сущности
			switch data.Type {
			case 0: // EntityTypePlayer
				// Обновление игрока (если нужно)
			case 1: // EntityTypeNPC
				// Обновление NPC
				bc.updateNPC(entityID, data)
			case 2: // EntityTypeMonster
				// Обновление монстра
				bc.updateMonster(entityID, data)
			}
		}
	}
}

// updateNPC обновляет состояние NPC
func (bc *BigChunk) updateNPC(entityID uint64, data EntityData) {
	// Здесь будет логика обновления NPC
	// Например, перемещение, диалоги, торговля и т.д.

	// Пример: случайное перемещение
	if rand.Float32() < 0.01 { // 1% шанс в тик
		// Генерируем случайное направление
		directions := []vec.Vec2{
			{X: 0, Y: 1},  // Вниз
			{X: 1, Y: 0},  // Вправо
			{X: 0, Y: -1}, // Вверх
			{X: -1, Y: 0}, // Влево
		}
		dir := directions[rand.Intn(len(directions))]

		// Вычисляем новую позицию
		newPos := vec.Vec2{
			X: data.Position.X + dir.X,
			Y: data.Position.Y + dir.Y,
		}

		// Проверяем, можно ли переместиться (проверка коллизий)
		if bc.canEntityMoveTo(entityID, newPos) {
			// Обновляем позицию
			data.Position = newPos
			bc.entities[entityID] = data

			// Если нужно, отправляем событие о перемещении
			// (например, для синхронизации с клиентами)
			moveEvent := EntityEvent{
				EventType: EventTypeEntityMove,
				EntityID:  entityID,
				Position:  newPos,
				Data:      data,
			}
			bc.eventsOut <- moveEvent
		}
	}
}

// updateMonster обновляет состояние монстра
func (bc *BigChunk) updateMonster(entityID uint64, data EntityData) {
	// Аналогично updateNPC, но с логикой для монстров
	// Например, поиск игроков, атака, преследование и т.д.
}

// canEntityMoveTo проверяет, может ли сущность переместиться в указанную позицию
func (bc *BigChunk) canEntityMoveTo(entityID uint64, newPos vec.Vec2) bool {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	// Получаем сущность по ID
	_, exists := bc.entities[entityID]
	if !exists {
		return false
	}

	// Создаем коллайдер для сущности (по умолчанию 1x1)
	collider := physics.NewBoxCollider(1, 1)

	// Создаем функцию проверки проходимости блока
	blockChecker := func(pos vec.Vec2) bool {
		chunkCoords := pos.ToChunkCoords()
		chunk, exists := bc.chunks[chunkCoords]
		if !exists {
			// Если чанк не загружен, создаем его
			chunk = NewChunk(chunkCoords)
			bc.chunks[chunkCoords] = chunk
		}

		localPos := pos.LocalInChunk()
		blockID := chunk.GetBlock(localPos)

		// Проверяем проходимость блока
		_, exists = block.Get(blockID)
		if !exists {
			return false // Блок неизвестного типа считаем непроходимым
		}

		// Проверяем, является ли блок проходимым
		// В реальной реализации у BlockBehavior должен быть метод IsPassable()
		return blockID == block.AirBlockID // Упрощенно: только воздух проходим
	}

	// Проверяем коллизии с блоками
	if !physics.CanMoveToPosition(newPos, collider, blockChecker) {
		return false
	}

	// Проверяем коллизии с другими сущностями
	for otherID, otherData := range bc.entities {
		if otherID == entityID {
			continue // Пропускаем саму сущность
		}

		otherEntity, ok := otherData.(EntityData)
		if !ok {
			continue
		}

		// Создаем коллайдер для другой сущности
		otherCollider := physics.NewBoxCollider(1, 1)

		// Проверяем коллизию
		if physics.CheckBoxCollision(newPos, collider, otherEntity.Position, otherCollider) {
			return false // Есть коллизия с другой сущностью
		}
	}

	return true
}

// handleEvent обрабатывает входящее событие
func (bc *BigChunk) handleEvent(event Event) {
	switch e := event.(type) {
	case TickEvent:
		// Обработка тика (если нужно что-то дополнительное)
	case BlockEvent:
		bc.handleBlockEvent(e)
	case EntityEvent:
		bc.handleEntityEvent(e)
	case SaveEvent:
		bc.saveState(e.Forced)
	}
}

// handleBlockEvent обрабатывает событие, связанное с блоком
func (bc *BigChunk) handleBlockEvent(event BlockEvent) {
	switch event.EventType {
	case EventTypeBlockChange:
		// Изменение блока
		bc.setBlock(event.Position, event.Block)

		// Если изменение влияет на соседние блоки, обрабатываем это
		// Например, для травы проверяем соседние блоки
		if event.Block.ID == block.GrassBlockID {
			// Добавляем блок в список тикаемых, если ещё не добавлен
			bc.mu.Lock()
			bc.tickables[event.Position] = struct{}{}
			bc.mu.Unlock()
		}
	case EventTypeBlockInteract:
		// Взаимодействие с блоком
		// TODO: Реализовать
	}
}

// handleEntityEvent обрабатывает событие сущности
func (bc *BigChunk) handleEntityEvent(event EntityEvent) {
	switch event.EventType {
	case EventTypeEntitySpawn:
		// Создание сущности
		bc.spawnEntity(event)
	case EventTypeEntityDespawn:
		// Удаление сущности
		bc.despawnEntity(event)
	case EventTypeEntityMove:
		// Перемещение сущности
		bc.moveEntity(event)
	case EventTypeEntityInteract:
		// Взаимодействие сущности
		bc.entityInteract(event)
	}
}

// saveState сохраняет состояние всех чанков и сущностей в BigChunk
func (bc *BigChunk) saveState(forced bool) {
	// Сохраняем чанки
	bc.mu.RLock()
	chunks := make([]*Chunk, 0, len(bc.chunks))
	for _, chunk := range bc.chunks {
		chunks = append(chunks, chunk)
	}

	// Копируем сущности для сохранения, чтобы не держать блокировку
	entitiesCopy := make(map[uint64]interface{}, len(bc.entities))
	for id, entity := range bc.entities {
		entitiesCopy[id] = entity
	}
	bc.mu.RUnlock()

	// Отправляем событие сохранения с чанками
	saveEvent := SaveEvent{
		Forced: forced,
		Chunks: chunks,
	}

	// Отправляем событие сохранения
	select {
	case bc.eventsOut <- saveEvent:
		// Успешно отправлено
	default:
		// Канал переполнен, логируем
		// log.Printf("Не удалось отправить событие сохранения: канал переполнен")
	}

	// Отправляем отдельное событие для сохранения сущностей
	if len(entitiesCopy) > 0 {
		entitySaveEvent := EntitySaveEvent{
			BigChunkCoords: bc.coords,
			Entities:       entitiesCopy,
		}

		select {
		case bc.eventsOut <- entitySaveEvent:
			// Успешно отправлено
		default:
			// Канал переполнен, логируем
			// log.Printf("Не удалось отправить событие сохранения сущностей: канал переполнен")
		}
	}
}

// GetChunks возвращает копию карты чанков (для тестирования и отладки)
func (bc *BigChunk) GetChunks() map[vec.Vec2]*Chunk {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	result := make(map[vec.Vec2]*Chunk, len(bc.chunks))
	for k, v := range bc.chunks {
		result[k] = v
	}

	return result
}

// createBlockAPI создаёт экземпляр BlockAPI для использования блоками
func (bc *BigChunk) createBlockAPI() *bigChunkBlockAPI {
	return &bigChunkBlockAPI{
		bigChunk: bc,
		world:    bc.world,
	}
}

// setBlock устанавливает блок по глобальным координатам
func (bc *BigChunk) setBlock(pos vec.Vec2, block Block) {
	chunkCoords := pos.ToChunkCoords()

	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Получаем текущий блок для проверки, нужно ли вызывать OnBreak
	var oldBlock Block
	chunk, exists := bc.chunks[chunkCoords]
	if exists {
		localPos := pos.LocalInChunk()
		oldBlockID := chunk.GetBlock(localPos)
		oldPayload := chunk.GetBlockMetadata(localPos)
		oldBlock = Block{ID: oldBlockID, Payload: oldPayload}
	}

	// Если чанк не существует, создаем его
	if !exists {
		chunk = NewChunk(chunkCoords)
		bc.chunks[chunkCoords] = chunk
	}

	localPos := pos.LocalInChunk()

	// Если старый блок существует, вызываем OnBreak
	if exists && oldBlock.ID != block.ID {
		if behavior, exists := oldBlock.GetBehavior(); exists {
			api := bc.createBlockAPI()
			behavior.OnBreak(api, pos)
		}
	}

	// Устанавливаем блок и его метаданные
	chunk.Blocks[localPos.X][localPos.Y] = block.ID

	// Если есть метаданные - устанавливаем их
	if len(block.Payload) > 0 {
		chunk.SetBlockMetadataMap(localPos, block.Payload)
	}

	// Отмечаем блок как измененный
	if chunk.Changes == nil {
		chunk.Changes = make(map[vec.Vec2]struct{})
	}
	chunk.Changes[localPos] = struct{}{}
	chunk.ChangeCounter++

	// Обновляем список тикаемых блоков
	if block.NeedsTick() {
		bc.tickables[pos] = struct{}{}
	} else {
		delete(bc.tickables, pos)
	}

	// Вызываем OnPlace для нового блока
	if behavior, exists := block.GetBehavior(); exists && oldBlock.ID != block.ID {
		api := bc.createBlockAPI()
		behavior.OnPlace(api, pos)
	}
}

// spawnEntity создает новую сущность в BigChunk
func (bc *BigChunk) spawnEntity(event EntityEvent) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Если ID равен 0, генерируем новый ID
	entityID := event.EntityID
	if entityID == 0 {
		// В реальной реализации ID должен быть уникальным глобально
		entityID = uint64(rand.Int63())
	}

	// Создаем данные сущности
	entityData := EntityData{
		ID:       entityID,
		Type:     uint16(0), // По умолчанию тип 0
		Position: event.Position,
		Metadata: make(map[string]interface{}),
	}

	// Если есть дополнительные данные, обрабатываем их
	if event.Data != nil {
		if data, ok := event.Data.(map[string]interface{}); ok {
			// Копируем данные
			for k, v := range data {
				entityData.Metadata[k] = v
			}

			// Если указан тип, используем его
			if typeVal, ok := data["type"].(uint16); ok {
				entityData.Type = typeVal
			}
		}
	}

	// Добавляем сущность в BigChunk
	bc.entities[entityID] = entityData

	// Отправляем подтверждение создания
	confirmEvent := EntityEvent{
		EventType: EventTypeEntitySpawn,
		EntityID:  entityID,
		Position:  event.Position,
		Data:      entityData,
	}
	bc.eventsOut <- confirmEvent
}

// despawnEntity удаляет сущность из BigChunk
func (bc *BigChunk) despawnEntity(event EntityEvent) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	entityID := event.EntityID

	// Проверяем, существует ли сущность
	if _, exists := bc.entities[entityID]; exists {
		// Удаляем сущность
		delete(bc.entities, entityID)

		// Отправляем подтверждение удаления
		confirmEvent := EntityEvent{
			EventType: EventTypeEntityDespawn,
			EntityID:  entityID,
			Position:  event.Position,
		}
		bc.eventsOut <- confirmEvent
	}
}

// moveEntity перемещает сущность в новую позицию
func (bc *BigChunk) moveEntity(event EntityEvent) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	entityID := event.EntityID

	// Проверяем, существует ли сущность
	if entityData, exists := bc.entities[entityID]; exists {
		if data, ok := entityData.(EntityData); ok {
			// Обновляем позицию
			newPos := event.Position

			// Проверяем, не выходит ли сущность за пределы BigChunk
			if newPos.ToBigChunkCoords() != bc.coords {
				// Сущность перемещается в другой BigChunk
				// В этом случае обработка должна быть на уровне WorldManager
				return
			}

			// Проверяем, можно ли переместиться
			if bc.canEntityMoveTo(entityID, newPos) {
				// Обновляем позицию
				data.Position = newPos
				bc.entities[entityID] = data

				// Отправляем подтверждение перемещения
				confirmEvent := EntityEvent{
					EventType: EventTypeEntityMove,
					EntityID:  entityID,
					Position:  newPos,
					Data:      data,
				}
				bc.eventsOut <- confirmEvent
			}
		}
	}
}

// entityInteract обрабатывает взаимодействие сущности
func (bc *BigChunk) entityInteract(event EntityEvent) {
	// Здесь будет логика взаимодействия сущностей
	// Например, диалоги, торговля, атака и т.д.
}
