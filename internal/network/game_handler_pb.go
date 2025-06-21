package network

import (
	"context"
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
	"github.com/annel0/mmo-game/internal/storage"
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
	positionRepo  storage.PositionRepo // Репозиторий позиций игроков

	tcpServer *TCPServerPB
	udpServer *UDPServerPB

	playerEntities map[string]uint64   // connID -> entityID
	sessions       map[string]*Session // connID -> session

	serializer   *protocol.MessageSerializer
	lastEntityID uint64
	mu           sync.RWMutex

	// Оптимизация частоты обновлений
	tickCounter         int     // Счетчик тиков
	worldUpdateInterval int     // Интервал обновлений в тиках (20 тиков = 1 сек при 20 TPS)
	lastUpdateTime      float64 // Время последнего обновления
}

// Session stores authenticated player data for the lifetime of a TCP connection.
// UserID - постоянный идентификатор аккаунта (для сохранения позиций)
// EntityID - временный идентификатор сущности в текущей сессии
type Session struct {
	UserID   uint64 // Постоянный идентификатор пользователя (бывший PlayerID)
	EntityID uint64 // Идентификатор текущей сущности игрока в мире
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

		serializer:   createMessageSerializer(),
		lastEntityID: 0,

		// Инициализация оптимизации
		tickCounter:         0,
		worldUpdateInterval: 2, // Обновления каждые 10 тиков = 2 раза в секунду при 20 TPS
		lastUpdateTime:      0,
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

// SetPositionRepo устанавливает репозиторий позиций
func (gh *GameHandlerPB) SetPositionRepo(positionRepo storage.PositionRepo) {
	gh.positionRepo = positionRepo
}

// GetEntityPosition возвращает позицию сущности в формате Vec3 (x, y, layer).
// Используется для сохранения позиций игроков.
//
// Параметры:
//
//	entityID - идентификатор сущности
//
// Возвращает:
//
//	vec.Vec3 - позиция в 3D пространстве (где Z = layer)
//	bool - true если сущность найдена
func (gh *GameHandlerPB) GetEntityPosition(entityID uint64) (vec.Vec3, bool) {
	entity, exists := gh.entityManager.GetEntity(entityID)
	if !exists {
		return vec.Vec3{}, false
	}

	// Определяем layer на основе того, в каком слое находится игрок
	// Пока используем 1 как дефолтный layer
	layer := 1

	// В будущем можно добавить логику определения layer на основе:
	// - текущего блока под игроком
	// - специального поля в сущности
	// - глобальных настроек мира

	return vec.Vec3{
		X: entity.Position.X,
		Y: entity.Position.Y,
		Z: layer,
	}, true
}

// GetDefaultSpawnPosition возвращает позицию для спавна по умолчанию.
//
// Возвращает:
//
//	vec.Vec3 - позиция спавна по умолчанию
func (gh *GameHandlerPB) GetDefaultSpawnPosition() vec.Vec3 {
	// Пока используем фиксированную позицию спавна
	// В будущем можно добавить логику поиска безопасной позиции спавна
	return vec.Vec3{X: 0, Y: 0, Z: 1}
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

	// Находим сессию игрока
	session, sessionExists := gh.sessions[connID]
	entityID, entityExists := gh.playerEntities[connID]

	if sessionExists && entityExists {
		// Сохраняем позицию игрока перед отключением
		if gh.positionRepo != nil {
			if currentPos, found := gh.GetEntityPosition(entityID); found {
				ctx := context.Background()
				if err := gh.positionRepo.Save(ctx, session.UserID, currentPos); err != nil {
					log.Printf("❌ Ошибка сохранения позиции для пользователя %d: %v", session.UserID, err)
				} else {
					log.Printf("💾 Позиция игрока %s сохранена: (%d, %d, %d)", session.Username, currentPos.X, currentPos.Y, currentPos.Z)
				}
			} else {
				log.Printf("⚠️ Не удалось получить позицию сущности %d для сохранения", entityID)
			}
		} else {
			log.Printf("⚠️ Репозиторий позиций не настроен, позиция не сохранена")
		}

		// Удаляем сущность из мира
		gh.DespawnEntity(entityID)

		// Удаляем привязки
		delete(gh.playerEntities, connID)
		delete(gh.sessions, connID)

		// Оповещаем других игроков
		despawnMsg := &protocol.EntityDespawnMessage{
			EntityId: entityID,
			Reason:   "disconnected",
		}
		gh.broadcastMessage(protocol.MessageType_ENTITY_DESPAWN, despawnMsg)

		log.Printf("🚪 Клиент %s (%s) отключен, позиция сохранена", connID, session.Username)
	} else {
		log.Printf("🚪 Клиент %s отключен (сессия не найдена)", connID)
	}
}

// Tick обновляет состояние игрового мира
func (gh *GameHandlerPB) Tick(dt float64) {
	// Обновляем все сущности
	gh.entityManager.UpdateEntities(dt, gh)

	// Увеличиваем счетчик тиков
	gh.tickCounter++

	// ОПТИМИЗАЦИЯ: Отправляем обновления не каждый тик, а с заданным интервалом
	// Это снижает нагрузку на сеть с 20 обновлений/сек до 2 обновлений/сек
	if gh.tickCounter%gh.worldUpdateInterval == 0 {
		gh.sendWorldUpdates()
		//log.Printf("🔄 Тик %d: отправка world updates (интервал: %d тиков)", gh.tickCounter, gh.worldUpdateInterval)
	}

	// Периодическое автосохранение позиций (каждые 30 секунд)
	gh.autoSavePositions()
}

// autoSavePositions выполняет автосохранение позиций всех онлайн игроков.
// Вызывается периодически из Tick для предотвращения потери данных.
func (gh *GameHandlerPB) autoSavePositions() {
	// Используем простой таймер - проверяем раз в 30 секунд
	// В продакшене лучше использовать отдельный тикер
	const autoSaveInterval = 30.0 // секунд

	// Статическая переменная для отслеживания времени
	// TODO: В будущем заменить на более элегантное решение
	now := float64(time.Now().Unix())

	// Читаем из контекста GameHandlerPB последнее время автосохранения
	// Для простоты пока используем проверку по времени
	gh.mu.RLock()
	sessionsCount := len(gh.sessions)
	playerCount := len(gh.playerEntities)
	gh.mu.RUnlock()

	// Если нет игроков онлайн, пропускаем автосохранение
	if sessionsCount == 0 || playerCount == 0 {
		return
	}

	// Простая проверка - автосохранение раз в 30 секунд
	// В реальном коде стоит добавить поле lastAutoSave в структуру
	if int(now)%int(autoSaveInterval) != 0 {
		return
	}

	if gh.positionRepo == nil {
		return // Репозиторий не настроен
	}

	// Собираем позиции всех онлайн игроков
	positionsToSave := make(map[uint64]vec.Vec3)

	gh.mu.RLock()
	for connID, session := range gh.sessions {
		if entityID, exists := gh.playerEntities[connID]; exists {
			if currentPos, found := gh.GetEntityPosition(entityID); found {
				positionsToSave[session.UserID] = currentPos
			}
		}
	}
	gh.mu.RUnlock()

	// Выполняем пакетное сохранение позиций
	if len(positionsToSave) > 0 {
		ctx := context.Background()
		if err := gh.positionRepo.BatchSave(ctx, positionsToSave); err != nil {
			log.Printf("❌ Ошибка автосохранения позиций игроков: %v", err)
		} else {
			log.Printf("💾 Автосохранение выполнено для %d игроков", len(positionsToSave))
		}
	}
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
		resp := &protocol.AuthResponseMessage{Success: false, Message: "Server authentication error"}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, resp)
		return
	}

	authMsg := &protocol.AuthMessage{}
	if err := gh.serializer.DeserializePayload(msg, authMsg); err != nil {
		log.Printf("❌ Ошибка десериализации Auth: %v", err)
		resp := &protocol.AuthResponseMessage{Success: false, Message: "Invalid request format"}
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
		resp := &protocol.AuthResponseMessage{Success: false, Message: "Authentication service error"}
		gh.sendTCPMessage(connID, protocol.MessageType_AUTH_RESPONSE, resp)
		return
	}

	// Если аутентификация не удалась
	if !authResult.Success {
		log.Printf("❌ Аутентификация не удалась для %s: %s", authMsg.Username, authResult.Message)
		authResp := &protocol.AuthResponseMessage{
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
		authResp := &protocol.AuthResponseMessage{
			Success:   true,
			Message:   authResult.Message,
			PlayerId:  entityID,
			JwtToken:  &authResult.Token,
			WorldName: "main_world",
			ServerInfo: &protocol.ServerInfo{
				Version:     "1.0.0",
				Environment: "development",
			},
		}

		gh.sessions[connID] = &Session{
			UserID:   authResult.UserID, // Постоянный идентификатор аккаунта
			EntityID: entityID,          // Временный идентификатор сущности
			Username: username,
			Token:    authResult.Token,
			IsAdmin:  isAdmin,
		}

		log.Printf("✅ Создана игровая сущность %d для пользователя %s", entityID, username)

		// Загружаем сохраненную позицию игрока или используем дефолтную
		var spawnPos vec.Vec2
		if gh.positionRepo != nil {
			if savedPos, found, err := gh.positionRepo.Load(context.Background(), authResult.UserID); err != nil {
				log.Printf("⚠️ Ошибка загрузки позиции для пользователя %d: %v", authResult.UserID, err)
				defaultPos := gh.GetDefaultSpawnPosition()
				spawnPos = defaultPos.ToVec2()
			} else if found {
				log.Printf("📍 Загружена сохраненная позиция для %s: (%d, %d, %d)", username, savedPos.X, savedPos.Y, savedPos.Z)
				spawnPos = savedPos.ToVec2()
			} else {
				log.Printf("🆕 Первый вход пользователя %s, используем позицию спавна по умолчанию", username)
				defaultPos := gh.GetDefaultSpawnPosition()
				spawnPos = defaultPos.ToVec2()
			}
		} else {
			log.Printf("⚠️ Репозиторий позиций не настроен, используем позицию спавна по умолчанию")
			defaultPos := gh.GetDefaultSpawnPosition()
			spawnPos = defaultPos.ToVec2()
		}

		// Создаем сущность игрока в мире
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
		authResp := &protocol.AuthResponseMessage{
			Success:   true,
			Message:   "Already authenticated",
			PlayerId:  entityID,
			JwtToken:  &authResult.Token,
			WorldName: "main_world",
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

	// === Валидация входных данных ===
	if blockUpdate.Position == nil {
		log.Printf("Недействительное обновление блока: позиция nil")
		return
	}

	pos := vec.Vec2{X: int(blockUpdate.Position.X), Y: int(blockUpdate.Position.Y)}

	// Проверяем, что клиент авторизован
	gh.mu.RLock()
	playerEntityID, exists := gh.playerEntities[connID]
	gh.mu.RUnlock()

	if !exists {
		log.Printf("❌ Неавторизованный клиент пытается изменить блок: %s", connID)
		return
	}

	// Получаем сущность игрока для проверки расстояния
	playerEntity, exists := gh.entityManager.GetEntity(playerEntityID)
	if !exists || playerEntity == nil {
		log.Printf("❌ Сущность игрока не найдена: %d", playerEntityID)
		return
	}

	// Проверяем расстояние до блока (защита от читов)
	blockPosFloat := vec.Vec2Float{X: float64(pos.X), Y: float64(pos.Y)}
	distance := playerEntity.PrecisePos.DistanceTo(blockPosFloat)
	const maxReachDistance = 10.0 // Максимальная дистанция взаимодействия
	if distance > maxReachDistance {
		log.Printf("❌ Игрок %d пытается изменить блок слишком далеко: %.2f > %.2f",
			playerEntityID, distance, maxReachDistance)
		return
	}

	// Валидация ID блока
	if blockUpdate.BlockId > 1000 { // Разумный лимит для ID блока
		log.Printf("❌ Недопустимый ID блока: %d", blockUpdate.BlockId)
		return
	}

	// Валидация размера метаданных
	if blockUpdate.Metadata != nil && len(blockUpdate.Metadata.JsonData) > 1024 {
		log.Printf("❌ Слишком большие метаданные блока: %d байт", len(blockUpdate.Metadata.JsonData))
		return
	}

	// Определяем слой для обновления (по умолчанию активный)
	layer := world.LayerActive
	if blockUpdate.Layer == protocol.BlockLayer_FLOOR {
		layer = world.LayerFloor
	} else if blockUpdate.Layer == protocol.BlockLayer_CEILING {
		layer = world.LayerCeiling
	}

	// Получаем текущий блок на указанном слое
	oldBlock := gh.worldManager.GetBlockLayer(pos, layer)
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

	// Применяем изменения на указанном слое
	blockObj := world.NewBlock(newID)
	blockObj.Payload = newPayload
	gh.worldManager.SetBlockLayer(pos, layer, blockObj)

	// Формируем ответ
	metaStr, _ := protocol.MapToJsonMetadata(newPayload)
	respMeta := &protocol.JsonMetadata{JsonData: metaStr}
	response := &protocol.BlockUpdateResponseMessage{
		Success:  result.Success,
		Message:  result.Message,
		BlockId:  uint32(newID),
		Position: blockUpdate.Position,
		Layer:    blockUpdate.Layer,
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
	success, message, shouldBroadcast := gh.processEntityAction(entityID, action)

	// Отправляем ответ
	response := &protocol.EntityActionResponse{
		Success: success,
		Message: message,
	}

	gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_ACTION_RESPONSE, response)

	// Если действие успешно и требует трансляции, оповещаем других игроков
	if success && shouldBroadcast {
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
	gh.broadcastMessage(protocol.MessageType_CHAT_BROADCAST, &protocol.ChatBroadcastMessage{
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
				if session.EntityID == entity.ID {
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

		// ИСПРАВЛЕНИЕ: Отправляем сообщение только если есть сущности для отправки
		// Это предотвращает отправку пустых ENTITY_MOVE сообщений каждый тик
		if len(entityDataList) > 0 {
			updateMsg := &protocol.EntityMoveMessage{
				Entities: entityDataList,
			}

			// Добавляем детальное логирование для диагностики (только первые 5 сущностей)
			log.Printf("🔄 Отправка ENTITY_MOVE клиенту %s: %d сущностей", connID, len(entityDataList))
			maxLog := len(entityDataList)
			if maxLog > 3 { // Ограничиваем детальный лог до 3 сущностей
				maxLog = 3
			}
			for i := 0; i < maxLog; i++ {
				entityData := entityDataList[i]
				log.Printf("  [%d] Entity ID=%d, Type=%v, Pos=(%d,%d)",
					i, entityData.Id, entityData.Type, entityData.Position.X, entityData.Position.Y)
			}
			if len(entityDataList) > maxLog {
				log.Printf("  ... и еще %d сущностей", len(entityDataList)-maxLog)
			}

			gh.sendTCPMessage(connID, protocol.MessageType_ENTITY_MOVE, updateMsg)
		} else {
			// Логируем случаи, когда сообщение не отправляется (реже для снижения спама)
			if gh.tickCounter%100 == 0 { // Логируем каждые 100 тиков = раз в 5 секунд
				log.Printf("⏭️ Пропуск ENTITY_MOVE для клиента %s: нет сущностей для отправки (всего видимых: %d)", connID, len(visibleEntities))
			}
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

// processEntityAction обрабатывает различные типы действий сущности
// Возвращает: success, message, shouldBroadcast
func (gh *GameHandlerPB) processEntityAction(actorID uint64, action *protocol.EntityActionRequest) (bool, string, bool) {
	// Получаем сущность актора
	actor, exists := gh.entityManager.GetEntity(actorID)
	if !exists {
		return false, "Сущность не найдена", false
	}

	// Обрабатываем действие в зависимости от типа
	switch action.ActionType {
	case protocol.EntityActionType_ACTION_INTERACT:
		return gh.handleInteractAction(actor, action)

	case protocol.EntityActionType_ACTION_ATTACK:
		return gh.handleAttackAction(actor, action)

	case protocol.EntityActionType_ACTION_USE_ITEM:
		return gh.handleUseItemAction(actor, action)

	case protocol.EntityActionType_ACTION_PICKUP:
		return gh.handlePickupAction(actor, action)

	case protocol.EntityActionType_ACTION_DROP:
		return gh.handleDropAction(actor, action)

	case protocol.EntityActionType_ACTION_BUILD_PLACE:
		return gh.handleBuildPlaceAction(actor, action)

	case protocol.EntityActionType_ACTION_BUILD_BREAK:
		return gh.handleBuildBreakAction(actor, action)

	case protocol.EntityActionType_ACTION_EMOTE:
		return gh.handleEmoteAction(actor, action)

	case protocol.EntityActionType_ACTION_RESPAWN:
		return gh.handleRespawnAction(actor, action)

	default:
		return false, "Неизвестный тип действия", false
	}
}

// handleInteractAction обрабатывает взаимодействие с объектами
func (gh *GameHandlerPB) handleInteractAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	// Если указана целевая сущность
	if action.TargetId != nil {
		target, exists := gh.entityManager.GetEntity(*action.TargetId)
		if !exists {
			return false, "Цель не найдена", false
		}

		// Проверяем расстояние
		distance := gh.calculateDistance(actor.Position, target.Position)
		if distance > 3.0 { // Максимальное расстояние взаимодействия
			return false, "Слишком далеко", false
		}

		// Обрабатываем взаимодействие с разными типами сущностей
		switch target.Type {
		case entity.EntityTypeNPC:
			return true, "Разговор с NPC", true
		case entity.EntityTypePlayer:
			return true, "Взаимодействие с игроком", true
		default:
			return false, "Нельзя взаимодействовать с этим объектом", false
		}
	}

	// Если указана позиция - взаимодействие с блоком
	if action.Position != nil {
		blockPos := vec.Vec2{X: int(action.Position.X), Y: int(action.Position.Y)}

		// Проверяем расстояние до блока
		distance := gh.calculateDistance(actor.Position, blockPos)
		if distance > 3.0 {
			return false, "Слишком далеко", false
		}

		blockData := gh.worldManager.GetBlock(blockPos)

		// Проверяем, можно ли взаимодействовать с блоком
		if behavior, exists := block.Get(blockData.ID); exists {
			if interactable, ok := behavior.(interface{ IsInteractable() bool }); ok {
				if interactable.IsInteractable() {
					return true, "Взаимодействие с блоком", true
				}
			}
		}

		return false, "С этим блоком нельзя взаимодействовать", false
	}

	return false, "Не указана цель взаимодействия", false
}

// handleAttackAction обрабатывает атаку
func (gh *GameHandlerPB) handleAttackAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	if action.TargetId == nil {
		return false, "Не указана цель атаки", false
	}

	target, exists := gh.entityManager.GetEntity(*action.TargetId)
	if !exists {
		return false, "Цель не найдена", false
	}

	// Проверяем расстояние атаки
	distance := gh.calculateDistance(actor.Position, target.Position)
	attackRange := 2.0 // Базовая дальность атаки

	if distance > attackRange {
		return false, "Слишком далеко для атаки", false
	}

	// Нельзя атаковать себя
	if actor.ID == target.ID {
		return false, "Нельзя атаковать себя", false
	}

	// Базовый урон
	damage := 10

	// Применяем урон к цели
	if behavior, ok := gh.entityManager.GetBehavior(target.Type); ok {
		if behavior.OnDamage(gh, target, damage, actor) {
			// Цель получила урон
			return true, "Атака успешна", true
		} else {
			return false, "Атака заблокирована", false
		}
	}

	return true, "Атака выполнена", true
}

// handleUseItemAction обрабатывает использование предмета
func (gh *GameHandlerPB) handleUseItemAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	if action.ItemId == nil {
		return false, "Не указан предмет", false
	}

	itemID := *action.ItemId

	// Проверяем, есть ли предмет у игрока (заглушка)
	// В реальной реализации нужно проверить инвентарь

	// Обрабатываем использование разных предметов
	switch itemID {
	case 1: // Зелье лечения
		return true, "Использовано зелье лечения", false
	case 2: // Инструмент
		return true, "Использован инструмент", false
	default:
		return false, "Неизвестный предмет", false
	}
}

// handlePickupAction обрабатывает подбор предметов
func (gh *GameHandlerPB) handlePickupAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	if action.TargetId == nil {
		return false, "Не указан предмет для подбора", false
	}

	target, exists := gh.entityManager.GetEntity(*action.TargetId)
	if !exists {
		return false, "Предмет не найден", false
	}

	// Проверяем, что это предмет
	if target.Type != entity.EntityTypeItem {
		return false, "Это не предмет", false
	}

	// Проверяем расстояние
	distance := gh.calculateDistance(actor.Position, target.Position)
	if distance > 2.0 {
		return false, "Слишком далеко", false
	}

	// Удаляем предмет из мира
	gh.DespawnEntity(target.ID)

	return true, "Предмет подобран", true
}

// handleDropAction обрабатывает выбрасывание предметов
func (gh *GameHandlerPB) handleDropAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	if action.ItemId == nil {
		return false, "Не указан предмет", false
	}

	// Определяем позицию для выбрасывания
	dropPos := actor.Position
	if action.Position != nil {
		dropPos = vec.Vec2{X: int(action.Position.X), Y: int(action.Position.Y)}

		// Проверяем расстояние
		distance := gh.calculateDistance(actor.Position, dropPos)
		if distance > 2.0 {
			return false, "Слишком далеко", false
		}
	}

	// Проверяем, свободна ли позиция
	if !gh.isPositionWalkable(dropPos) {
		return false, "Позиция занята", false
	}

	// Создаем предмет в мире
	gh.SpawnEntity(entity.EntityTypeItem, dropPos)

	return true, "Предмет выброшен", true
}

// handleBuildPlaceAction обрабатывает размещение блоков
func (gh *GameHandlerPB) handleBuildPlaceAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	if action.Position == nil {
		return false, "Не указана позиция", false
	}

	blockPos := vec.Vec2{X: int(action.Position.X), Y: int(action.Position.Y)}

	// Проверяем расстояние
	distance := gh.calculateDistance(actor.Position, blockPos)
	if distance > 5.0 {
		return false, "Слишком далеко", false
	}

	// Определяем тип блока (по умолчанию камень)
	blockID := block.StoneBlockID
	if action.ItemId != nil {
		// Преобразуем ID предмета в ID блока
		blockID = block.BlockID(*action.ItemId)
	}

	// Проверяем, можно ли разместить блок
	currentBlock := gh.worldManager.GetBlock(blockPos)
	if currentBlock.ID != block.AirBlockID {
		return false, "Позиция занята", false
	}

	// Размещаем блок
	gh.worldManager.SetBlock(blockPos, world.NewBlock(blockID))

	return true, "Блок размещён", true
}

// handleBuildBreakAction обрабатывает разрушение блоков
func (gh *GameHandlerPB) handleBuildBreakAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	if action.Position == nil {
		return false, "Не указана позиция", false
	}

	blockPos := vec.Vec2{X: int(action.Position.X), Y: int(action.Position.Y)}

	// Проверяем расстояние
	distance := gh.calculateDistance(actor.Position, blockPos)
	if distance > 5.0 {
		return false, "Слишком далеко", false
	}

	// Получаем текущий блок
	currentBlock := gh.worldManager.GetBlock(blockPos)
	if currentBlock.ID == block.AirBlockID {
		return false, "Нечего ломать", false
	}

	// Проверяем, можно ли сломать блок
	if behavior, exists := block.Get(currentBlock.ID); exists {
		if breakable, ok := behavior.(interface{ IsBreakable() bool }); ok {
			if !breakable.IsBreakable() {
				return false, "Блок нельзя сломать", false
			}
		}
	}

	// Ломаем блок
	gh.worldManager.SetBlock(blockPos, world.NewBlock(block.AirBlockID))

	// Можно добавить выпадение предметов
	gh.SpawnEntity(entity.EntityTypeItem, blockPos)

	return true, "Блок сломан", true
}

// handleEmoteAction обрабатывает эмоции
func (gh *GameHandlerPB) handleEmoteAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	// Эмоции всегда транслируются другим игрокам
	return true, "Эмоция выполнена", true
}

// handleRespawnAction обрабатывает возрождение
func (gh *GameHandlerPB) handleRespawnAction(actor *entity.Entity, action *protocol.EntityActionRequest) (bool, string, bool) {
	// Проверяем, нужно ли возрождение
	if actor.Active {
		return false, "Игрок уже жив", false
	}

	// Возрождаем игрока на спавне
	spawnPos := gh.GetDefaultSpawnPosition()
	actor.Position = vec.Vec2{X: int(spawnPos.X), Y: int(spawnPos.Y)}
	actor.PrecisePos = vec.Vec2Float{X: float64(spawnPos.X), Y: float64(spawnPos.Y)}
	actor.Active = true

	return true, "Игрок возрождён", true
}

// calculateDistance вычисляет расстояние между двумя позициями
func (gh *GameHandlerPB) calculateDistance(pos1, pos2 vec.Vec2) float64 {
	dx := float64(pos1.X - pos2.X)
	dy := float64(pos1.Y - pos2.Y)
	return math.Sqrt(dx*dx + dy*dy)
}
