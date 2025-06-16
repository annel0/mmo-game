package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
)

// Vec2Client представляет 2D координаты для клиента
type Vec2Client struct {
	X, Y int
}

// ClientBlock содержит информацию о блоке на клиенте
type ClientBlock struct {
	ID       uint32                 `json:"id"`
	Metadata map[string]interface{} `json:"metadata"`
	Version  uint64                 `json:"version"`
}

// ChunkDeltaMessage представляет delta-обновление чанка
type ChunkDeltaMessage struct {
	ChunkCoords  Vec2Client          `json:"chunk_coords"`
	BlockChanges []ClientBlockChange `json:"block_changes"`
	DeltaVersion uint64              `json:"delta_version"`
	CRC32        uint32              `json:"crc32"`
}

// ClientBlockChange представляет изменение блока
type ClientBlockChange struct {
	LocalPos   Vec2Client             `json:"local_pos"`
	BlockID    uint32                 `json:"block_id"`
	Metadata   map[string]interface{} `json:"metadata"`
	ChangeType string                 `json:"change_type"`
}

// BlockDeltaHandler обрабатывает delta-обновления блоков на клиенте
type BlockDeltaHandler struct {
	blockCache map[Vec2Client]*ClientBlock
}

// NewBlockDeltaHandler создаёт новый обработчик
func NewBlockDeltaHandler() *BlockDeltaHandler {
	return &BlockDeltaHandler{
		blockCache: make(map[Vec2Client]*ClientBlock),
	}
}

// ProcessDeltaMessage обрабатывает полученное delta-сообщение
func (bdh *BlockDeltaHandler) ProcessDeltaMessage(deltaData []byte) error {
	var delta ChunkDeltaMessage
	if err := json.Unmarshal(deltaData, &delta); err != nil {
		log.Printf("Ошибка декодирования delta-сообщения: %v", err)
		return err
	}

	log.Printf("Получено delta-обновление чанка %v: %d изменений", delta.ChunkCoords, len(delta.BlockChanges))

	// Обрабатываем каждое изменение блока
	for _, change := range delta.BlockChanges {
		// Вычисляем мировые координаты блока
		worldPos := Vec2Client{
			X: delta.ChunkCoords.X*16 + change.LocalPos.X,
			Y: delta.ChunkCoords.Y*16 + change.LocalPos.Y,
		}

		// Создаём или обновляем блок в кеше
		block := &ClientBlock{
			ID:       change.BlockID,
			Metadata: change.Metadata,
			Version:  delta.DeltaVersion,
		}

		bdh.blockCache[worldPos] = block
		log.Printf("Обновлён блок в позиции %v: ID=%d, тип=%s", worldPos, change.BlockID, change.ChangeType)
	}

	return nil
}

// GetBlock возвращает блок из кеша
func (bdh *BlockDeltaHandler) GetBlock(worldPos Vec2Client) *ClientBlock {
	return bdh.blockCache[worldPos]
}

// GetCachedBlocksCount возвращает количество блоков в кеше
func (bdh *BlockDeltaHandler) GetCachedBlocksCount() int {
	return len(bdh.blockCache)
}

// MockNetworkManager реализует интерфейс NetworkManager для демонстрации
type MockNetworkManager struct {
	clientHandler *BlockDeltaHandler
}

func (mnm *MockNetworkManager) SendBlockUpdate(blockPos vec.Vec2, block world.Block) {
	// Конвертируем в формат, понятный клиенту
	clientPos := Vec2Client{X: blockPos.X, Y: blockPos.Y}
	clientBlock := &ClientBlock{
		ID:       uint32(block.ID),
		Metadata: block.Payload,
		Version:  1,
	}

	// Отправляем "по сети" (в реальности это просто direct call)
	if mnm.clientHandler != nil {
		mnm.clientHandler.blockCache[clientPos] = clientBlock
	}
}

func main() {
	log.Println("=== Демонстрация Delta-обновлений блоков ===")

	// 1. Создаём клиентскую часть
	clientHandler := NewBlockDeltaHandler()

	// 2. Создаём серверную часть
	mockNetwork := &MockNetworkManager{clientHandler: clientHandler}
	worldManager := world.NewWorldManager(12345)
	deltaManager := world.NewBlockDeltaManager(mockNetwork)

	// Запускаем WorldManager
	go worldManager.Run(nil)

	// Запускаем DeltaManager
	deltaManager.Start()
	defer deltaManager.Stop()

	log.Printf("Система запущена. Подписчиков: %d, ожидающих изменений: %d",
		deltaManager.GetSubscribersCount(), deltaManager.GetPendingChangesCount())

	// 3. Подписываем "клиента" на обновления
	deltaManager.Subscribe("client-1", vec.Vec2{X: 0, Y: 0}, 5)

	log.Printf("Клиент подписался. Подписчиков: %d", deltaManager.GetSubscribersCount())

	// 4. Имитируем изменения блоков на сервере
	log.Println("\n--- Имитация изменений блоков ---")

	// Изменение блока в позиции (5, 5)
	pos1 := vec.Vec2{X: 5, Y: 5}
	metadata1 := map[string]interface{}{
		"material": "stone",
		"hardness": 3.5,
	}
	deltaManager.AddBlockChange(pos1, 2, metadata1, "place", 100)
	worldManager.SetBlockMetadataValue(pos1, "material", "stone")
	worldManager.SetBlockMetadataValue(pos1, "hardness", 3.5)

	log.Printf("Добавлено изменение блока в позиции %v", pos1)

	// Изменение блока в позиции (10, 8)
	pos2 := vec.Vec2{X: 10, Y: 8}
	metadata2 := map[string]interface{}{
		"type":   "grass",
		"growth": 0.8,
	}
	deltaManager.AddBlockChange(pos2, 3, metadata2, "update", 100)
	worldManager.SetBlockMetadataValue(pos2, "type", "grass")
	worldManager.SetBlockMetadataValue(pos2, "growth", 0.8)

	log.Printf("Добавлено изменение блока в позиции %v", pos2)

	// 5. Даём время для обработки delta-обновлений
	time.Sleep(200 * time.Millisecond)

	log.Printf("Ожидающих изменений: %d", deltaManager.GetPendingChangesCount())

	// 6. Тестируем получение delta-сообщений
	log.Println("\n--- Тестирование обработки delta-сообщений ---")

	// Создаём тестовое delta-сообщение
	testDelta := ChunkDeltaMessage{
		ChunkCoords: Vec2Client{X: 0, Y: 0},
		BlockChanges: []ClientBlockChange{
			{
				LocalPos:   Vec2Client{X: 5, Y: 5},
				BlockID:    2,
				Metadata:   metadata1,
				ChangeType: "place",
			},
			{
				LocalPos:   Vec2Client{X: 10, Y: 8},
				BlockID:    3,
				Metadata:   metadata2,
				ChangeType: "update",
			},
		},
		DeltaVersion: 1,
		CRC32:        0x12345678,
	}

	// Сериализуем в JSON
	deltaData, err := json.Marshal(testDelta)
	if err != nil {
		log.Fatalf("Ошибка сериализации delta: %v", err)
	}

	log.Printf("Размер delta-сообщения: %d байт", len(deltaData))

	// Обрабатываем на клиенте
	err = clientHandler.ProcessDeltaMessage(deltaData)
	if err != nil {
		log.Fatalf("Ошибка обработки delta: %v", err)
	}

	// 7. Проверяем результаты
	log.Println("\n--- Результаты ---")

	log.Printf("Блоков в кеше клиента: %d", clientHandler.GetCachedBlocksCount())

	// Проверяем конкретные блоки
	block1 := clientHandler.GetBlock(Vec2Client{X: 5, Y: 5})
	if block1 != nil {
		log.Printf("Блок (5,5): ID=%d, metadata=%v", block1.ID, block1.Metadata)
	}

	block2 := clientHandler.GetBlock(Vec2Client{X: 10, Y: 8})
	if block2 != nil {
		log.Printf("Блок (10,8): ID=%d, metadata=%v", block2.ID, block2.Metadata)
	}

	// 8. Тестируем отписку
	log.Println("\n--- Тестирование отписки ---")
	deltaManager.Unsubscribe("client-1")
	log.Printf("Подписчиков после отписки: %d", deltaManager.GetSubscribersCount())

	log.Println("\n=== Демонстрация завершена ===")
}
