package world

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
)

// NetworkManager представляет сетевой менеджер для отправки обновлений клиентам
type NetworkManager interface {
	// SendBlockUpdate отправляет обновление блока всем клиентам в зоне видимости
	SendBlockUpdate(blockPos vec.Vec2, block Block)
}

// WorldManager управляет миром игры и координирует все процессы
type WorldManager struct {
	bigChunks         map[vec.Vec2]*BigChunk                       // Активные BigChunk'и
	globalEvents      chan Event                                   // Глобальные события
	seed              int64                                        // Глобальный сид для генерации
	generator         *WorldGenerator                              // Генератор мира
	currentTick       uint64                                       // Текущий глобальный тик
	lastSaveTime      time.Time                                    // Время последнего сохранения
	saveMu            sync.Mutex                                   // Мьютекс для операций сохранения
	mu                sync.RWMutex                                 // Мьютекс для общего доступа
	dataPath          string                                       // Путь к директории данных
	nextEntityID      uint64                                       // Счетчик для генерации уникальных ID сущностей
	entityIDMu        sync.Mutex                                   // Мьютекс для генерации ID
	ctx               context.Context                              // Контекст для управления жизненным циклом
	cancelFunc        context.CancelFunc                           // Функция отмены контекста
	saveEntitiesFunc  func(vec.Vec2, map[uint64]interface{}) error // Функция для сохранения сущностей
	loadEntitiesFunc  func(vec.Vec2) (interface{}, error)          // Функция для загрузки сущностей
	applyEntitiesFunc func(map[uint64]interface{}, interface{})    // Функция для применения загруженных сущностей
	networkManager    NetworkManager                               // Менеджер сети
}

// NewWorldManager создаёт новый менеджер мира с указанным сидом
func NewWorldManager(seed int64) *WorldManager {
	ctx, cancel := context.WithCancel(context.Background())

	// Создаем генератор мира
	generator := NewWorldGenerator(seed)

	return &WorldManager{
		bigChunks:    make(map[vec.Vec2]*BigChunk),
		globalEvents: make(chan Event, 5000),
		seed:         seed,
		generator:    generator,
		currentTick:  0,
		lastSaveTime: time.Now(),
		nextEntityID: 1000, // Начинаем с 1000, чтобы избежать конфликтов с малыми ID
		ctx:          ctx,
		cancelFunc:   cancel,
	}
}

// InitStorage инициализирует хранилище данных мира
func (wm *WorldManager) InitStorage(dataPath string) error {
	wm.dataPath = dataPath
	return nil
}

// Run запускает обработку событий в WorldManager
func (wm *WorldManager) Run(parentCtx context.Context) {
	// Если parentCtx != nil, создаем новый контекст отменяемый от него
	if parentCtx != nil {
		childCtx, cancel := context.WithCancel(parentCtx)
		wm.ctx = childCtx
		wm.cancelFunc = cancel
	}

	// Запускаем обработку глобальных событий
	go wm.processGlobalEvents()

	// Запускаем автоматическое сохранение мира
	go wm.autoSaveLoop()
}

// processGlobalEvents обрабатывает глобальные события
func (wm *WorldManager) processGlobalEvents() {
	for {
		select {
		case <-wm.ctx.Done():
			return
		case event := <-wm.globalEvents:
			// Обрабатываем событие в зависимости от типа
			switch e := event.(type) {
			case BlockEvent:
				wm.routeBlockEvent(e)
			case EntityEvent:
				wm.routeEntityEvent(e)
			default:
				log.Printf("Неизвестный тип события: %T", event)
			}
		}
	}
}

// autoSaveLoop запускает периодическое сохранение мира
func (wm *WorldManager) autoSaveLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-wm.ctx.Done():
			return
		case <-ticker.C:
			wm.SaveWorld(false)
		}
	}
}

// processTick обрабатывает один глобальный тик
func (wm *WorldManager) processTick() {
	wm.mu.Lock()
	wm.currentTick++
	tickID := wm.currentTick
	wm.mu.Unlock()

	// Создаем событие тика
	tickEvent := TickEvent{
		TickID:    tickID,
		DeltaTime: 1.0 / 60.0,
	}

	// Отправляем тик всем активным BigChunk'ам
	wm.mu.RLock()
	for _, bc := range wm.bigChunks {
		select {
		case bc.eventsIn <- tickEvent:
			// Успешно отправлено
		default:
			// Канал переполнен, пропускаем (можно добавить логирование)
		}
	}
	wm.mu.RUnlock()
}

// handleEvent обрабатывает глобальное событие
func (wm *WorldManager) handleEvent(event Event) {
	switch e := event.(type) {
	case BlockEvent:
		wm.routeBlockEvent(e)
	case EntityEvent:
		wm.routeEntityEvent(e)
	case SaveEvent:
		wm.processSaveEvent(e)
	case EntitySaveEvent:
		wm.processSaveEvent(e)
	}
}

// processSaveEvent обрабатывает событие сохранения
func (wm *WorldManager) processSaveEvent(event Event) {
	switch e := event.(type) {
	case SaveEvent:
		// Принудительное сохранение или по расписанию
		wm.SaveWorld(e.Forced)

		// Если в событии есть чанки для сохранения, сохраняем их
		if len(e.Chunks) > 0 {
			// В реальной реализации нужно использовать storage.WorldStorage
			// для сохранения конкретных чанков
		}
	case EntitySaveEvent:
		// Сохранение сущностей
		wm.SaveEntities(e.BigChunkCoords, e.Entities)
	}
}

// GenerateEntityID генерирует уникальный ID для сущности
func (wm *WorldManager) GenerateEntityID() uint64 {
	wm.entityIDMu.Lock()
	defer wm.entityIDMu.Unlock()

	wm.nextEntityID++
	return wm.nextEntityID
}

// SetStorageFunctions устанавливает функции для работы с хранилищем сущностей
func (wm *WorldManager) SetStorageFunctions(
	saveFunc func(vec.Vec2, map[uint64]interface{}) error,
	loadFunc func(vec.Vec2) (interface{}, error),
	applyFunc func(map[uint64]interface{}, interface{}),
) {
	wm.saveEntitiesFunc = saveFunc
	wm.loadEntitiesFunc = loadFunc
	wm.applyEntitiesFunc = applyFunc
}

// SaveEntities сохраняет сущности из BigChunk
func (wm *WorldManager) SaveEntities(bigChunkCoords vec.Vec2, entities map[uint64]interface{}) {
	if wm.saveEntitiesFunc != nil {
		if err := wm.saveEntitiesFunc(bigChunkCoords, entities); err != nil {
			log.Printf("Ошибка сохранения сущностей для BigChunk %v: %v", bigChunkCoords, err)
		}
	}
}

// routeBlockEvent маршрутизирует событие блока в соответствующий BigChunk
func (wm *WorldManager) routeBlockEvent(event BlockEvent) {
	targetCoords := event.Position.ToBigChunkCoords()

	wm.mu.RLock()
	targetChunk, exists := wm.bigChunks[targetCoords]
	wm.mu.RUnlock()

	if !exists {
		// Если BigChunk не существует, создаём его
		wm.mu.Lock()
		// Проверяем еще раз под блокировкой записи
		targetChunk, exists = wm.bigChunks[targetCoords]
		if !exists {
			targetChunk = wm.createBigChunk(targetCoords)
		}
		wm.mu.Unlock()
	}

	// Отправляем событие в BigChunk
	select {
	case targetChunk.eventsIn <- event:
		// Успешно отправлено
	default:
		// Канал переполнен, логируем
		log.Printf("Переполнен канал событий для BigChunk %v, событие блока отброшено", targetChunk.coords)
	}

	// Если это событие изменения блока, уведомляем NetworkManager
	if event.EventType == EventTypeBlockChange && wm.networkManager != nil {
		// Отправляем уведомление об изменении блока клиентам
		wm.networkManager.SendBlockUpdate(event.Position, event.Block)
	}
}

// routeEntityEvent маршрутизирует событие сущности в соответствующий BigChunk
func (wm *WorldManager) routeEntityEvent(event EntityEvent) {
	// Аналогично routeBlockEvent
	targetCoords := event.Position.ToBigChunkCoords()

	wm.mu.RLock()
	targetChunk, exists := wm.bigChunks[targetCoords]
	wm.mu.RUnlock()

	if !exists {
		wm.mu.Lock()
		targetChunk, exists = wm.bigChunks[targetCoords]
		if !exists {
			targetChunk = wm.createBigChunk(targetCoords)
		}
		wm.mu.Unlock()
	}

	select {
	case targetChunk.eventsIn <- event:
		// Успешно отправлено
	default:
		// Канал переполнен, логируем
		log.Printf("Переполнен канал событий для BigChunk %v, событие сущности отброшено", targetChunk.coords)
	}
}

// createBigChunk создаёт новый BigChunk и запускает его
func (wm *WorldManager) createBigChunk(coords vec.Vec2) *BigChunk {
	bigChunk := NewBigChunk(coords, wm, wm.globalEvents)
	wm.bigChunks[coords] = bigChunk

	// Запускаем BigChunk в отдельной горутине
	go bigChunk.Run(wm.ctx)

	// Загружаем сущности из хранилища (если есть)
	wm.loadEntities(bigChunk)

	return bigChunk
}

// loadEntities загружает сохраненные сущности для BigChunk
func (wm *WorldManager) loadEntities(bigChunk *BigChunk) {
	if wm.loadEntitiesFunc == nil || wm.applyEntitiesFunc == nil {
		return // Функции не установлены
	}

	entitiesData, err := wm.loadEntitiesFunc(bigChunk.coords)
	if err != nil {
		log.Printf("Ошибка загрузки сущностей для BigChunk %v: %v", bigChunk.coords, err)
		return
	}

	if entitiesData != nil {
		// Применяем данные к BigChunk
		bigChunk.mu.Lock()
		wm.applyEntitiesFunc(bigChunk.entities, entitiesData)
		bigChunk.mu.Unlock()
	}
}

// SaveWorld сохраняет все активные чанки и метаданные мира
func (wm *WorldManager) SaveWorld(force bool) {
	wm.saveMu.Lock()
	defer wm.saveMu.Unlock()

	// Проверяем, нужно ли сохранять
	if !force && time.Since(wm.lastSaveTime) < time.Minute {
		return // Сохранение было недавно, пропускаем
	}

	log.Printf("Начато сохранение мира...")

	// Сохраняем все активные BigChunk'и
	wm.mu.RLock()
	for coords, bigChunk := range wm.bigChunks {
		// Отправляем событие сохранения в BigChunk
		saveEvent := SaveEvent{Forced: force}
		select {
		case bigChunk.eventsIn <- saveEvent:
			// Успешно отправлено
		default:
			log.Printf("Переполнен канал событий для BigChunk %v, событие сохранения отброшено", bigChunk.coords)
		}

		// Сохраняем сущности
		bigChunk.mu.RLock()
		entities := make(map[uint64]interface{})
		for id, entity := range bigChunk.entities {
			entities[id] = entity
		}
		bigChunk.mu.RUnlock()

		wm.SaveEntities(coords, entities)
	}
	wm.mu.RUnlock()

	wm.lastSaveTime = time.Now()
	log.Printf("Сохранение мира завершено")
}

// GetBlock возвращает блок по глобальным координатам
func (wm *WorldManager) GetBlock(pos vec.Vec2) Block {
	return wm.GetBlockLayer(pos, LayerActive)
}

// GetBlockLayer возвращает блок на указанном слое.
func (wm *WorldManager) GetBlockLayer(pos vec.Vec2, layer BlockLayer) Block {
	bigChunkCoords := pos.ToBigChunkCoords()

	wm.mu.RLock()
	bigChunk, exists := wm.bigChunks[bigChunkCoords]
	wm.mu.RUnlock()

	if !exists {
		wm.mu.Lock()
		bigChunk, exists = wm.bigChunks[bigChunkCoords]
		if !exists {
			bigChunk = wm.createBigChunk(bigChunkCoords)
		}
		wm.mu.Unlock()
	}

	chunkCoords := pos.ToChunkCoords()
	localPos := pos.LocalInChunk()

	bigChunk.mu.RLock()
	chunk, exists := bigChunk.chunks[chunkCoords]
	bigChunk.mu.RUnlock()

	if !exists {
		chunk = wm.generateChunk(chunkCoords)
		bigChunk.mu.Lock()
		if _, exists := bigChunk.chunks[chunkCoords]; !exists {
			bigChunk.chunks[chunkCoords] = chunk
		}
		bigChunk.mu.Unlock()
	}

	chunk.Mu.RLock()
	blockID := chunk.GetBlockLayer(layer, localPos)
	var metadata map[string]interface{}

	metadata = chunk.GetBlockMetadataLayer(layer, localPos)
	chunk.Mu.RUnlock()

	return Block{ID: blockID, Payload: metadata}
}

// SetBlock устанавливает блок по глобальным координатам
func (wm *WorldManager) SetBlock(pos vec.Vec2, block Block) {
	wm.SetBlockLayer(pos, LayerActive, block)
}

// SetBlockLayer устанавливает блок на указанном слое (пока без событий).
func (wm *WorldManager) SetBlockLayer(pos vec.Vec2, layer BlockLayer, block Block) {
	bigChunkCoords := pos.ToBigChunkCoords()

	wm.mu.RLock()
	bigChunk, exists := wm.bigChunks[bigChunkCoords]
	wm.mu.RUnlock()

	if !exists {
		wm.mu.Lock()
		bigChunk, exists = wm.bigChunks[bigChunkCoords]
		if !exists {
			bigChunk = wm.createBigChunk(bigChunkCoords)
		}
		wm.mu.Unlock()
	}

	chunkCoords := pos.ToChunkCoords()
	localPos := pos.LocalInChunk()

	bigChunk.mu.RLock()
	chunk, exists := bigChunk.chunks[chunkCoords]
	bigChunk.mu.RUnlock()

	if !exists {
		chunk = wm.generateChunk(chunkCoords)
		bigChunk.mu.Lock()
		if _, exists := bigChunk.chunks[chunkCoords]; !exists {
			bigChunk.chunks[chunkCoords] = chunk
		}
		bigChunk.mu.Unlock()
	}

	chunk.SetBlockLayer(layer, localPos, block.ID)

	if len(block.Payload) > 0 {
		chunk.SetBlockMetadataLayer(layer, localPos, "payload", block.Payload) // simplistic
	}
}

// HandleEntityEvent обрабатывает глобальное событие сущности
func (wm *WorldManager) HandleEntityEvent(event EntityEvent) {
	// Отправляем событие в globalEvents для обработки
	select {
	case wm.globalEvents <- event:
		// Успешно отправлено
	default:
		// Канал переполнен, можно добавить логирование
	}
}

// ProcessEntityMovement обрабатывает перемещение сущности между BigChunk'ами
func (wm *WorldManager) ProcessEntityMovement(entityID uint64, oldPos, newPos vec.Vec2) {
	// Получаем координаты BigChunk для старой и новой позиции
	oldBCCoords := oldPos.ToBigChunkCoords()
	newBCCoords := newPos.ToBigChunkCoords()

	// Если BigChunk не изменился, ничего не делаем
	if oldBCCoords == newBCCoords {
		return
	}

	// Отправляем событие выхода из старого BigChunk
	exitEvent := EntityEvent{
		EventType:   EventTypeEntityDespawn,
		EntityID:    entityID,
		Position:    oldPos,
		SourceChunk: oldBCCoords,
		TargetChunk: oldBCCoords,
	}

	// Отправляем событие входа в новый BigChunk
	enterEvent := EntityEvent{
		EventType:   EventTypeEntitySpawn,
		EntityID:    entityID,
		Position:    newPos,
		SourceChunk: newBCCoords,
		TargetChunk: newBCCoords,
	}

	// Маршрутизируем события
	wm.routeEntityEvent(exitEvent)
	wm.routeEntityEvent(enterEvent)
}

// SpawnEntity создает новую сущность в мире
func (wm *WorldManager) SpawnEntity(entityType uint16, position vec.Vec2, entityData interface{}) uint64 {
	// Генерируем новый ID для сущности
	entityID := wm.GenerateEntityID()

	// Создаем событие создания сущности
	spawnEvent := EntityEvent{
		EventType:   EventTypeEntitySpawn,
		EntityID:    entityID, // Используем сгенерированный ID
		Position:    position,
		TargetChunk: position.ToBigChunkCoords(),
		Data:        entityData,
	}

	// Маршрутизируем событие
	wm.routeEntityEvent(spawnEvent)

	return entityID
}

// DespawnEntity удаляет сущность из мира
func (wm *WorldManager) DespawnEntity(entityID uint64, position vec.Vec2) {
	// Создаем событие удаления сущности
	despawnEvent := EntityEvent{
		EventType:   EventTypeEntityDespawn,
		EntityID:    entityID,
		Position:    position,
		SourceChunk: position.ToBigChunkCoords(),
		TargetChunk: position.ToBigChunkCoords(),
	}

	// Маршрутизируем событие
	wm.routeEntityEvent(despawnEvent)
}

// Stop останавливает все процессы WorldManager
func (wm *WorldManager) Stop() {
	// Принудительное сохранение при завершении
	wm.SaveWorld(true)

	// Отменяем контекст, что приведет к остановке всех BigChunk
	wm.cancelFunc()
}

// generateChunk генерирует новый чанк с указанными координатами
func (wm *WorldManager) generateChunk(coords vec.Vec2) *Chunk {
	// Используем генератор мира для создания чанка
	return wm.generator.GenerateChunk(coords)
}

// GetChunk возвращает чанк по координатам
func (wm *WorldManager) GetChunk(coords vec.Vec2) *Chunk {
	// Получаем координаты BigChunk, в котором находится чанк
	bigChunkCoords := vec.Vec2{
		X: (coords.X >> 4) * 4, // Преобразуем координаты чанка в координаты BigChunk
		Y: (coords.Y >> 4) * 4,
	}

	wm.mu.RLock()
	bigChunk, exists := wm.bigChunks[bigChunkCoords]
	wm.mu.RUnlock()

	if !exists {
		// Если BigChunk не существует, создаём его
		wm.mu.Lock()
		// Проверяем еще раз под блокировкой записи
		bigChunk, exists = wm.bigChunks[bigChunkCoords]
		if !exists {
			bigChunk = wm.createBigChunk(bigChunkCoords)
		}
		wm.mu.Unlock()
	}

	// Получаем чанк из BigChunk
	bigChunk.mu.RLock()
	chunk, exists := bigChunk.chunks[coords]
	bigChunk.mu.RUnlock()

	if !exists {
		// Если чанк не существует, генерируем его
		chunk = wm.generateChunk(coords)

		bigChunk.mu.Lock()
		// Проверяем еще раз под блокировкой записи
		_, exists := bigChunk.chunks[coords]
		if !exists {
			bigChunk.chunks[coords] = chunk
		}
		bigChunk.mu.Unlock()
	}

	return chunk
}

// SetNetworkManager устанавливает сетевой менеджер для отправки обновлений клиентам
func (wm *WorldManager) SetNetworkManager(networkManager NetworkManager) {
	wm.networkManager = networkManager
}
