package network

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/auth"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/annel0/mmo-game/internal/world/block"
	"github.com/annel0/mmo-game/internal/world/entity"
	"google.golang.org/protobuf/proto"
)

// Размер чанка в блоках
const ChunkSize = 16

// GameHandlerPB обрабатывает сообщения Protocol Buffers
type GameHandlerPB struct {
	worldManager  *world.WorldManager
	entityManager *entity.EntityManager
	userRepo      auth.UserRepository
	gameAuth      *auth.GameAuthenticator

	tcpServer *TCPServerPB
	udpServer *UDPServerPB

	playerEntities map[string]uint64   // connID -> entityID
	sessions       map[string]*Session // connID -> session

	serializer   *protocol.MessageSerializer
	lastEntityID uint64
	mu           sync.RWMutex
}

// Session stores authenticated player data for the lifetime of a TCP connection.
type Session struct {
	PlayerID uint64
	Username string
	Token    string
	IsAdmin  bool
}

// NewGameHandlerPB создает новый обработчик для Protocol Buffers
func NewGameHandlerPB(worldManager *world.WorldManager, entityManager *entity.EntityManager, userRepo auth.UserRepository) *GameHandlerPB {
	handler := &GameHandlerPB{
		worldManager:   worldManager,
		entityManager:  entityManager,
		userRepo:       userRepo,
		playerEntities: make(map[string]uint64),
		sessions:       make(map[string]*Session),

		serializer:   protocol.NewMessageSerializer(),
		lastEntityID: 0,
	}

	// Устанавливаем обработчик как сетевой менеджер для мира
	worldManager.SetNetworkManager(handler)

	return handler
}

// SetTCPServer устанавливает TCP сервер
func (gh *GameHandlerPB) SetTCPServer(server *TCPServerPB) {
	gh.tcpServer = server
}

// SetUDPServer устанавливает UDP сервер
func (gh *GameHandlerPB) SetUDPServer(server *UDPServerPB) {
	gh.udpServer = server
}

// SetGameAuthenticator устанавливает аутентификатор
func (gh *GameHandlerPB) SetGameAuthenticator(gameAuth *auth.GameAuthenticator) {
	gh.gameAuth = gameAuth

}

// HandleMessage обрабатывает входящие сообщения от клиентов
func (gh *GameHandlerPB) HandleMessage(connID string, msg *protocol.GameMessage) {
	switch msg.Type {
	case protocol.MessageType_AUTH:
		gh.handleAuth(connID, msg)
	case protocol.MessageType_BLOCK_UPDATE:
		gh.handleBlockUpdate(connID, msg)
	case protocol.MessageType_CHUNK_REQUEST:
		gh.handleChunkRequest(connID, msg)
	case protocol.MessageType_CHUNK_BATCH_REQUEST:
		gh.handleChunkBatchRequest(connID, msg)
	case protocol.MessageType_ENTITY_ACTION:
		gh.handleEntityAction(connID, msg)
	case protocol.MessageType_ENTITY_MOVE:
		gh.handleEntityMove(connID, msg)
	case protocol.MessageType_CHAT:
		gh.handleChat(connID, msg)
	default:
		log.Printf("Неизвестный тип сообщения: %d", msg.Type)
	}
}

// OnClientConnect вызывается при подключении клиента
func (gh *GameHandlerPB) OnClientConnect(connID string) {
	log.Printf("Клиент подключен: %s", connID)
}

// OnClientDisconnect вызывается при отключении клиента
func (gh *GameHandlerPB) OnClientDisconnect(connID string) {
	gh.mu.Lock()
	defer gh.mu.Unlock()

	// Находим и удаляем сущность игрока
	if entityID, exists := gh.playerEntities[connID]; exists {
		// Удаляем сущность из мира
		gh.DespawnEntity(entityID)

		// Удаляем привязку к игроку
		delete(gh.playerEntities, connID)

		// Оповещаем других игроков
		despawnMsg := &protocol.EntityDespawnMessage{
			EntityId: entityID,
			Reason:   "disconnected",
		}
		gh.broadcastMessage(protocol.MessageType_ENTITY_DESPAWN, despawnMsg)
	}

	log.Printf("Клиент отключен: %s", connID)
}

// Tick обновляет состояние игрового мира
func (gh *GameHandlerPB) Tick(dt float64) {
	// Обновляем все сущности
	gh.entityManager.UpdateEntities(dt, gh)

	// Отправляем обновления игрокам
	gh.sendWorldUpdates()
}

// GetBlock реализует интерфейс EntityAPI
func (gh *GameHandlerPB) GetBlock(pos vec.Vec2) block.BlockID {
	// Получаем блок из мира
	worldBlock := gh.worldManager.GetBlock(pos)
	return worldBlock.ID
}

// SetBlock реализует интерфейс EntityAPI
func (gh *GameHandlerPB) SetBlock(pos vec.Vec2, id block.BlockID) {
	// Создаем блок для мира
	worldBlock := world.NewBlock(id)

	// Устанавливаем блок в мире
	gh.worldManager.SetBlock(pos, worldBlock)
}

// GetBlockMetadata реализует интерфейс EntityAPI
func (gh *GameHandlerPB) GetBlockMetadata(pos vec.Vec2, key string) interface{} {
	// Получаем блок из мира
	worldBlock := gh.worldManager.GetBlock(pos)

	// Получаем метаданные
	if worldBlock.Payload != nil {
		if value, exists := worldBlock.Payload[key]; exists {
			return value
		}
	}

	return nil
}

// SetBlockMetadata реализует интерфейс EntityAPI
func (gh *GameHandlerPB) SetBlockMetadata(pos vec.Vec2, key string, value interface{}) {
	// Получаем текущий блок
	worldBlock := gh.worldManager.GetBlock(pos)

	// Обновляем метаданные
	if worldBlock.Payload == nil {
		worldBlock.Payload = make(map[string]interface{})
	}

	worldBlock.Payload[key] = value

	// Устанавливаем обновленный блок
	gh.worldManager.SetBlock(pos, worldBlock)
}

// GetEntitiesInRange реализует интерфейс EntityAPI
func (gh *GameHandlerPB) GetEntitiesInRange(center vec.Vec2, radius float64) []*entity.Entity {
	return gh.entityManager.GetEntitiesInRange(center, radius)
}

// GetBehavior реализует интерфейс EntityAPI
func (gh *GameHandlerPB) GetBehavior(entityType entity.EntityType) (entity.EntityBehavior, bool) {
	// Получаем поведение из менеджера сущностей
	return gh.entityManager.GetBehavior(entityType)
}

// MoveEntity реализует интерфейс EntityAPI
func (gh *GameHandlerPB) MoveEntity(entity *entity.Entity, direction entity.MovementDirection, dt float64) bool {
	// Получаем поведение для данного типа сущности
	behavior, exists := gh.GetBehavior(entity.Type)
	if !exists {
		log.Printf("Нет поведения для сущности типа %d", entity.Type)
		return false
	}

	// Получаем скорость движения сущности
	moveSpeed := behavior.GetMoveSpeed()

	// Вычисляем вектор направления
	moveDir := vec.Vec2Float{X: 0, Y: 0}

	if direction.Up {
		moveDir.Y -= 1
	}
	if direction.Down {
		moveDir.Y += 1
	}
	if direction.Left {
		moveDir.X -= 1
	}
	if direction.Right {
		moveDir.X += 1
	}

	// Обновляем направление взгляда сущности, если есть движение
	if moveDir.X != 0 || moveDir.Y != 0 {
		entity.Direction = calculateDirection(moveDir)
	} else {
		return false // Нет движения
	}

	// Нормализуем вектор для диагонального движения
	if moveDir.X != 0 && moveDir.Y != 0 {
		length := math.Sqrt(moveDir.X*moveDir.X + moveDir.Y*moveDir.Y)
		moveDir.X /= length
		moveDir.Y /= length
	}

	// Вычисляем новую позицию
	newPos := entity.PrecisePos.Add(moveDir.Mul(moveSpeed * dt))

	// Проверяем столкновения с блоками с учётом слоёв и проходимости
	blockX := int(math.Floor(newPos.X))
	blockY := int(math.Floor(newPos.Y))

	for x := blockX - 1; x <= blockX+1; x++ {
		for y := blockY - 1; y <= blockY+1; y++ {
			pos := vec.Vec2{X: x, Y: y}

			// Если позиция непроходима, обрабатываем коллизию
			if !gh.isPositionWalkable(pos) {
				if gh.checkEntityBlockCollision(entity, newPos, pos) {
					behavior.OnCollision(gh, entity, gh.worldManager.GetBlockLayer(pos, world.LayerActive).ID, newPos)
					return false
				}
			}
		}
	}

	// Проверяем коллизии с другими сущностями
	nearbyEntities := gh.GetEntitiesInRange(entity.Position, 2.0)
	for _, other := range nearbyEntities {
		if other.ID == entity.ID {
			continue // Пропускаем саму сущность
		}

		// Проверяем коллизию между сущностями
		if gh.checkEntityEntityCollision(entity, newPos, other) {
			// Вызываем обработчик коллизий
			behavior.OnCollision(gh, entity, other, newPos)
			return false
		}
	}

	// Если коллизий нет, обновляем позицию
	entity.PrecisePos = newPos
	entity.Position = newPos.ToVec2()

	// Оповещаем клиентов о перемещении
	gh.sendEntityMoveUpdate(entity)

	return true
}

// calculateDirection определяет направление взгляда по вектору движения
func calculateDirection(moveDir vec.Vec2Float) int {
	// Для 4 направлений
	if math.Abs(moveDir.X) > math.Abs(moveDir.Y) {
		if moveDir.X > 0 {
			return 1 // Восток (вправо)
		} else {
			return 3 // Запад (влево)
		}
	} else {
		if moveDir.Y > 0 {
			return 0 // Юг (вниз)
		} else {
			return 2 // Север (вверх)
		}
	}
}

// checkEntityBlockCollision проверяет коллизию сущности с блоком
func (gh *GameHandlerPB) checkEntityBlockCollision(entity *entity.Entity, newPos vec.Vec2Float, blockPos vec.Vec2) bool {
	// Границы сущности
	entityLeft := newPos.X - entity.Size.X/2
	entityRight := newPos.X + entity.Size.X/2
	entityTop := newPos.Y - entity.Size.Y/2
	entityBottom := newPos.Y + entity.Size.Y/2

	// Границы блока
	blockLeft := float64(blockPos.X)
	blockRight := float64(blockPos.X) + 1.0
	blockTop := float64(blockPos.Y)
	blockBottom := float64(blockPos.Y) + 1.0

	// Проверка пересечения
	return entityRight > blockLeft &&
		entityLeft < blockRight &&
		entityBottom > blockTop &&
		entityTop < blockBottom
}

// checkEntityEntityCollision проверяет коллизию между двумя сущностями
func (gh *GameHandlerPB) checkEntityEntityCollision(entity *entity.Entity, newPos vec.Vec2Float, other *entity.Entity) bool {
	// Расстояние между центрами сущностей
	distance := newPos.DistanceTo(other.PrecisePos)

	// Сумма радиусов (полуразмеров) сущностей
	radiusSum := (entity.Size.X + other.Size.X) / 2

	// Если расстояние меньше суммы радиусов, есть коллизия
	return distance < radiusSum
}

// sendEntityMoveUpdate отправляет обновление о перемещении сущности
func (gh *GameHandlerPB) sendEntityMoveUpdate(entity *entity.Entity) {
	// Создаем данные сущности для сообщения
	entityData := &protocol.EntityData{
		Id:        entity.ID,
		Type:      protocol.EntityType(entity.Type),
		Position:  &protocol.Vec2{X: int32(entity.Position.X), Y: int32(entity.Position.Y)},
		Direction: int32(entity.Direction),
		Active:    entity.Active,
	}

	// Создаем сообщение о перемещении
	moveMsg := &protocol.EntityMoveMessage{
		Entities: []*protocol.EntityData{entityData},
	}

	// Отправляем всем клиентам, кроме владельца сущности
	gh.mu.RLock()
	playerConnID := ""
	for connID, entID := range gh.playerEntities {
		if entID == entity.ID {
			playerConnID = connID
			break
		}
	}
	gh.mu.RUnlock()

	// Отправляем всем, кроме владельца
	for connID := range gh.tcpServer.connections {
		if connID != playerConnID {
			gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_MOVE, moveMsg)
		}
	}
}

// sendEntityPositionCorrection отправляет владельцу сущности корректирующее сообщение,
// содержащее её фактическую позицию на сервере. Используется, когда перемещение
// клиента было отклонено (коллизия, непроходимая область и т.п.), чтобы клиент
// «откатился» к авторитетной позиции сервера.
func (gh *GameHandlerPB) sendEntityPositionCorrection(connID string, entity *entity.Entity) {
	if connID == "" || gh.tcpServer == nil {
		return
	}

	// Формируем данные сущности
	entityData := &protocol.EntityData{
		Id:        entity.ID,
		Type:      protocol.EntityType(entity.Type),
		Position:  &protocol.Vec2{X: int32(entity.Position.X), Y: int32(entity.Position.Y)},
		Active:    entity.Active,
		Direction: int32(entity.Direction),
		Velocity:  &protocol.Vec2Float{X: 0, Y: 0}, // после отката скорость обнуляем
	}

	// Создаём и отправляем сообщение
	moveMsg := &protocol.EntityMoveMessage{Entities: []*protocol.EntityData{entityData}}
	gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_MOVE, moveMsg)
}

// IsSessionValid проверяет, что для данного connID существует активная сессия.
// Подробная валидация JWT может быть добавлена позднее; для исключения ложных
// отрицаний при повторных авторизованных запросах достаточно факта наличия
// сессии.
func (gh *GameHandlerPB) IsSessionValid(connID string) bool {
	gh.mu.RLock()
	_, ok := gh.sessions[connID]
	gh.mu.RUnlock()
	return ok
}

// handleAuth обрабатывает аутентификацию с использованием GameAuthenticator
func (gh *GameHandlerPB) handleAuth(connID string, msg *protocol.GameMessage) {
	// Проверяем, что GameAuthenticator инициализирован
	if gh.gameAuth == nil {
		log.Printf("❌ GameAuthenticator не инициализирован")
		resp := &protocol.AuthResponse{Success: false, Message: "Server authentication error"}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, resp)
		return
	}

	authMsg := &protocol.AuthRequest{}
	if err := gh.serializer.DeserializePayload(msg, authMsg); err != nil {
		log.Printf("❌ Ошибка десериализации Auth: %v", err)
		resp := &protocol.AuthResponse{Success: false, Message: "Invalid request format"}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, resp)
		return
	}

	// Если уже имеется валидная сессия – пропускаем повторную авторизацию
	if gh.IsSessionValid(connID) {
		log.Printf("⚠️ Повторная авторизация от %s игнорируется", connID)
		return
	}

	// === НОВАЯ ЛОГИКА С GAME AUTHENTICATOR ===
	// Выполняем аутентификацию через GameAuthenticator
	password := ""
	if authMsg.Password != nil {
		password = *authMsg.Password
	}

	authResult, err := gh.gameAuth.AuthenticateUser(authMsg.Username, password)
	if err != nil {
		log.Printf("❌ Ошибка при аутентификации: %v", err)
		resp := &protocol.AuthResponse{Success: false, Message: "Authentication service error"}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, resp)
		return
	}

	// Если аутентификация не удалась
	if !authResult.Success {
		log.Printf("❌ Аутентификация не удалась для %s: %s", authMsg.Username, authResult.Message)
		authResp := &protocol.AuthResponse{
			Success: false,
			Message: authResult.Message,
		}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, authResp)
		return
	}

	// Аутентификация успешна
	username := authMsg.Username

	// Определяем роль пользователя
	isAdmin := false
	serverCapabilities := make([]string, 0)
	if len(authResult.Roles) > 0 {
		for _, role := range authResult.Roles {
			serverCapabilities = append(serverCapabilities, role)
			if role == "admin" {
				isAdmin = true
			}
		}
	}

	// Создаем игровую сущность
	var entityID uint64
	gh.mu.Lock()
	if existingEntityID, exists := gh.playerEntities[connID]; !exists {
		// НЕ используем gh.generateEntityID() потому что мы уже в блокировке!
		gh.lastEntityID++
		entityID = gh.lastEntityID
		gh.playerEntities[connID] = entityID

		// Создаем AuthResponse с JWT токеном
		authResp := &protocol.AuthResponse{
			Success:            true,
			Message:            authResult.Message,
			PlayerId:           entityID,
			JwtToken:           &authResult.Token,
			ServerCapabilities: serverCapabilities,
			WorldName:          "main_world",
			ServerInfo: &protocol.ServerInfo{
				Version:     "1.0.0",
				Environment: "development",
			},
		}

		gh.sessions[connID] = &Session{
			PlayerID: entityID,
			Username: username,
			Token:    authResult.Token,
			IsAdmin:  isAdmin,
		}

		log.Printf("✅ Создана игровая сущность %d для пользователя %s", entityID, username)

		// Создаем сущность игрока в мире
		spawnPos := vec.Vec2{X: 0, Y: 0}
		gh.spawnEntityWithID(entity.EntityTypePlayer, spawnPos, entityID)

		// Связываем TCP-соединение с playerID для дальнейших проверок
		if gh.tcpServer != nil {
			gh.tcpServer.mu.Lock()
			if conn, ok := gh.tcpServer.connections[connID]; ok {
				conn.playerID = entityID
			}
			gh.tcpServer.mu.Unlock()
		}

		// Отправляем успешный ответ
		log.Printf("✅ Аутентификация успешна для %s (ID: %d)", username, entityID)
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, authResp)

	} else {
		entityID = existingEntityID
		log.Printf("⚠️ Игровая сущность уже существует для %s", connID)

		// Отправляем ответ для существующей сессии
		authResp := &protocol.AuthResponse{
			Success:            true,
			Message:            "Already authenticated",
			PlayerId:           entityID,
			JwtToken:           &authResult.Token,
			ServerCapabilities: serverCapabilities,
			WorldName:          "main_world",
		}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, authResp)
	}
	gh.mu.Unlock()

	// Отправляем данные мира
	if entityID, exists := gh.playerEntities[connID]; exists {
		gh.sendWorldDataToPlayer(connID, entityID)
	}
}

// handleBlockUpdate обрабатывает обновление блока
func (gh *GameHandlerPB) handleBlockUpdate(connID string, msg *protocol.GameMessage) {
	blockUpdate := &protocol.BlockUpdateRequest{}
	if err := gh.serializer.DeserializePayload(msg, blockUpdate); err != nil {
		log.Printf("Ошибка десериализации BlockUpdate: %v", err)
		return
	}

	// === Новый универсальный обработчик ===
	if blockUpdate.Position == nil {
		log.Printf("Недействительное обновление блока: позиция nil")
		return
	}

	pos := vec.Vec2{X: int(blockUpdate.Position.X), Y: int(blockUpdate.Position.Y)}

	// Получаем текущий блок
	oldBlock := gh.worldManager.GetBlock(pos)
	currentBehavior, _ := block.Get(oldBlock.ID)

	// actionPayload из запроса
	var actionPayload map[string]interface{}
	if blockUpdate.Metadata != nil && blockUpdate.Metadata.JsonData != "" {
		actionPayload, _ = protocol.JsonToMap(blockUpdate.Metadata.JsonData)
	}

	action := blockUpdate.Action
	if action == "" {
		action = "place"
	}

	var newID block.BlockID
	var newPayload map[string]interface{}
	var result block.InteractionResult

	switch action {
	case "place":
		newID = block.BlockID(blockUpdate.BlockId)
		newBehavior, _ := block.Get(newID)
		newPayload = make(map[string]interface{})
		if newBehavior != nil {
			newPayload = newBehavior.CreateMetadata()
		}
		result = block.InteractionResult{Success: true}

	case "mine", "break":
		// OnBreak будет вызван автоматически в WorldManager при замене блока
		newID = block.AirBlockID
		newPayload = nil
		result = block.InteractionResult{Success: true}

	default: // use / custom
		if currentBehavior != nil {
			newID, newPayload, result = currentBehavior.HandleInteraction(action, oldBlock.Payload, actionPayload)
		} else {
			result = block.InteractionResult{Success: false, Message: "No behavior"}
			newID = oldBlock.ID
			newPayload = oldBlock.Payload
		}
	}

	// Применяем изменения
	blockObj := world.NewBlock(newID)
	blockObj.Payload = newPayload
	gh.worldManager.SetBlock(pos, blockObj)

	// Формируем ответ
	metaStr, _ := protocol.MapToJsonMetadata(newPayload)
	respMeta := &protocol.JsonMetadata{JsonData: metaStr}
	response := &protocol.BlockUpdateResponse{
		Success:  result.Success,
		Message:  result.Message,
		BlockId:  uint32(newID),
		Position: blockUpdate.Position,
		Metadata: respMeta,
		Effects:  result.Effects,
	}

	gh.sendTCPMessage(connID, protocol.MessageType_BLOCK_UPDATE_RESPONSE, response)
}

// handleChunkBatchRequest обрабатывает запрос пакета чанков
func (gh *GameHandlerPB) handleChunkBatchRequest(connID string, msg *protocol.GameMessage) {
	batchReq := &protocol.ChunkBatchRequest{}
	if err := gh.serializer.DeserializePayload(msg, batchReq); err != nil {
		log.Printf("Ошибка десериализации ChunkBatchRequest: %v", err)
		return
	}

	// Обрабатываем каждый чанк в пакете
	for _, chunk := range batchReq.Chunks {
		gh.sendChunkToClient(connID, int(chunk.X), int(chunk.Y))
	}
}

// handleChunkRequest обрабатывает запрос чанка
func (gh *GameHandlerPB) handleChunkRequest(connID string, msg *protocol.GameMessage) {
	chunkRequest := &protocol.ChunkRequest{}
	if err := gh.serializer.DeserializePayload(msg, chunkRequest); err != nil {
		log.Printf("Ошибка десериализации ChunkRequest: %v", err)
		return
	}

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	_, exists := gh.playerEntities[connID]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("Неавторизованный клиент запрашивает чанк: %s", connID)
		return
	}

	// Отправляем чанк клиенту
	gh.sendChunkToClient(connID, int(chunkRequest.ChunkX), int(chunkRequest.ChunkY))
}

// sendChunkToClient отправляет чанк клиенту
func (gh *GameHandlerPB) sendChunkToClient(connID string, chunkX, chunkY int) {
	// Искусственная задержка 40–60 мс для сглаживания потока
	jitter := 40 + rand.Intn(21) // 40..60
	time.Sleep(time.Duration(jitter) * time.Millisecond)

	// Получаем чанк из мира
	chunkPos := vec.Vec2{X: chunkX, Y: chunkY}
	chunk := gh.worldManager.GetChunk(chunkPos)

	// Сериализуем чанк в Protocol Buffers (многослойная схема)
	chunkData := &protocol.ChunkData{
		ChunkX: int32(chunkX),
		ChunkY: int32(chunkY),
	}

	crc := crc32.NewIEEE()
	nonEmpty := 0

	// Слои: FLOOR и ACTIVE
	layers := []*protocol.ChunkLayer{}
	for _, layerID := range []world.BlockLayer{world.LayerFloor, world.LayerActive} {
		layerMsg := &protocol.ChunkLayer{Layer: uint32(layerID), Rows: make([]*protocol.BlockRow, 16)}
		for blockY := 0; blockY < 16; blockY++ {
			row := make([]uint32, 16)
			for blockX := 0; blockX < 16; blockX++ {
				bID := uint32(chunk.GetBlockLayer(layerID, vec.Vec2{X: blockX, Y: blockY}))
				row[blockX] = bID
				_ = binary.Write(crc, binary.LittleEndian, bID)
				if bID != 0 {
					nonEmpty++
				}
			}
			layerMsg.Rows[blockY] = &protocol.BlockRow{BlockIds: row}
		}
		layers = append(layers, layerMsg)
	}
	chunkData.Layers = layers

	// Создаём контейнер для метаданных блоков
	blockMetadata := &protocol.ChunkBlockMetadata{BlockMetadata: make(map[string]*protocol.JsonMetadata)}

	// Заполняем blockMetadata из данных чанка (только слой ACTIVE)
	for coord, metadata := range chunk.Metadata3D {
		if coord.Layer == world.LayerActive && len(metadata) > 0 {
			jsonStr, err := protocol.MapToJsonMetadata(metadata)
			if err == nil {
				key := fmt.Sprintf("%d:%d", coord.Pos.X, coord.Pos.Y)
				blockMetadata.BlockMetadata[key] = &protocol.JsonMetadata{JsonData: jsonStr}
			}
		}
	}

	// Подготовка финальной карты метаданных
	metaMap := map[string]interface{}{
		"checksum": crc.Sum32(),
		"nonEmpty": nonEmpty,
	}
	if len(blockMetadata.BlockMetadata) > 0 {
		metaMap["blockMetadata"] = blockMetadata
	}

	metadataJson, errMeta := protocol.MapToJsonMetadata(metaMap)
	if errMeta == nil {
		chunkData.Metadata = &protocol.JsonMetadata{JsonData: metadataJson}
	}

	// Отправляем чанк
	gh.sendTCPMessage(connID, protocol.MessageType_CHUNK_DATA, chunkData)
}

// handleEntityAction обрабатывает действия сущности
func (gh *GameHandlerPB) handleEntityAction(connID string, msg *protocol.GameMessage) {
	action := &protocol.EntityActionRequest{}
	if err := gh.serializer.DeserializePayload(msg, action); err != nil {
		log.Printf("Ошибка десериализации EntityAction: %v", err)
		return
	}

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[connID]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("Неавторизованный клиент выполняет действие: %s", connID)
		return
	}

	// Проверяем существование сущности
	_, exists = gh.entityManager.GetEntity(entityID)
	if !exists {
		log.Printf("Сущность %d не найдена", entityID)
		return
	}

	// Обрабатываем действие
	// TODO: Реализовать конкретную логику действий
	success := true
	message := "Действие выполнено"

	// Отправляем ответ
	response := &protocol.EntityActionResponse{
		Success: success,
		Message: message,
	}

	gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_ACTION_RESPONSE, response)

	// Если действие успешно, оповещаем других игроков
	if success {
		gh.broadcastMessage(protocol.MessageType_ENTITY_ACTION, action)
	}
}

// handleEntityMove обрабатывает движение сущности
func (gh *GameHandlerPB) handleEntityMove(connID string, msg *protocol.GameMessage) {
	// Десериализуем сообщение перемещения
	moveMsg := &protocol.EntityMoveMessage{}
	if err := gh.serializer.DeserializePayload(msg, moveMsg); err != nil {
		log.Printf("Ошибка десериализации EntityMove: %v", err)
		return
	}

	// Проверяем сессию
	gh.mu.RLock()
	ownerID, ok := gh.playerEntities[connID]
	gh.mu.RUnlock()
	if !ok {
		log.Printf("Неавторизованный клиент перемещает сущности: %s", connID)
		return
	}

	// Для каждой сущности в сообщении
	for _, ed := range moveMsg.Entities {
		// Пока разрешаем перемещать только собственную сущность
		if ed.Id != ownerID {
			log.Printf("Игрок %d пытается переместить чужую сущность %d", ownerID, ed.Id)
			continue
		}

		ent, exists := gh.entityManager.GetEntity(ed.Id)
		if !exists {
			log.Printf("Сущность %d не найдена", ed.Id)
			continue
		}

		// Целевая позиция
		targetPos := vec.Vec2{
			X: int(ed.Position.X),
			Y: int(ed.Position.Y),
		}

		// Проверяем коллизии с использованием многослойной логики
		if !gh.isPositionWalkable(targetPos) {
			log.Printf("Сущность %d попытка переместиться в непроходимую позицию (%d,%d)", ed.Id, targetPos.X, targetPos.Y)
			// Отправляем корректирующее сообщение владельцу, чтобы клиент откатил позицию
			gh.sendEntityPositionCorrection(connID, ent)
			continue
		}

		// Обновляем позицию
		oldPos := ent.PrecisePos
		ent.PrecisePos = vec.Vec2Float{X: float64(targetPos.X), Y: float64(targetPos.Y)}
		ent.Position = targetPos

		// Сообщаем worldManager о смене BigChunk
		gh.worldManager.ProcessEntityMovement(ent.ID, vec.Vec2{X: int(oldPos.X), Y: int(oldPos.Y)}, targetPos)

		// Рассылаем обновление другим игрокам
		gh.sendEntityMoveUpdate(ent)
	}
}

// handleChat обрабатывает сообщения чата
func (gh *GameHandlerPB) handleChat(connID string, msg *protocol.GameMessage) {
	// Упрощенная обработка для примера
	log.Printf("Получено сообщение чата от %s", connID)

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[connID]
	session, sessionExists := gh.sessions[connID]
	gh.mu.RUnlock()

	if !exists || !sessionExists {
		log.Printf("Неавторизованный клиент отправляет сообщение: %s", connID)
		return
	}

	playerName := session.Username

	// Отправляем простое сообщение всем
	gh.broadcastMessage(protocol.MessageType_CHAT_BROADCAST, &protocol.ChatBroadcast{
		Type:       protocol.ChatType_CHAT_GLOBAL,
		Message:    "Чат временно отключен",
		SenderId:   entityID,
		SenderName: playerName,
		Timestamp:  time.Now().UnixNano(),
	})
}

// sendWorldDataToPlayer отправляет начальные данные о мире игроку
func (gh *GameHandlerPB) sendWorldDataToPlayer(connID string, playerID uint64) {
	// Отправляем первоначальные чанки
	gh.sendInitialChunks(connID, playerID)

	// Отправляем сведения о текущем состоянии мира
	worldData := map[string]interface{}{
		"time_of_day": 0.5,
		"weather":     "clear",
		"season":      "summer",
		"game_mode":   "survival",
		"world_id":    1234,
		"world_name":  "default",
	}

	// Сериализуем метаданные в JSON
	jsonData, err := json.Marshal(worldData)
	if err != nil {
		log.Printf("Ошибка сериализации данных мира: %v", err)
		return
	}

	// Создаем сообщение о состоянии мира с метаданными
	worldMetadata := &protocol.JsonMetadata{
		JsonData: string(jsonData),
	}

	// Отправляем метаданные мира через сообщение с метаданными
	gh.sendTCPMessage(connID, protocol.MessageType_CHUNK_DATA, worldMetadata)

	// Отправляем данные о других игроках в зоне видимости
	// Получаем сущность игрока
	playerEntity, exists := gh.entityManager.GetEntity(playerID)
	if !exists {
		return
	}

	// Получаем сущности поблизости
	nearbyEntities := gh.GetEntitiesInRange(playerEntity.Position, 100.0)

	// Формируем данные для отправки
	var spawnedEntities []*protocol.EntityData

	for _, entity := range nearbyEntities {
		if entity.ID == playerID {
			continue // Пропускаем собственную сущность
		}

		entityData := &protocol.EntityData{
			Id:        entity.ID,
			Type:      protocol.EntityType(entity.Type),
			Position:  &protocol.Vec2{X: int32(entity.Position.X), Y: int32(entity.Position.Y)},
			Direction: int32(entity.Direction),
			Active:    entity.Active,
		}

		// Если это сущность игрока, добавляем имя
		if int(entity.Type) == 0 { // EntityTypePlayer = 0 in entity package
			gh.mu.RLock()
			// Ищем имя игрока по entityID в sessions
			var username string
			for _, session := range gh.sessions {
				if session.PlayerID == entity.ID {
					username = session.Username
					break
				}
			}
			gh.mu.RUnlock()

			if username != "" {
				// Добавляем имя в атрибуты сущности
				entityData.Attributes = &protocol.JsonMetadata{
					JsonData: `{"username": "` + username + `"}`,
				}
			}
		}

		spawnedEntities = append(spawnedEntities, entityData)
	}

	// Отправляем сообщение о сущностях в зоне видимости
	if len(spawnedEntities) > 0 {
		spawnMsg := &protocol.EntityMoveMessage{
			Entities: spawnedEntities,
		}

		gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_MOVE, spawnMsg)
	}
}

// sendInitialChunks отправляет начальные чанки игроку
func (gh *GameHandlerPB) sendInitialChunks(connID string, playerID uint64) {
	// Получаем сущность игрока
	playerEntity, exists := gh.entityManager.GetEntity(playerID)
	if !exists {
		return
	}

	// Получаем координаты чанка игрока
	playerChunkCoords := playerEntity.Position.ToChunkCoords()

	// Отправляем чанки в радиусе видимости (5 чанков)
	chunkRadius := 5

	for x := playerChunkCoords.X - chunkRadius; x <= playerChunkCoords.X+chunkRadius; x++ {
		for y := playerChunkCoords.Y - chunkRadius; y <= playerChunkCoords.Y+chunkRadius; y++ {
			chunkPos := vec.Vec2{X: x, Y: y}

			// Получаем данные чанка из мира
			chunk := gh.worldManager.GetChunk(chunkPos)
			if chunk == nil {
				continue
			}

			// Преобразуем данные чанка в протокольный формат
			chunkData := &protocol.ChunkData{
				ChunkX: int32(x),
				ChunkY: int32(y),
			}

			// Слои: FLOOR и ACTIVE
			layers := []*protocol.ChunkLayer{}
			for _, layerID := range []world.BlockLayer{world.LayerFloor, world.LayerActive} {
				layerMsg := &protocol.ChunkLayer{Layer: uint32(layerID), Rows: make([]*protocol.BlockRow, 16)}
				for blockY := 0; blockY < 16; blockY++ {
					row := make([]uint32, 16)
					for blockX := 0; blockX < 16; blockX++ {
						bID := uint32(chunk.GetBlockLayer(layerID, vec.Vec2{X: blockX, Y: blockY}))
						row[blockX] = bID
					}
					layerMsg.Rows[blockY] = &protocol.BlockRow{BlockIds: row}
				}
				layers = append(layers, layerMsg)
			}
			chunkData.Layers = layers

			// Отправляем данные чанка
			gh.sendTCPMessage(connID, protocol.MessageType_CHUNK_DATA, chunkData)

			// Добавляем небольшую задержку, чтобы не перегружать клиента
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// sendWorldUpdates отправляет периодические обновления игрового мира всем клиентам
func (gh *GameHandlerPB) sendWorldUpdates() {
	// Группируем сущности для отправки клиентам
	// Каждый клиент должен получать только сущности в его зоне видимости
	gh.mu.RLock()
	playerConnections := make(map[string]uint64)
	for connID, playerID := range gh.playerEntities {
		playerConnections[connID] = playerID
	}
	gh.mu.RUnlock()

	// Для каждого клиента формируем и отправляем список видимых сущностей
	for connID, playerID := range playerConnections {
		// Получаем собственную сущность игрока
		playerEntity, exists := gh.entityManager.GetEntity(playerID)
		if !exists {
			continue
		}

		// Получаем все сущности в радиусе видимости от игрока
		// (используем радиус 100 блоков как зону видимости)
		visibleEntities := gh.GetEntitiesInRange(playerEntity.Position, 100.0)

		// Формируем список данных сущностей для отправки
		entityDataList := make([]*protocol.EntityData, 0, len(visibleEntities))

		for _, entity := range visibleEntities {
			// Не отправляем информацию о собственной сущности игрока
			if entity.ID == playerID {
				continue
			}

			// Создаем данные сущности
			entityData := &protocol.EntityData{
				Id:        entity.ID,
				Type:      protocol.EntityType(entity.Type),
				Position:  &protocol.Vec2{X: int32(entity.Position.X), Y: int32(entity.Position.Y)},
				Direction: int32(entity.Direction),
				Active:    entity.Active,
			}

			// Если есть скорость, добавляем её
			if entity.Velocity.X != 0 || entity.Velocity.Y != 0 {
				entityData.Velocity = &protocol.Vec2Float{
					X: float32(entity.Velocity.X),
					Y: float32(entity.Velocity.Y),
				}
			}

			entityDataList = append(entityDataList, entityData)
		}

		// Если есть сущности для отправки, отправляем сообщение клиенту
		if len(entityDataList) > 0 {
			updateMsg := &protocol.EntityMoveMessage{
				Entities: entityDataList,
			}

			gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_MOVE, updateMsg)
		}
	}
}

// SpawnEntity реализует интерфейс EntityAPI - изменяем сигнатуру
func (gh *GameHandlerPB) SpawnEntity(entityType entity.EntityType, position vec.Vec2) uint64 {
	// Генерируем ID для новой сущности
	entityID := gh.generateEntityID()

	// Вызываем внутренний метод с дополнительным параметром
	return gh.spawnEntityWithID(entityType, position, entityID)
}

// spawnEntityWithID - внутренний метод для создания сущности с указанным ID
func (gh *GameHandlerPB) spawnEntityWithID(entityType entity.EntityType, position vec.Vec2, entityID uint64) uint64 {
	log.Printf("Создание сущности типа %d с ID %d в позиции (%d, %d)",
		entityType, entityID, position.X, position.Y)

	// === 1. Реально создаём сущность и регистрируем в EntityManager ===
	newEntity := entity.NewEntity(entityID, entityType, position)
	gh.entityManager.AddEntity(newEntity)

	// === 2. При необходимости уведомляем поведение сущности ===
	if behavior, ok := gh.entityManager.GetBehavior(entityType); ok {
		behavior.OnSpawn(gh, newEntity)
	}

	// === 3. Шлём сообщение всем клиентам ===
	entityData := &protocol.EntityData{
		Id:       entityID,
		Type:     protocol.EntityType(entityType),
		Position: &protocol.Vec2{X: int32(position.X), Y: int32(position.Y)},
		Active:   true,
	}

	entitySpawn := &protocol.EntitySpawnMessage{
		Entity: entityData,
	}

	gh.broadcastMessage(protocol.MessageType_ENTITY_SPAWN, entitySpawn)

	return entityID
}

// DespawnEntity удаляет сущность из мира
func (gh *GameHandlerPB) DespawnEntity(entityID uint64) {
	// Временная заглушка до полной реализации
	log.Printf("Удаление сущности с ID %d", entityID)

	// Оповещаем всех игроков
	despawnMsg := &protocol.EntityDespawnMessage{
		EntityId: entityID,
		Reason:   "deleted",
	}
	gh.broadcastMessage(protocol.MessageType_ENTITY_DESPAWN, despawnMsg)
}

// SendBlockUpdate отправляет обновление блока всем клиентам
func (gh *GameHandlerPB) SendBlockUpdate(blockPos vec.Vec2, block world.Block) {
	// Создаем сообщение об обновлении блока
	blockData := &protocol.BlockData{
		Position: &protocol.Vec2{
			X: int32(blockPos.X),
			Y: int32(blockPos.Y),
		},
		BlockId: uint32(block.ID),
	}

	// Добавляем метаданные, если они есть
	if block.Payload != nil && len(block.Payload) > 0 {
		jsonStr, err := protocol.MapToJsonMetadata(block.Payload)
		if err == nil {
			blockData.Metadata = &protocol.JsonMetadata{
				JsonData: jsonStr,
			}
		}
	}

	blockUpdate := &protocol.BlockUpdateMessage{
		Blocks: []*protocol.BlockData{blockData},
	}

	// Отправляем всем клиентам
	gh.broadcastMessage(protocol.MessageType_BLOCK_UPDATE, blockUpdate)
}

// broadcastMessage отправляет сообщение всем подключенным клиентам
func (gh *GameHandlerPB) broadcastMessage(msgType protocol.MessageType, payload proto.Message) {
	if gh.tcpServer != nil {
		gh.tcpServer.broadcastMessage(msgType, payload)
	}
}

// sendTCPMessage отправляет сообщение конкретному клиенту через TCP
func (gh *GameHandlerPB) sendTCPMessage(connID string, msgType protocol.MessageType, payload proto.Message) {
	if gh.tcpServer != nil {
		gh.tcpServer.sendToClient(connID, msgType, payload)
	}
}

// generateEntityID генерирует уникальный ID для сущности
func (gh *GameHandlerPB) generateEntityID() uint64 {
	gh.mu.Lock()
	defer gh.mu.Unlock()

	gh.lastEntityID++
	return gh.lastEntityID
}

// SendMessage реализует интерфейс EntityAPI
func (gh *GameHandlerPB) SendMessage(entityID uint64, messageType string, data interface{}) {
	// Находим клиента, связанного с этой сущностью
	var connID string

	gh.mu.RLock()
	for cid, eid := range gh.playerEntities {
		if eid == entityID {
			connID = cid
			break
		}
	}
	gh.mu.RUnlock()

	if connID == "" {
		return // Сущность не связана с клиентом
	}

	// Отправляем сообщение клиенту
	log.Printf("Отправка сообщения типа %s игроку %s", messageType, connID)
}

// isPositionWalkable применяет логику слоёв: сначала ACTIVE, затем FLOOR.
func (gh *GameHandlerPB) isPositionWalkable(pos vec.Vec2) bool {
	// Проверяем ACTIVE слой
	activeBlock := gh.worldManager.GetBlockLayer(pos, world.LayerActive)

	passable := func(id block.BlockID) bool {
		if behavior, exists := block.Get(id); exists {
			if p, ok := behavior.(interface{ IsPassable() bool }); ok {
				return p.IsPassable()
			}
		}
		return id == block.AirBlockID
	}

	if !passable(activeBlock.ID) {
		return false
	}

	// Если ACTIVE – воздух, проверяем FLOOR как «опору»
	if activeBlock.ID == block.AirBlockID {
		floorBlock := gh.worldManager.GetBlockLayer(pos, world.LayerFloor)
		if floorBlock.ID == block.AirBlockID {
			return false // пропасть
		}
	}

	return true
}
