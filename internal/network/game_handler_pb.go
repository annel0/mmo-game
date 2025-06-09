package network

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
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

	tcpServer *TCPServerPB
	udpServer *UDPServerPB

	playerEntities map[string]uint64   // connID -> entityID
	sessions       map[string]*Session // connID -> session
	playerAuth     map[uint64]string   // entityID -> username (legacy usage)

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
		playerAuth:     make(map[uint64]string),
		serializer:     protocol.NewMessageSerializer(),
		lastEntityID:   0,
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

// HandleMessage обрабатывает входящие сообщения от клиентов
func (gh *GameHandlerPB) HandleMessage(connID string, msg *protocol.GameMsg) {
	switch msg.Type {
	case protocol.MsgAuth:
		gh.handleAuth(connID, msg)
	case protocol.MsgBlockUpdate:
		gh.handleBlockUpdate(connID, msg)
	case protocol.MsgChunkRequest:
		gh.handleChunkRequest(connID, msg)
	case protocol.MsgEntityAction:
		gh.handleEntityAction(connID, msg)
	case protocol.MsgEntityMove:
		gh.handleEntityMove(connID, msg)
	case protocol.MsgChat:
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
		delete(gh.playerAuth, entityID)

		// Оповещаем других игроков
		despawnMsg := &protocol.EntityDespawnMessage{
			EntityId: entityID,
			Reason:   "disconnected",
		}
		gh.broadcastMessage(protocol.MsgEntityDespawn, despawnMsg)
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

	// Проверяем столкновения с блоками
	blockX := int(math.Floor(newPos.X))
	blockY := int(math.Floor(newPos.Y))

	// Проверяем все блоки вокруг сущности на коллизии
	for x := blockX - 1; x <= blockX+1; x++ {
		for y := blockY - 1; y <= blockY+1; y++ {
			pos := vec.Vec2{X: x, Y: y}
			blockID := gh.GetBlock(pos)

			// Проверяем, является ли блок твердым (не воздухом)
			if blockID != block.AirBlockID {
				// Проверяем коллизию с блоком
				if gh.checkEntityBlockCollision(entity, newPos, pos) {
					// Вызываем обработчик коллизий
					behavior.OnCollision(gh, entity, blockID, newPos)
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
			gh.sendTCPMessage(connID, protocol.MsgEntityMove, moveMsg)
		}
	}
}

// IsSessionValid verifies that the connection has a stored session and the token is still valid.
func (gh *GameHandlerPB) IsSessionValid(connID string) bool {
	gh.mu.RLock()
	sess, ok := gh.sessions[connID]
	gh.mu.RUnlock()
	if !ok {
		return false
	}
	pid, valid, _ := auth.ValidateJWT(sess.Token)
	return valid && pid == sess.PlayerID
}

// handleAuth обрабатывает аутентификацию / регистрацию и создает игровую сущность
func (gh *GameHandlerPB) handleAuth(connID string, msg *protocol.GameMsg) {
	authMsg := &protocol.AuthRequest{}
	if err := gh.serializer.DeserializePayload(msg, authMsg); err != nil {
		log.Printf("Ошибка десериализации Auth: %v", err)
		return
	}

	// Если уже имеется валидная сессия – пропускаем повторную авторизацию
	if gh.IsSessionValid(connID) {
		log.Printf("Повторная авторизация от %s игнорируется", connID)
		return
	}

	var (
		user      *auth.User
		playerID  uint64
		token     string
		isAdmin   bool
		loginFail = func(reason string) {
			resp := &protocol.AuthResponse{Success: false, Message: reason}
			gh.sendTCPMessage(connID, protocol.MsgAuthResponse, resp)
		}
	)

	// branch 1: токен
	if authMsg.Token != nil && *authMsg.Token != "" {
		pid, valid, admin := auth.ValidateJWT(*authMsg.Token)
		if !valid {
			loginFail("invalid token")
			return
		}
		isAdmin = admin
		playerID = pid // Для mock JWT PID — псевдо-значение

		// Попробуем найти пользователя по username (может быть пусто)
		username := authMsg.Username
		if username == "" {
			username = fmt.Sprintf("player%d", playerID)
		}
		u, err := gh.userRepo.GetUserByUsername(username)
		if err == auth.ErrUserNotFound {
			// создаём
			u, err = gh.userRepo.CreateUser(username, "", isAdmin)
			if err != nil {
				loginFail("repository error")
				return
			}
		}
		user = u
		token = *authMsg.Token
	} else {
		// branch 2: username + password
		if authMsg.Username == "" || authMsg.Password == nil {
			loginFail("username/password required")
			return
		}
		u, err := gh.userRepo.GetUserByUsername(authMsg.Username)
		if err != nil {
			loginFail("user not found")
			return
		}
		if !auth.CheckPassword(u.PasswordHash, *authMsg.Password) {
			loginFail("invalid credentials")
			return
		}
		user = u
		isAdmin = u.IsAdmin
		token, err = auth.GenerateJWT(u)
		if err != nil {
			loginFail(fmt.Sprintf("failed to generate token: %v", err))
			return
		}
	}

	// Создаем игровую сущность
	gh.mu.Lock()
	if _, exists := gh.playerEntities[connID]; !exists {
		entityID := gh.generateEntityID()
		gh.playerEntities[connID] = entityID
		gh.playerAuth[entityID] = user.Username
		// Сохраняем сессию
		gh.sessions[connID] = &Session{PlayerID: entityID, Username: user.Username, Token: token, IsAdmin: isAdmin}

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
		playerID = entityID
	} else {
		playerID = gh.playerEntities[connID]
	}
	gh.mu.Unlock()

	// Успешный ответ
	resp := &protocol.AuthResponse{
		Success:   true,
		PlayerId:  playerID,
		Message:   "Авторизация успешна",
		Token:     token,
		WorldName: "default",
	}
	gh.sendTCPMessage(connID, protocol.MsgAuthResponse, resp)

	// Отправляем данные мира
	gh.sendWorldDataToPlayer(connID, playerID)
}

// handleBlockUpdate обрабатывает обновление блока
func (gh *GameHandlerPB) handleBlockUpdate(connID string, msg *protocol.GameMsg) {
	blockUpdate := &protocol.BlockUpdateRequest{}
	if err := gh.serializer.DeserializePayload(msg, blockUpdate); err != nil {
		log.Printf("Ошибка десериализации BlockUpdate: %v", err)
		return
	}

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	_, exists := gh.playerEntities[connID]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("Неавторизованный клиент пытается обновить блок: %s", connID)
		return
	}

	// Валидируем позицию
	if blockUpdate.Position == nil {
		log.Printf("Недействительное обновление блока: позиция nil")
		return
	}

	// Проверяем, что игрок имеет право изменять блоки в данной позиции
	position := vec.Vec2{X: int(blockUpdate.Position.X), Y: int(blockUpdate.Position.Y)}
	
	// Получаем позицию игрока
	gh.mu.RLock()
	entityID, hasEntity := gh.playerEntities[connID]
	gh.mu.RUnlock()
	
	if !hasEntity {
		log.Printf("Игрок не имеет сущности для проверки прав: %s", connID)
		return
	}
	
	playerEntity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		log.Printf("Сущность игрока не найдена для проверки прав: %s", connID)
		return
	}

	// Проверяем дистанцию - игрок может изменять блоки только в радиусе 10 блоков
	playerPos := playerEntity.Position
	distance := math.Sqrt(math.Pow(float64(position.X-playerPos.X), 2) + math.Pow(float64(position.Y-playerPos.Y), 2))
	if distance > 10 {
		log.Printf("Игрок %s пытается изменить блок слишком далеко: %.2f блоков", connID, distance)
		response := &protocol.BlockUpdateResponse{
			Success: false,
			Message: "Block is too far away",
		}
		if gh.tcpServer != nil {
			if conn, exists := gh.tcpServer.connections[connID]; exists {
				conn.sendMessage(protocol.MsgBlockUpdateResponse, response)
			}
		}
		return
	}

	// Валидируем ID блока
	if !block.IsValidBlockID(block.BlockID(blockUpdate.BlockId)) {
		log.Printf("Недействительный ID блока: %d", blockUpdate.BlockId)
		response := &protocol.BlockUpdateResponse{
			Success: false,
			Message: "Invalid block ID",
		}
		if gh.tcpServer != nil {
			if conn, exists := gh.tcpServer.connections[connID]; exists {
				conn.sendMessage(protocol.MsgBlockUpdateResponse, response)
			}
		}
		return
	}

	// Создаем блок для мира
	worldBlock := world.NewBlock(block.BlockID(blockUpdate.BlockId))

	// Устанавливаем метаданные если есть
	if blockUpdate.Metadata != nil && blockUpdate.Metadata.JsonData != "" {
		// Ограничиваем размер метаданных
		if len(blockUpdate.Metadata.JsonData) > 1024 {
			log.Printf("Метаданные блока слишком большие: %d байт", len(blockUpdate.Metadata.JsonData))
			return
		}
		
		metadata, err := protocol.JsonToMap(blockUpdate.Metadata.JsonData)
		if err == nil {
			worldBlock.Payload = metadata
		}
	}

	// Устанавливаем блок в мире
	gh.worldManager.SetBlock(position, worldBlock)

	// Отправляем подтверждение
	response := &protocol.BlockUpdateResponse{
		Success: true,
		BlockId: blockUpdate.BlockId,
		Position: &protocol.Vec2{
			X: blockUpdate.Position.X,
			Y: blockUpdate.Position.Y,
		},
	}

	gh.sendTCPMessage(connID, protocol.MsgBlockUpdateResponse, response)
}

// handleChunkRequest обрабатывает запрос чанка
func (gh *GameHandlerPB) handleChunkRequest(connID string, msg *protocol.GameMsg) {
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

	// Получаем чанк из мира
	chunkPos := vec.Vec2{X: int(chunkRequest.ChunkX), Y: int(chunkRequest.ChunkY)}
	chunk := gh.worldManager.GetChunk(chunkPos)

	// Сериализуем чанк в Protocol Buffers
	// Создаем ChunkData
	chunkData := &protocol.ChunkData{
		ChunkX: chunkRequest.ChunkX,
		ChunkY: chunkRequest.ChunkY,
		Blocks: make([]*protocol.BlockRow, 16), // 16x16 блоков в чанке
	}

	// Заполняем данные блоков
	for y := 0; y < 16; y++ {
		blockIds := make([]uint32, 16)
		for x := 0; x < 16; x++ {
			localPos := vec.Vec2{X: x, Y: y}
			blockIds[x] = uint32(chunk.GetBlock(localPos))
		}
		chunkData.Blocks[y] = &protocol.BlockRow{
			BlockIds: blockIds,
		}
	}

	// Получаем метаданные блоков
	blockMetadata := &protocol.ChunkBlockMetadata{
		BlockMetadata: make(map[string]*protocol.JsonMetadata),
	}

	// Обрабатываем метаданные для каждого блока с метаданными
	for localPos, metadata := range chunk.Metadata {
		if len(metadata) > 0 {
			jsonStr, err := protocol.MapToJsonMetadata(metadata)
			if err == nil {
				key := fmt.Sprintf("%d:%d", localPos.X, localPos.Y)
				blockMetadata.BlockMetadata[key] = &protocol.JsonMetadata{
					JsonData: jsonStr,
				}
			}
		}
	}

	// Добавляем метаданные в чанк
	if len(blockMetadata.BlockMetadata) > 0 {
		metadataJson, err := protocol.MapToJsonMetadata(map[string]interface{}{
			"blockMetadata": blockMetadata,
		})
		if err == nil {
			chunkData.Metadata = &protocol.JsonMetadata{
				JsonData: metadataJson,
			}
		}
	}

	// Отправляем чанк
	gh.sendTCPMessage(connID, protocol.MsgChunkData, chunkData)
}

// handleEntityAction обрабатывает действия сущности
func (gh *GameHandlerPB) handleEntityAction(connID string, msg *protocol.GameMsg) {
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

	gh.sendTCPMessage(connID, protocol.MsgEntityActionResponse, response)

	// Если действие успешно, оповещаем других игроков
	if success {
		gh.broadcastMessage(protocol.MsgEntityAction, action)
	}
}

// handleEntityMove обрабатывает движение сущности
func (gh *GameHandlerPB) handleEntityMove(connID string, msg *protocol.GameMsg) {
	// Упрощенная обработка для примера
	log.Printf("Получено сообщение о движении от %s", connID)

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[connID]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("Неавторизованный клиент перемещает сущность: %s", connID)
		return
	}

	// Получаем сущность из менеджера
	ent, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		log.Printf("Сущность %d не найдена", entityID)
		return
	}

	// Просто логируем информацию о сущности
	log.Printf("Перемещение сущности %d типа %d в позиции (%d, %d)",
		ent.ID, ent.Type, ent.Position.X, ent.Position.Y)
}

// handleChat обрабатывает сообщения чата
func (gh *GameHandlerPB) handleChat(connID string, msg *protocol.GameMsg) {
	// Упрощенная обработка для примера
	log.Printf("Получено сообщение чата от %s", connID)

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	entityID, exists := gh.playerEntities[connID]
	playerName := gh.playerAuth[entityID]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("Неавторизованный клиент отправляет сообщение: %s", connID)
		return
	}

	// Отправляем простое сообщение всем
	gh.broadcastMessage(protocol.MsgChatBroadcast, &protocol.ChatBroadcast{
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
	gh.sendTCPMessage(connID, protocol.MsgChunkData, worldMetadata)

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
			username, exists := gh.playerAuth[entity.ID]
			gh.mu.RUnlock()

			if exists {
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

		gh.sendTCPMessage(connID, protocol.MsgEntityMove, spawnMsg)
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
				Blocks: make([]*protocol.BlockRow, 0, 16), // 16 строк в чанке
			}

			// Заполняем данные блоков для каждой строки
			for blockY := 0; blockY < 16; blockY++ { // Чанк размером 16x16
				blockRow := &protocol.BlockRow{
					BlockIds: make([]uint32, 0, 16),
				}

				for blockX := 0; blockX < 16; blockX++ {
					localPos := vec.Vec2{X: blockX, Y: blockY}
					blockID := chunk.GetBlock(localPos)
					blockRow.BlockIds = append(blockRow.BlockIds, uint32(blockID))
				}

				chunkData.Blocks = append(chunkData.Blocks, blockRow)
			}

			// Отправляем данные чанка
			gh.sendTCPMessage(connID, protocol.MsgChunkData, chunkData)

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

			gh.sendTCPMessage(connID, protocol.MsgEntityMove, updateMsg)
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
	// Временная заглушка до полной реализации
	log.Printf("Создание сущности типа %d с ID %d в позиции (%d, %d)",
		entityType, entityID, position.X, position.Y)

	// Отправляем всем игрокам сообщение о создании сущности
	entityData := &protocol.EntityData{
		Id:       entityID,
		Type:     protocol.EntityType(entityType),
		Position: &protocol.Vec2{X: int32(position.X), Y: int32(position.Y)},
		Active:   true,
	}

	entitySpawn := &protocol.EntitySpawnMessage{
		Entity: entityData,
	}

	gh.broadcastMessage(protocol.MsgEntitySpawn, entitySpawn)

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
	gh.broadcastMessage(protocol.MsgEntityDespawn, despawnMsg)
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
	gh.broadcastMessage(protocol.MsgBlockUpdate, blockUpdate)
}

// broadcastMessage отправляет сообщение всем подключенным клиентам
func (gh *GameHandlerPB) broadcastMessage(msgType protocol.MsgType, payload proto.Message) {
	if gh.tcpServer != nil {
		gh.tcpServer.broadcastMessage(msgType, payload)
	}
}

// sendTCPMessage отправляет сообщение конкретному клиенту через TCP
func (gh *GameHandlerPB) sendTCPMessage(connID string, msgType protocol.MsgType, payload proto.Message) {
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
