package network

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/block"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// GameHandler обрабатывает игровые сообщения и взаимодействует с миром
type GameHandler struct {
	gameWorld      *world.WorldManager   // Менеджер игрового мира
	entityManager  *entity.EntityManager // Менеджер сущностей
	server         *GameServer           // Ссылка на сервер
	playerEntities map[string]uint64     // Мапа client.id -> entity.ID
	lastEntityID   uint64                // Счетчик для генерации entity ID
	mu             sync.RWMutex          // Мьютекс для синхронизации
}

// NewGameHandler создает новый обработчик игры
func NewGameHandler(worldManager *world.WorldManager, entityManager *entity.EntityManager) *GameHandler {
	handler := &GameHandler{
		gameWorld:      worldManager,
		entityManager:  entityManager,
		playerEntities: make(map[string]uint64),
		lastEntityID:   0,
	}

	// Регистрируем GameHandler как NetworkManager для WorldManager
	worldManager.SetNetworkManager(handler)

	return handler
}

// SetServer устанавливает ссылку на сервер
func (gh *GameHandler) SetServer(server *GameServer) {
	gh.server = server
}

// HandleMessage обрабатывает входящие сообщения от клиентов
func (gh *GameHandler) HandleMessage(client *Client, message *Message) error {
	switch message.Type {
	case MsgTypeAuth:
		return gh.handleAuth(client, message)
	case MsgTypeMovement:
		return gh.handleMovement(client, message)
	case MsgTypeAction:
		return gh.handleAction(client, message)
	case MsgTypeChat:
		return gh.handleChat(client, message)
	case MsgTypeBlockInteract:
		return gh.handleBlockInteract(client, message)
	case MsgTypeEntityInteract:
		return gh.handleEntityInteract(client, message)
	case MsgTypeInventoryAction:
		return gh.handleInventoryAction(client, message)
	case MsgTypeChunkRequest:
		return gh.handleChunkRequest(client, message)
	default:
		return fmt.Errorf("unknown message type: %s", message.Type)
	}
}

// OnClientConnect вызывается при подключении клиента
func (gh *GameHandler) OnClientConnect(client *Client) {
	// Ничего не делаем, пока клиент не аутентифицируется
	log.Printf("Client connected: %s", client.id)
}

// OnClientDisconnect вызывается при отключении клиента
func (gh *GameHandler) OnClientDisconnect(client *Client) {
	// Удаляем игрока из мира
	gh.mu.Lock()
	if entityID, exists := gh.playerEntities[client.id]; exists {
		// Удаляем сущность
		gh.DespawnEntity(entityID)
		delete(gh.playerEntities, client.id)

		// Оповещаем других игроков об удалении
		despawnMsg, _ := NewMessage(MsgTypeEntityDespawn, EntityDespawnMessage{
			EntityID: entityID,
			Reason:   "disconnected",
		})
		gh.server.BroadcastMessage(despawnMsg)
	}
	gh.mu.Unlock()

	log.Printf("Client disconnected: %s", client.id)
}

// Tick обновляет состояние игрового мира
func (gh *GameHandler) Tick(dt float64) {
	// Обновляем все сущности
	gh.entityManager.UpdateEntities(dt, gh)

	// Отправляем обновления игрокам
	gh.sendWorldUpdates()
}

// GetBlock реализует интерфейс EntityAPI
func (gh *GameHandler) GetBlock(pos vec.Vec2) block.BlockID {
	// Получаем блок из мира
	worldBlock := gh.gameWorld.GetBlock(pos)
	return worldBlock.ID
}

// SetBlock реализует интерфейс EntityAPI
func (gh *GameHandler) SetBlock(pos vec.Vec2, id block.BlockID) {
	// Создаем блок для мира
	worldBlock := world.NewBlock(id)

	// Устанавливаем блок в мире
	// WorldManager сам отправит обновление через NetworkManager (этот GameHandler)
	gh.gameWorld.SetBlock(pos, worldBlock)

	// Не отправляем обновление здесь, оно будет отправлено через SendBlockUpdate
}

// GetBlockMetadata реализует интерфейс EntityAPI
func (gh *GameHandler) GetBlockMetadata(pos vec.Vec2, key string) interface{} {
	// Получаем блок из мира
	worldBlock := gh.gameWorld.GetBlock(pos)

	// Получаем метаданные
	if worldBlock.Payload != nil {
		if value, exists := worldBlock.Payload[key]; exists {
			return value
		}
	}

	return nil
}

// SetBlockMetadata реализует интерфейс EntityAPI
func (gh *GameHandler) SetBlockMetadata(pos vec.Vec2, key string, value interface{}) {
	// Получаем текущий блок
	worldBlock := gh.gameWorld.GetBlock(pos)

	// Обновляем метаданные
	if worldBlock.Payload == nil {
		worldBlock.Payload = make(map[string]interface{})
	}

	worldBlock.Payload[key] = value

	// Устанавливаем обновленный блок
	// WorldManager сам отправит обновление через NetworkManager (этот GameHandler)
	gh.gameWorld.SetBlock(pos, worldBlock)
}

// GetEntitiesInRange реализует интерфейс EntityAPI
func (gh *GameHandler) GetEntitiesInRange(center vec.Vec2, radius float64) []*entity.Entity {
	return gh.entityManager.GetEntitiesInRange(center, radius)
}

// SpawnEntity реализует интерфейс EntityAPI
func (gh *GameHandler) SpawnEntity(entityType entity.EntityType, position vec.Vec2) uint64 {
	// Создаем новую сущность
	entityID := gh.entityManager.SpawnEntity(entityType, position, gh)

	// Получаем сущность
	ent, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return 0
	}

	// Информируем WorldManager о новой сущности
	entityData := map[string]interface{}{
		"type":      uint16(entityType),
		"velocity":  ent.Velocity,
		"direction": ent.Direction,
		"payload":   ent.Payload,
	}

	// Создаем событие для WorldManager
	gh.gameWorld.SpawnEntity(uint16(entityType), position, entityData)

	// Отправляем сообщение о создании сущности всем клиентам
	spawnMsg, _ := NewMessage(MsgTypeEntitySpawn, EntitySpawnMessage{
		Entity: EntityData{
			ID:         entityID,
			Type:       entityType,
			Position:   position,
			Direction:  ent.Direction,
			Active:     true,
			Attributes: ent.Payload,
		},
	})
	gh.server.BroadcastMessage(spawnMsg)

	return entityID
}

// DespawnEntity реализует интерфейс EntityAPI
func (gh *GameHandler) DespawnEntity(entityID uint64) {
	// Получаем сущность перед удалением для получения позиции
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return
	}

	position := entity.Position

	// Удаляем сущность из EntityManager
	gh.entityManager.DespawnEntity(entityID, gh)

	// Информируем WorldManager об удалении сущности
	gh.gameWorld.DespawnEntity(entityID, position)

	// Оповещаем клиентов
	despawnMsg, _ := NewMessage(MsgTypeEntityDespawn, EntityDespawnMessage{
		EntityID: entityID,
		Reason:   "despawned",
	})
	gh.server.BroadcastMessage(despawnMsg)
}

// MoveEntity реализует интерфейс EntityAPI
func (gh *GameHandler) MoveEntity(entity *entity.Entity, direction entity.MovementDirection, dt float64) bool {
	// Обрабатываем движение сущности
	moved := gh.entityManager.ProcessMovement(entity.ID, direction, dt, gh)
	return moved
}

// SendMessage реализует интерфейс EntityAPI
func (gh *GameHandler) SendMessage(entityID uint64, messageType string, data interface{}) {
	// Находим клиента, связанного с этой сущностью
	var clientID string

	gh.mu.RLock()
	for cid, eid := range gh.playerEntities {
		if eid == entityID {
			clientID = cid
			break
		}
	}
	gh.mu.RUnlock()

	if clientID == "" {
		return // Сущность не связана с клиентом
	}

	// Отправляем сообщение клиенту
	msg, _ := NewMessage(messageType, data)
	gh.server.SendToClient(clientID, msg)
}

// GetBehavior реализует интерфейс EntityAPI
func (gh *GameHandler) GetBehavior(entityType entity.EntityType) (entity.EntityBehavior, bool) {
	return gh.entityManager.GetBehavior(entityType)
}

// Обработчики сообщений

// handleAuth обрабатывает запрос на аутентификацию
func (gh *GameHandler) handleAuth(client *Client, message *Message) error {
	var authReq AuthRequest
	if err := json.Unmarshal(message.Data, &authReq); err != nil {
		return err
	}

	// В реальном проекте здесь будет проверка учетных данных
	// Для примера считаем, что аутентификация всегда успешна

	// Создаем игрока
	playerPos := vec.Vec2{X: 0, Y: 0} // Начальная позиция
	entityID := gh.SpawnEntity(entity.EntityTypePlayer, playerPos)

	// Связываем клиента с сущностью
	gh.mu.Lock()
	gh.playerEntities[client.id] = entityID
	gh.mu.Unlock()

	// Устанавливаем имя игрока
	playerEntity, _ := gh.entityManager.GetEntity(entityID)
	playerEntity.Payload["username"] = authReq.Username

	// Устанавливаем флаг аутентификации
	gh.server.SetClientAuthenticated(client.id, entityID, true)

	// Отправляем ответ клиенту
	authResp := AuthResponse{
		Success:   true,
		Message:   "Authentication successful",
		PlayerID:  entityID,
		Token:     "dummy-token", // В реальном проекте здесь будет настоящий токен
		WorldName: "GameWorld",
	}

	respMsg, _ := NewMessage(MsgTypeAuthResponse, authResp)
	if err := gh.server.SendToClient(client.id, respMsg); err != nil {
		return err
	}

	// Отправляем данные о мире
	gh.sendWorldToClient(client)

	// Оповещаем других игроков о новом игроке
	spawnMsg, _ := NewMessage(MsgTypeEntitySpawn, EntitySpawnMessage{
		Entity: EntityData{
			ID:         entityID,
			Type:       entity.EntityTypePlayer,
			Position:   playerPos,
			Direction:  playerEntity.Direction,
			Active:     true,
			Attributes: playerEntity.Payload,
		},
	})
	gh.server.BroadcastMessage(spawnMsg)

	log.Printf("Player %s authenticated with username %s", client.id, authReq.Username)
	return nil
}

// handleMovement обрабатывает запрос на перемещение
func (gh *GameHandler) handleMovement(client *Client, message *Message) error {
	var moveReq MovementRequest
	if err := json.Unmarshal(message.Data, &moveReq); err != nil {
		return err
	}

	// Получаем сущность игрока
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем сущность
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return fmt.Errorf("entity not found")
	}

	// Получаем старую позицию для отслеживания перехода между BigChunk'ами
	oldPos := entity.Position

	// Обрабатываем движение (dt = 1/60 секунды или можно использовать значение из клиента)
	dt := 1.0 / 60.0
	moved := gh.MoveEntity(entity, moveReq.Direction, dt)

	if moved {
		// Если сущность переместилась, проверяем переход между BigChunk'ами
		newPos := entity.Position
		gh.gameWorld.ProcessEntityMovement(entityID, oldPos, newPos)

		// Отправляем обновление всем клиентам
		updateMsg, _ := NewMessage(MsgTypeEntityUpdate, EntityUpdateMessage{
			Entities: []EntityData{
				{
					ID:        entityID,
					Type:      entity.Type,
					Position:  entity.Position,
					Velocity:  entity.Velocity,
					Direction: entity.Direction,
					Active:    entity.Active,
				},
			},
		})
		gh.server.BroadcastMessage(updateMsg)
	}

	return nil
}

// handleAction обрабатывает запрос на выполнение действия
func (gh *GameHandler) handleAction(client *Client, message *Message) error {
	var actionReq ActionRequest
	if err := json.Unmarshal(message.Data, &actionReq); err != nil {
		return err
	}

	// Получаем сущность игрока
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем сущность
	playerEntity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return fmt.Errorf("entity not found")
	}

	// Обрабатываем различные типы действий
	switch actionReq.Action {
	case ActionAttack:
		// Логика атаки (пример для игрока)
		playerBehavior, ok := gh.GetBehavior(entity.EntityTypePlayer)
		if !ok {
			return fmt.Errorf("player behavior not found")
		}

		// Проверяем, что это конкретно поведение игрока
		if pb, ok := playerBehavior.(*entity.PlayerBehavior); ok {
			pb.Attack(gh, playerEntity)

			// Отправляем игровое событие всем клиентам
			eventMsg, _ := NewMessage(MsgTypeGameEvent, GameEventMessage{
				EventType: "player_attack",
				EntityID:  entityID,
				Position:  playerEntity.Position,
				Parameters: map[string]interface{}{
					"direction": playerEntity.Direction,
				},
			})
			gh.server.BroadcastMessage(eventMsg)
		}

	case ActionUseItem:
		// Логика использования предмета
		// ...

	case ActionBuildPlace:
		// Логика размещения блока
		pos := actionReq.Position
		blockID := block.BlockID(1) // ID блока из запроса
		gh.SetBlock(pos, blockID)

	case ActionBuildBreak:
		// Логика разрушения блока
		pos := actionReq.Position
		gh.SetBlock(pos, block.BlockID(0)) // 0 - воздух
	}

	return nil
}

// handleChat обрабатывает сообщения чата
func (gh *GameHandler) handleChat(client *Client, message *Message) error {
	var chatMsg ChatMessage
	if err := json.Unmarshal(message.Data, &chatMsg); err != nil {
		return err
	}

	// Получаем имя отправителя
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем сущность
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return fmt.Errorf("entity not found")
	}

	username, _ := entity.Payload["username"].(string)
	if username == "" {
		username = "Player"
	}

	// Создаем широковещательное сообщение
	broadcastMsg := ChatBroadcastMessage{
		SenderID:   entityID,
		SenderName: username,
		Content:    chatMsg.Content,
		Channel:    chatMsg.Channel,
		Timestamp:  message.Timestamp,
	}

	// Отправляем всем или конкретному игроку
	if chatMsg.TargetID != 0 {
		// Приватное сообщение
		msg, _ := NewMessage(MsgTypeChatBroadcast, broadcastMsg)
		gh.server.SendToPlayer(chatMsg.TargetID, msg)
		// Отправляем копию отправителю
		gh.server.SendToClient(client.id, msg)
	} else {
		// Публичное сообщение
		msg, _ := NewMessage(MsgTypeChatBroadcast, broadcastMsg)
		gh.server.BroadcastMessage(msg)
	}

	return nil
}

// handleBlockInteract обрабатывает взаимодействие с блоком
func (gh *GameHandler) handleBlockInteract(client *Client, message *Message) error {
	var blockReq BlockInteractRequest
	if err := json.Unmarshal(message.Data, &blockReq); err != nil {
		return err
	}

	// Получаем сущность игрока
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем сущность
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return fmt.Errorf("entity not found")
	}

	// Проверяем, находится ли блок в зоне досягаемости
	blockPos := blockReq.Position
	entityPos := entity.Position

	// Примитивная проверка дистанции (в реальном проекте будет более сложная)
	if vec.FromVec2(entityPos).DistanceTo(vec.FromVec2(blockPos)) > 3.0 {
		// Блок слишком далеко
		return fmt.Errorf("block is too far")
	}

	// Обрабатываем взаимодействие
	switch blockReq.Action {
	case "break":
		// Разрушение блока
		gh.SetBlock(blockPos, block.BlockID(0)) // 0 - воздух

	case "place":
		// Размещение блока
		blockID := block.BlockID(1) // Получаем из запроса или предмета
		gh.SetBlock(blockPos, blockID)

	case "interact":
		// Взаимодействие с блоком (например, открытие сундука)
		// Получаем текущий блок
		blockID := gh.GetBlock(blockPos)

		// Обрабатываем в зависимости от типа блока
		// Здесь может быть логика для разных типов блоков

		// Оповещаем клиента о результате взаимодействия
		eventMsg, _ := NewMessage(MsgTypeGameEvent, GameEventMessage{
			EventType: "block_interaction",
			EntityID:  entityID,
			Position:  blockPos,
			Parameters: map[string]interface{}{
				"block_id": blockID,
				"action":   blockReq.Action,
			},
		})
		gh.server.SendToClient(client.id, eventMsg)
	}

	return nil
}

// handleEntityInteract обрабатывает взаимодействие с сущностью
func (gh *GameHandler) handleEntityInteract(client *Client, message *Message) error {
	var entityReq EntityInteractRequest
	if err := json.Unmarshal(message.Data, &entityReq); err != nil {
		return err
	}

	// Получаем сущность игрока
	gh.mu.RLock()
	playerEntityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем сущность игрока
	playerEntity, exists := gh.entityManager.GetEntity(playerEntityID)
	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем целевую сущность
	targetEntity, exists := gh.entityManager.GetEntity(entityReq.EntityID)
	if !exists {
		return fmt.Errorf("target entity not found")
	}

	// Проверяем, находится ли сущность в зоне досягаемости
	playerPos := playerEntity.PrecisePos
	targetPos := targetEntity.PrecisePos

	// Проверка дистанции
	if playerPos.DistanceTo(targetPos) > 3.0 {
		// Сущность слишком далеко
		return fmt.Errorf("entity is too far")
	}

	// Обрабатываем взаимодействие
	switch entityReq.Action {
	case "talk":
		// Разговор с NPC
		if targetEntity.Type == entity.EntityTypeNPC {
			// Логика диалога
			// ...

			// Отправляем сообщение игроку
			eventMsg, _ := NewMessage(MsgTypeGameEvent, GameEventMessage{
				EventType: "npc_talk",
				EntityID:  entityReq.EntityID,
				Parameters: map[string]interface{}{
					"message": "Hello, traveler!",
				},
			})
			gh.server.SendToClient(client.id, eventMsg)
		}

	case "trade":
		// Торговля с NPC
		if targetEntity.Type == entity.EntityTypeNPC {
			// Проверяем, является ли NPC торговцем
			if npcType, ok := targetEntity.Payload["npcType"].(string); ok && npcType == "trader" {
				// Получаем инвентарь торговца
				inventory, _ := targetEntity.Payload["inventory"].(map[string]interface{})
				prices, _ := targetEntity.Payload["prices"].(map[string]interface{})

				// Отправляем сообщение игроку
				eventMsg, _ := NewMessage(MsgTypeGameEvent, GameEventMessage{
					EventType: "npc_trade",
					EntityID:  entityReq.EntityID,
					Parameters: map[string]interface{}{
						"inventory": inventory,
						"prices":    prices,
					},
				})
				gh.server.SendToClient(client.id, eventMsg)
			}
		}
	}

	return nil
}

// handleInventoryAction обрабатывает действия с инвентарем
func (gh *GameHandler) handleInventoryAction(client *Client, message *Message) error {
	var invReq InventoryActionRequest
	if err := json.Unmarshal(message.Data, &invReq); err != nil {
		return err
	}

	// Получаем сущность игрока
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем сущность
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return fmt.Errorf("entity not found")
	}

	// Получаем инвентарь игрока
	inventory, ok := entity.Payload["inventory"].(map[string]interface{})
	if !ok {
		// Создаем инвентарь, если его нет
		inventory = make(map[string]interface{})
		entity.Payload["inventory"] = inventory
	}

	// Обрабатываем действия с инвентарем
	switch invReq.Action {
	case "move":
		// Перемещение предмета между слотами
		// ...

	case "split":
		// Разделение стопки предметов
		// ...

	case "drop":
		// Выбрасывание предмета
		// ...
	}

	// Отправляем обновление инвентаря
	slots := make(map[int]InventoryItem)

	// Заполняем слоты из инвентаря игрока
	// В реальном коде тут будет конвертация из внутреннего формата в формат сообщения

	invMsg, _ := NewMessage(MsgTypePlayerInventory, PlayerInventoryMessage{
		Slots:        slots,
		EquippedSlot: 0, // Активный слот
	})
	gh.server.SendToClient(client.id, invMsg)

	return nil
}

// handleChunkRequest обрабатывает запрос на получение данных чанка
func (gh *GameHandler) handleChunkRequest(client *Client, message *Message) error {
	var req ChunkRequestMessage
	if err := json.Unmarshal(message.Data, &req); err != nil {
		return err
	}

	// Создаем сообщение чанка
	chunkMsg := WorldChunkMessage{
		ChunkX:   req.ChunkX,
		ChunkY:   req.ChunkY,
		Blocks:   make([][]uint16, 16),
		Entities: []EntityData{},
		Metadata: make(map[string]interface{}), // Инициализируем как map, а не interface{}
	}

	// Заполняем данные блоков
	for y := 0; y < 16; y++ {
		chunkMsg.Blocks[y] = make([]uint16, 16)
		for x := 0; x < 16; x++ {
			blockPos := vec.Vec2{X: req.ChunkX*16 + x, Y: req.ChunkY*16 + y}
			worldBlock := gh.gameWorld.GetBlock(blockPos)
			chunkMsg.Blocks[y][x] = uint16(worldBlock.ID)

			// Если у блока есть метаданные, добавляем их в ответ
			if worldBlock.Payload != nil && len(worldBlock.Payload) > 0 {
				// Формируем ключ для метаданных блока
				blockKey := fmt.Sprintf("block:%d:%d", x, y)
				chunkMsg.Metadata[blockKey] = worldBlock.Payload
			}
		}
	}

	// Получаем сущности в чанке
	chunkPos := vec.Vec2{X: req.ChunkX * 16, Y: req.ChunkY * 16}
	entities := gh.GetEntitiesInRange(chunkPos, 32.0) // Больший радиус для захвата сущностей на границе

	// Добавляем сущности в сообщение
	for _, ent := range entities {
		// Проверяем, находится ли сущность в запрашиваемом чанке
		entChunkX, entChunkY := ent.Position.X/16, ent.Position.Y/16
		if entChunkX == req.ChunkX && entChunkY == req.ChunkY {
			chunkMsg.Entities = append(chunkMsg.Entities, EntityData{
				ID:         ent.ID,
				Type:       ent.Type,
				Position:   ent.Position,
				Velocity:   ent.Velocity,
				Direction:  ent.Direction,
				Active:     ent.Active,
				Attributes: ent.Payload,
			})
		}
	}

	// Отправляем данные чанка
	msg, _ := NewMessage(MsgTypeWorldChunk, chunkMsg)
	return gh.server.SendToClient(client.id, msg)
}

// sendPlayerStats отправляет информацию о состоянии игрока
func (gh *GameHandler) sendPlayerStats(clientID string, entityID uint64) error {
	// Получаем сущность игрока
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return fmt.Errorf("player entity not found")
	}

	// Получаем данные о здоровье и других параметрах
	health, _ := entity.Payload["health"].(int)
	maxHealth, _ := entity.Payload["maxHealth"].(int)
	experience, _ := entity.Payload["experience"].(int)
	level, _ := entity.Payload["level"].(int)

	// Создаем сообщение со статистикой
	statsMsg := PlayerStatsMessage{
		Health:     health,
		MaxHealth:  maxHealth,
		Experience: experience,
		Level:      level,
	}

	// Добавляем эффекты статуса, если есть
	if statusEffects, ok := entity.Payload["statusEffects"].([]interface{}); ok {
		for _, effect := range statusEffects {
			if effectMap, ok := effect.(map[string]interface{}); ok {
				effectType, _ := effectMap["type"].(string)
				duration, _ := effectMap["duration"].(int)
				intensity, _ := effectMap["intensity"].(int)

				statsMsg.StatusEffects = append(statsMsg.StatusEffects, StatusEffect{
					Type:      effectType,
					Duration:  duration,
					Intensity: intensity,
				})
			}
		}
	}

	// Отправляем сообщение клиенту
	msg, _ := NewMessage(MsgTypePlayerStats, statsMsg)
	return gh.server.SendToClient(clientID, msg)
}

// sendWorldEvent отправляет событие мира всем клиентам
func (gh *GameHandler) sendWorldEvent(eventType WorldEventType, data map[string]interface{}) error {
	// Создаем сообщение о событии мира
	worldEvent := WorldEventMessage{
		EventType: eventType,
		Data:      data,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	}

	// Отправляем сообщение всем клиентам
	msg, _ := NewMessage(MsgTypeWorldEvent, worldEvent)
	return gh.server.BroadcastMessage(msg)
}

// sendWorldToClient отправляет начальные данные о мире новому клиенту
func (gh *GameHandler) sendWorldToClient(client *Client) {
	// Получаем ID сущности игрока
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[client.id]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("Не удалось найти сущность игрока для клиента: %s", client.id)
		return
	}

	// Получаем сущность игрока
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		log.Printf("Не удалось найти сущность с ID: %d", entityID)
		return
	}

	// Определяем, какие чанки нужно отправить
	playerChunkX, playerChunkY := entity.Position.X/16, entity.Position.Y/16

	// Отправляем чанки в радиусе видимости (3x3 чанка вокруг игрока)
	for y := playerChunkY - 1; y <= playerChunkY+1; y++ {
		for x := playerChunkX - 1; x <= playerChunkX+1; x++ {
			// Создаем запрос на чанк
			req := ChunkRequestMessage{
				ChunkX: int(x),
				ChunkY: int(y),
			}

			// Создаем сообщение
			reqMsg := Message{
				Type:     MsgTypeChunkRequest,
				Data:     []byte{}, // Будет заполнено при обработке
				ClientID: client.id,
			}

			// Сериализуем запрос
			reqData, err := json.Marshal(req)
			if err != nil {
				log.Printf("Ошибка сериализации запроса чанка: %v", err)
				continue
			}
			reqMsg.Data = reqData

			// Обрабатываем запрос
			gh.handleChunkRequest(client, &reqMsg)
		}
	}

	// Отправляем статистику игрока
	gh.sendPlayerStats(client.id, entityID)

	// Отправляем текущее состояние мира (время суток и т.д.)
	// В реальной реализации эти данные будут получены из WorldManager
	worldData := map[string]interface{}{
		"time_of_day": 0.5, // Полдень
		"weather":     "clear",
		"season":      "summer",
	}

	worldEventMsg, _ := NewMessage(MsgTypeWorldEvent, WorldEventMessage{
		EventType: WorldEventDayNightCycle,
		Data:      worldData,
		Timestamp: time.Now().UnixNano() / int64(time.Millisecond),
	})
	gh.server.SendToClient(client.id, worldEventMsg)
}

// sendWorldUpdates отправляет обновления игрового мира всем клиентам
func (gh *GameHandler) sendWorldUpdates() {
	// Собираем данные о всех активных сущностях
	entities := gh.GetEntitiesInRange(vec.Vec2{X: 0, Y: 0}, 1000.0) // Большой радиус

	// Создаем сообщение с обновлениями
	var entityDataList []EntityData

	for _, ent := range entities {
		if !ent.Active {
			continue
		}

		entityDataList = append(entityDataList, EntityData{
			ID:        ent.ID,
			Type:      ent.Type,
			Position:  ent.Position,
			Velocity:  ent.Velocity,
			Direction: ent.Direction,
			Active:    ent.Active,
		})
	}

	// Если есть сущности для обновления, отправляем сообщение
	if len(entityDataList) > 0 {
		updateMsg, _ := NewMessage(MsgTypeEntityUpdate, EntityUpdateMessage{
			Entities: entityDataList,
		})
		gh.server.BroadcastMessage(updateMsg)
	}
}

// SendBlockUpdate отправляет обновление блока всем клиентам в зоне видимости
// Этот метод реализует интерфейс world.NetworkManager
func (gh *GameHandler) SendBlockUpdate(blockPos vec.Vec2, block world.Block) {
	if gh.server == nil {
		return
	}

	// Отправляем обновление блока всем клиентам
	blockUpdateMsg, _ := NewMessage(MsgTypeBlockUpdate, BlockUpdateMessage{
		Blocks: []BlockData{
			{
				Position: blockPos,
				BlockID:  uint16(block.ID),
				Metadata: block.Payload,
			},
		},
	})
	gh.server.BroadcastMessage(blockUpdateMsg)
}
