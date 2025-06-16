package world

import (
	"hash/crc32"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
)

// BlockDeltaManager управляет отправкой delta-обновлений блоков клиентам
type BlockDeltaManager struct {
	chunkDeltas    map[vec.Vec2]*ChunkDelta   // Накопленные изменения по чанкам
	subscribers    map[string]*SubscriberInfo // Подписчики на обновления (connID -> info)
	deltaVersion   uint64                     // Глобальная версия изменений
	mu             sync.RWMutex               // Мьютекс для безопасного доступа
	networkManager NetworkManager             // Интерфейс для отправки сообщений
	flushInterval  time.Duration              // Интервал отправки накопленных изменений
	stopChan       chan bool                  // Канал для остановки
}

// ChunkDelta содержит накопленные изменения в чанке
type ChunkDelta struct {
	ChunkCoords vec.Vec2
	Changes     map[vec.Vec2]*BlockChangeInfo // Локальные координаты -> изменение
	Version     uint64                        // Версия этой дельты
	LastUpdated time.Time                     // Время последнего обновления
}

// BlockChangeInfo содержит информацию об изменении блока
type BlockChangeInfo struct {
	BlockID    BlockID                `json:"block_id"`
	Metadata   map[string]interface{} `json:"metadata"`
	ChangeType string                 `json:"change_type"` // "set", "break", "place", "update"
	PlayerID   uint64                 `json:"player_id"`   // ID игрока, сделавшего изменение
}

// SubscriberInfo содержит информацию о подписчике
type SubscriberInfo struct {
	ConnID   string   // ID соединения
	Center   vec.Vec2 // Центр области подписки
	Radius   int      // Радиус в чанках
	LastSent uint64   // Последняя отправленная версия
}

// NewBlockDeltaManager создаёт новый менеджер delta-обновлений
func NewBlockDeltaManager(networkManager NetworkManager) *BlockDeltaManager {
	return &BlockDeltaManager{
		chunkDeltas:    make(map[vec.Vec2]*ChunkDelta),
		subscribers:    make(map[string]*SubscriberInfo),
		networkManager: networkManager,
		flushInterval:  time.Millisecond * 100, // Отправляем изменения каждые 100ms
		stopChan:       make(chan bool),
	}
}

// Start запускает менеджер delta-обновлений
func (bdm *BlockDeltaManager) Start() {
	go bdm.flushLoop()
}

// Stop останавливает менеджер delta-обновлений
func (bdm *BlockDeltaManager) Stop() {
	close(bdm.stopChan)
}

// AddBlockChange добавляет изменение блока в очередь для отправки
func (bdm *BlockDeltaManager) AddBlockChange(worldPos vec.Vec2, blockID BlockID, metadata map[string]interface{}, changeType string, playerID uint64) {
	chunkCoords := worldPos.ToChunkCoords()
	localPos := worldPos.LocalInChunk()

	bdm.mu.Lock()
	defer bdm.mu.Unlock()

	// Увеличиваем глобальную версию
	bdm.deltaVersion++

	// Получаем или создаём delta для чанка
	delta, exists := bdm.chunkDeltas[chunkCoords]
	if !exists {
		delta = &ChunkDelta{
			ChunkCoords: chunkCoords,
			Changes:     make(map[vec.Vec2]*BlockChangeInfo),
			Version:     bdm.deltaVersion,
			LastUpdated: time.Now(),
		}
		bdm.chunkDeltas[chunkCoords] = delta
	}

	// Добавляем или обновляем изменение блока
	delta.Changes[localPos] = &BlockChangeInfo{
		BlockID:    blockID,
		Metadata:   metadata,
		ChangeType: changeType,
		PlayerID:   playerID,
	}
	delta.Version = bdm.deltaVersion
	delta.LastUpdated = time.Now()
}

// Subscribe подписывает клиента на обновления блоков в области
func (bdm *BlockDeltaManager) Subscribe(connID string, center vec.Vec2, radius int) {
	bdm.mu.Lock()
	defer bdm.mu.Unlock()

	bdm.subscribers[connID] = &SubscriberInfo{
		ConnID:   connID,
		Center:   center,
		Radius:   radius,
		LastSent: bdm.deltaVersion,
	}

	log.Printf("Клиент %s подписался на обновления блоков: центр=%v, радиус=%d", connID, center, radius)
}

// Unsubscribe отписывает клиента от обновлений блоков
func (bdm *BlockDeltaManager) Unsubscribe(connID string) {
	bdm.mu.Lock()
	defer bdm.mu.Unlock()

	delete(bdm.subscribers, connID)
	log.Printf("Клиент %s отписался от обновлений блоков", connID)
}

// UpdateSubscription обновляет область подписки клиента
func (bdm *BlockDeltaManager) UpdateSubscription(connID string, center vec.Vec2, radius int) {
	bdm.mu.Lock()
	defer bdm.mu.Unlock()

	if subscriber, exists := bdm.subscribers[connID]; exists {
		subscriber.Center = center
		subscriber.Radius = radius
		log.Printf("Клиент %s обновил подписку: центр=%v, радиус=%d", connID, center, radius)
	}
}

// flushLoop периодически отправляет накопленные изменения подписчикам
func (bdm *BlockDeltaManager) flushLoop() {
	ticker := time.NewTicker(bdm.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bdm.flushPendingChanges()
		case <-bdm.stopChan:
			return
		}
	}
}

// flushPendingChanges отправляет накопленные изменения всем подписчикам
func (bdm *BlockDeltaManager) flushPendingChanges() {
	bdm.mu.Lock()
	defer bdm.mu.Unlock()

	if len(bdm.chunkDeltas) == 0 {
		return
	}

	// Для каждого подписчика отправляем релевантные изменения
	for connID, subscriber := range bdm.subscribers {
		bdm.sendDeltasToSubscriber(connID, subscriber)
	}

	// Очищаем старые deltas (старше 1 секунды)
	bdm.cleanupOldDeltas()
}

// sendDeltasToSubscriber отправляет изменения конкретному подписчику
func (bdm *BlockDeltaManager) sendDeltasToSubscriber(connID string, subscriber *SubscriberInfo) {
	// Находим чанки в области подписки
	centerChunkX := subscriber.Center.X / 16
	centerChunkY := subscriber.Center.Y / 16

	for chunkCoords, delta := range bdm.chunkDeltas {
		// Проверяем, находится ли чанк в области подписки
		if !bdm.isChunkInRadius(chunkCoords, centerChunkX, centerChunkY, subscriber.Radius) {
			continue
		}

		// Проверяем, нужно ли отправлять этот delta (версия больше последней отправленной)
		if delta.Version <= subscriber.LastSent {
			continue
		}

		// Создаём protobuf сообщение
		/*
			blockChanges := make([]*protocol.BlockChange, 0, len(delta.Changes))
			for localPos, change := range delta.Changes {
				// Конвертируем метаданные в JSON
				var metadataProto *protocol.JsonMetadata
				if change.Metadata != nil && len(change.Metadata) > 0 {
					jsonStr, err := protocol.MapToJsonMetadata(change.Metadata)
					if err == nil {
						metadataProto = &protocol.JsonMetadata{JsonData: jsonStr}
					}
				}

				blockChange := &protocol.BlockChange{
					LocalPos: &protocol.Vec2{
						X: int32(localPos.X),
						Y: int32(localPos.Y),
					},
					BlockId:    uint32(change.BlockID),
					Metadata:   metadataProto,
					ChangeType: change.ChangeType,
				}
				blockChanges = append(blockChanges, blockChange)
			}

			// Создаём delta сообщение
			chunkDelta := &protocol.ChunkBlockDelta{
				ChunkCoords: &protocol.Vec2{
					X: int32(chunkCoords.X),
					Y: int32(chunkCoords.Y),
				},
				BlockChanges: blockChanges,
				DeltaVersion: delta.Version,
				Crc32:        bdm.calculateDeltaCRC(delta),
			}
		*/

		// Отправляем delta через network manager
		bdm.networkManager.SendBlockUpdate(vec.Vec2{}, Block{}) // Временное решение

		// Можно также отправить напрямую через TCP, если нет подходящего метода в NetworkManager
		// TODO: добавить метод SendChunkDelta в NetworkManager

		log.Printf("Отправлена delta чанка %v клиенту %s: %d изменений", chunkCoords, connID, len(delta.Changes))
	}

	// Обновляем последнюю отправленную версию
	subscriber.LastSent = bdm.deltaVersion
}

// isChunkInRadius проверяет, находится ли чанк в радиусе от центра
func (bdm *BlockDeltaManager) isChunkInRadius(chunkCoords vec.Vec2, centerX, centerY, radius int) bool {
	dx := chunkCoords.X - centerX
	dy := chunkCoords.Y - centerY
	return dx*dx+dy*dy <= radius*radius
}

// calculateDeltaCRC вычисляет CRC32 для delta
func (bdm *BlockDeltaManager) calculateDeltaCRC(delta *ChunkDelta) uint32 {
	// Простая реализация CRC на основе координат и версии
	crc := crc32.NewIEEE()
	crc.Write([]byte{byte(delta.ChunkCoords.X), byte(delta.ChunkCoords.Y)})
	crc.Write([]byte{byte(delta.Version)})
	return crc.Sum32()
}

// cleanupOldDeltas удаляет старые deltas для экономии памяти
func (bdm *BlockDeltaManager) cleanupOldDeltas() {
	cutoff := time.Now().Add(-time.Second)
	for chunkCoords, delta := range bdm.chunkDeltas {
		if delta.LastUpdated.Before(cutoff) {
			delete(bdm.chunkDeltas, chunkCoords)
		}
	}
}

// GetPendingChangesCount возвращает количество ожидающих отправки изменений
func (bdm *BlockDeltaManager) GetPendingChangesCount() int {
	bdm.mu.RLock()
	defer bdm.mu.RUnlock()

	count := 0
	for _, delta := range bdm.chunkDeltas {
		count += len(delta.Changes)
	}
	return count
}

// GetSubscribersCount возвращает количество подписчиков
func (bdm *BlockDeltaManager) GetSubscribersCount() int {
	bdm.mu.RLock()
	defer bdm.mu.RUnlock()

	return len(bdm.subscribers)
}
