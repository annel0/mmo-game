package network

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
)

// MessageType определяет тип сообщения
type MessageType uint16

const (
	MsgAuth          MessageType = iota // 0: Авторизация
	MsgChunkData                        // 1: Данные чанка
	MsgPing                             // 2: Пинг для поддержания соединения
	MsgBlockUpdate                      // 3: Обновление блока
	MsgEntitySpawn                      // 4: Появление сущности
	MsgEntityMove                       // 5: Перемещение сущности
	MsgEntityAction                     // 6: Действие сущности
	MsgChat                             // 7: Сообщение чата
	MsgEntityDespawn                    // 8: Удаление сущности
	MsgChunkRequest                     // 9: Запрос чанка
)

// TCPServer обрабатывает TCP соединения
type TCPServer struct {
	listener     net.Listener
	connections  map[uint64]*TCPConnection
	worldManager *world.WorldManager
	nextConnID   uint64
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
}

// TCPConnection представляет соединение с клиентом
type TCPConnection struct {
	id         uint64
	conn       net.Conn
	playerID   uint64
	authorized bool
	server     *TCPServer
	lastPing   time.Time
}

// NewTCPServer создаёт новый TCP сервер
func NewTCPServer(address string, worldManager *world.WorldManager) (*TCPServer, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TCPServer{
		listener:     listener,
		connections:  make(map[uint64]*TCPConnection),
		worldManager: worldManager,
		nextConnID:   1,
		ctx:          ctx,
		cancel:       cancel,
	}, nil
}

// Start запускает TCP сервер
func (s *TCPServer) Start() {
	go s.acceptLoop()
	go s.healthCheckLoop()
}

// Stop останавливает TCP сервер
func (s *TCPServer) Stop() {
	s.cancel()
	s.listener.Close()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, conn := range s.connections {
		conn.conn.Close()
	}
}

// acceptLoop принимает новые соединения
func (s *TCPServer) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				log.Printf("Ошибка принятия соединения: %v", err)
				continue
			}

			s.mu.Lock()
			connID := s.nextConnID
			s.nextConnID++

			tcpConn := &TCPConnection{
				id:         connID,
				conn:       conn,
				server:     s,
				lastPing:   time.Now(),
				authorized: false,
			}

			s.connections[connID] = tcpConn
			s.mu.Unlock()

			go tcpConn.handleConnection()
		}
	}
}

// healthCheckLoop проверяет активность соединений
func (s *TCPServer) healthCheckLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, conn := range s.connections {
				// Если клиент не пинговался более 2 минут, отключаем его
				if time.Since(conn.lastPing) > 2*time.Minute {
					log.Printf("Соединение %d отключено из-за таймаута", id)
					conn.conn.Close()
					delete(s.connections, id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// handleConnection обрабатывает соединение с клиентом
func (c *TCPConnection) handleConnection() {
	defer func() {
		c.conn.Close()

		c.server.mu.Lock()
		delete(c.server.connections, c.id)
		c.server.mu.Unlock()

		// Если клиент был авторизован, удаляем его сущность из мира
		if c.authorized && c.playerID > 0 {
			// Получаем текущую позицию игрока (возможно из кеша или запроса к WorldManager)
			// Для примера используем 0,0
			playerPos := vec.Vec2{X: 0, Y: 0}
			c.server.worldManager.DespawnEntity(c.playerID, playerPos)
			log.Printf("Сущность игрока %d удалена из мира", c.playerID)
		}

		log.Printf("Соединение %d закрыто", c.id)
	}()

	headerBuf := make([]byte, 6) // 4 байта длины + 2 байта тип сообщения

	for {
		// Читаем заголовок
		_, err := c.conn.Read(headerBuf)
		if err != nil {
			log.Printf("Ошибка чтения заголовка: %v", err)
			return
		}

		msgLen := binary.BigEndian.Uint32(headerBuf[0:4])
		msgType := MessageType(binary.BigEndian.Uint16(headerBuf[4:6]))

		// Проверка на максимальный размер сообщения
		if msgLen > 1024*1024 { // 1 МБ
			log.Printf("Слишком большое сообщение: %d байт", msgLen)
			return
		}

		// Читаем тело сообщения
		msgBuf := make([]byte, msgLen)
		_, err = c.conn.Read(msgBuf)
		if err != nil {
			log.Printf("Ошибка чтения тела сообщения: %v", err)
			return
		}

		// Обрабатываем сообщение
		c.handleMessage(msgType, msgBuf)

		// Обновляем время последнего активного взаимодействия
		c.lastPing = time.Now()
	}
}

// handleMessage обрабатывает сообщение от клиента
func (c *TCPConnection) handleMessage(msgType MessageType, data []byte) {
	// Если не авторизован, принимаем только авторизацию
	if !c.authorized && msgType != MsgAuth {
		log.Printf("Неавторизованное сообщение от клиента %d", c.id)
		c.sendErrorResponse("Требуется авторизация")
		return
	}

	switch msgType {
	case MsgAuth:
		c.handleAuth(data)
	case MsgBlockUpdate:
		c.handleBlockUpdate(data)
	case MsgEntityAction:
		c.handleEntityAction(data)
	case MsgChat:
		c.handleChat(data)
	case MsgChunkRequest:
		c.handleChunkRequest(data)
	default:
		log.Printf("Неизвестный тип сообщения: %d", msgType)
	}
}

// sendMessage отправляет сообщение клиенту
func (c *TCPConnection) sendMessage(msgType MessageType, data []byte) error {
	headerBuf := make([]byte, 6)
	binary.BigEndian.PutUint32(headerBuf[0:4], uint32(len(data)))
	binary.BigEndian.PutUint16(headerBuf[4:6], uint16(msgType))

	// Отправляем заголовок
	_, err := c.conn.Write(headerBuf)
	if err != nil {
		return err
	}

	// Отправляем тело
	_, err = c.conn.Write(data)
	return err
}

// sendErrorResponse отправляет сообщение об ошибке клиенту
func (c *TCPConnection) sendErrorResponse(message string) {
	errResp := JSONAuthResponse{
		Success: false,
		Message: message,
	}

	data, err := json.Marshal(errResp)
	if err != nil {
		log.Printf("Ошибка сериализации ответа: %v", err)
		return
	}

	c.sendMessage(MsgAuth, data)
}

// Реализация обработчиков сообщений

// handleAuth обрабатывает запрос на авторизацию
func (c *TCPConnection) handleAuth(data []byte) {
	// Если клиент уже авторизован, игнорируем повторную авторизацию
	if c.authorized {
		log.Printf("Клиент %d уже авторизован", c.id)
		return
	}

	// Десериализуем запрос
	authReq, err := DeserializeJSONAuth(data)
	if err != nil {
		log.Printf("Ошибка разбора запроса авторизации: %v", err)
		c.sendErrorResponse("Неверный формат запроса")
		return
	}

	// В реальном приложении здесь будет проверка учетных данных
	// Для примера принимаем любую авторизацию с непустым именем пользователя
	if authReq.Username == "" {
		c.sendErrorResponse("Имя пользователя не может быть пустым")
		return
	}

	// Генерируем уникальный ID для игрока
	c.playerID = c.server.worldManager.GenerateEntityID()
	c.authorized = true

	// Создаем начальное положение игрока (в центре мира)
	startPos := vec.Vec2{X: 0, Y: 0}

	// Создаем сущность в мире
	// Дополнительные данные для сущности
	entityData := map[string]interface{}{
		"username": authReq.Username,
		"type":     uint16(1), // Тип - игрок
	}

	// Спавним сущность в мире
	c.server.worldManager.SpawnEntity(1, startPos, entityData)

	// Отправляем успешный ответ
	authResp := JSONAuthResponse{
		Success:  true,
		PlayerID: c.playerID,
		Token:    "token-" + authReq.Username, // В реальном приложении будет настоящий токен
		Message:  "Авторизация успешна",
	}

	respData, err := json.Marshal(authResp)
	if err != nil {
		log.Printf("Ошибка сериализации ответа: %v", err)
		return
	}

	err = c.sendMessage(MsgAuth, respData)
	if err != nil {
		log.Printf("Ошибка отправки ответа: %v", err)
		return
	}

	log.Printf("Клиент %d авторизован как игрок %d с именем %s", c.id, c.playerID, authReq.Username)

	// Автоматически отправляем начальные чанки вокруг игрока
	c.sendInitialChunks(startPos)
}

// sendInitialChunks отправляет начальные чанки вокруг указанной позиции
func (c *TCPConnection) sendInitialChunks(playerPos vec.Vec2) {
	// Определяем координаты чанка игрока
	playerChunkX := playerPos.X >> 4 // Деление на 16
	playerChunkY := playerPos.Y >> 4 // Деление на 16

	// Радиус видимости в чанках (можно настроить)
	viewDistance := 2

	log.Printf("Отправка начальных чанков для игрока %d вокруг позиции (%d, %d)",
		c.playerID, playerPos.X, playerPos.Y)

	// Отправляем чанки в радиусе видимости
	for x := playerChunkX - viewDistance; x <= playerChunkX+viewDistance; x++ {
		for y := playerChunkY - viewDistance; y <= playerChunkY+viewDistance; y++ {
			// Получаем чанк
			chunk := c.server.worldManager.GetChunk(vec.Vec2{X: x, Y: y})
			if chunk == nil {
				log.Printf("Чанк (%d, %d) не найден для начальной отправки", x, y)
				continue
			}

			// Преобразуем чанк в формат JSON
			chunkData := ConvertChunkToJSON(chunk)

			// Сериализуем данные
			respData, err := json.Marshal(chunkData)
			if err != nil {
				log.Printf("Ошибка сериализации данных чанка (%d, %d): %v", x, y, err)
				continue
			}

			// Отправляем данные клиенту
			err = c.sendMessage(MsgChunkData, respData)
			if err != nil {
				log.Printf("Ошибка отправки данных чанка (%d, %d): %v", x, y, err)
				continue
			}

			log.Printf("Отправлен начальный чанк (%d, %d) игроку %d", x, y, c.playerID)
		}
	}

	log.Printf("Завершена отправка начальных чанков для игрока %d", c.playerID)
}

// handleBlockUpdate обрабатывает запрос на изменение блока
func (c *TCPConnection) handleBlockUpdate(data []byte) {
	// Десериализуем запрос
	updateReq, err := DeserializeJSONBlockUpdate(data)
	if err != nil {
		log.Printf("Ошибка разбора запроса обновления блока: %v", err)
		return
	}

	// Создаем позицию блока
	blockPos := vec.Vec2{X: updateReq.X, Y: updateReq.Y}

	// Получаем текущий блок из мира
	currentBlock := c.server.worldManager.GetBlock(blockPos)

	// Получаем поведение блока
	behavior, exists := currentBlock.GetBehavior()
	if !exists {
		log.Printf("Не найдено поведение для блока %d", currentBlock.ID)

		// Отправляем ошибку
		response := JSONBlockUpdateResponse{
			Success: false,
			Message: "Неизвестный тип блока",
		}

		respData, _ := json.Marshal(response)
		c.sendMessage(MsgBlockUpdate, respData)
		return
	}

	// Обрабатываем взаимодействие с блоком
	action := "mine" // По умолчанию
	if actionParam, ok := updateReq.Payload["action"].(string); ok {
		action = actionParam
	}

	newBlockID, newPayload, result := behavior.HandleInteraction(action, currentBlock.Payload, updateReq.Payload)

	// Обновляем блок, если взаимодействие было успешным
	if result.Success {
		// Создаем новый блок с обновленными данными
		newBlock := world.Block{
			ID:      newBlockID,
			Payload: newPayload,
		}

		// Изменяем блок в мире
		c.server.worldManager.SetBlock(blockPos, newBlock)

		log.Printf("Игрок %d взаимодействовал с блоком на позиции (%d, %d), блок изменен на тип %d",
			c.playerID, blockPos.X, blockPos.Y, newBlockID)
	}

	// Отправляем ответ с обновленным состоянием блока
	response := JSONBlockUpdateResponse{
		Success: result.Success,
		Message: result.Message,
		BlockID: newBlockID,
		X:       blockPos.X,
		Y:       blockPos.Y,
		Payload: newPayload,
		Effects: result.Effects,
	}

	respData, err := json.Marshal(response)
	if err != nil {
		log.Printf("Ошибка сериализации ответа: %v", err)
		return
	}

	err = c.sendMessage(MsgBlockUpdate, respData)
	if err != nil {
		log.Printf("Ошибка отправки ответа: %v", err)
		return
	}
}

// handleEntityAction обрабатывает запрос на действие сущности
func (c *TCPConnection) handleEntityAction(data []byte) {
	// Десериализуем запрос
	actionReq, err := DeserializeJSONEntityAction(data)
	if err != nil {
		log.Printf("Ошибка разбора запроса действия сущности: %v", err)
		return
	}

	// Определяем тип действия
	var success bool
	var message string
	var results map[string]interface{}

	switch actionReq.ActionType {
	case JSONEntityActionInteract:
		// Обработка взаимодействия с сущностью
		// Получаем целевую сущность
		targetID := actionReq.TargetID

		// Проверяем возможность взаимодействия (расстояние и т.д.)
		// В реальной игре здесь будет проверка на расстояние, видимость и т.д.

		// Выполняем действие
		// Это заглушка, в реальной игре здесь будет вызов соответствующих методов
		success = true
		message = "Взаимодействие успешно"
		results = map[string]interface{}{
			"interaction_type": "talk",
			"target_id":        targetID,
		}

		log.Printf("Игрок %d взаимодействует с сущностью %d", c.playerID, targetID)

	case JSONEntityActionAttack:
		// Обработка атаки
		targetID := actionReq.TargetID

		// Проверяем возможность атаки
		// В реальной игре здесь будет проверка на расстояние, видимость и т.д.

		// Выполняем атаку
		// Это заглушка, в реальной игре здесь будет вызов соответствующих методов
		success = true
		message = "Атака выполнена"
		results = map[string]interface{}{
			"damage":    10,
			"target_id": targetID,
		}

		log.Printf("Игрок %d атакует сущность %d", c.playerID, targetID)

	case JSONEntityActionUseItem:
		// Обработка использования предмета
		itemID := actionReq.ItemID

		// Проверяем наличие предмета у игрока
		// В реальной игре здесь будет проверка инвентаря

		// Используем предмет
		success = true
		message = "Предмет использован"
		results = map[string]interface{}{
			"item_id": itemID,
			"effect":  "heal",
			"value":   20,
		}

		log.Printf("Игрок %d использует предмет %d", c.playerID, itemID)

	default:
		success = false
		message = "Неизвестный тип действия"
	}

	// Отправляем ответ
	response := JSONEntityActionResponse{
		Success: success,
		Message: message,
		Results: results,
	}

	respData, err := json.Marshal(response)
	if err != nil {
		log.Printf("Ошибка сериализации ответа: %v", err)
		return
	}

	err = c.sendMessage(MsgEntityAction, respData)
	if err != nil {
		log.Printf("Ошибка отправки ответа: %v", err)
	}
}

// handleChat обрабатывает сообщение чата
func (c *TCPConnection) handleChat(data []byte) {
	// Десериализуем запрос
	chatReq, err := DeserializeJSONChat(data)
	if err != nil {
		log.Printf("Ошибка разбора запроса чата: %v", err)
		return
	}

	// Получаем имя игрока из его сущности
	playerName := fmt.Sprintf("Player%d", c.playerID) // В реальной игре получать из данных сущности

	// Создаем сообщение чата
	chatResp := JSONChatResponse{
		Type:       chatReq.Type,
		Message:    chatReq.Message,
		SenderID:   c.playerID,
		SenderName: playerName,
		Timestamp:  time.Now().Unix(),
	}

	// Сериализуем сообщение
	respData, err := json.Marshal(chatResp)
	if err != nil {
		log.Printf("Ошибка сериализации сообщения чата: %v", err)
		return
	}

	// Определяем получателей сообщения в зависимости от типа
	switch chatReq.Type {
	case JSONChatGlobal:
		// Отправляем сообщение всем клиентам
		c.server.broadcastMessage(MsgChat, respData)
		log.Printf("Глобальное сообщение от игрока %d: %s", c.playerID, chatReq.Message)

	case JSONChatLocal:
		// Отправляем сообщение клиентам в радиусе видимости
		// В реальной игре здесь будет логика определения клиентов в радиусе
		c.server.broadcastMessage(MsgChat, respData)
		log.Printf("Локальное сообщение от игрока %d: %s", c.playerID, chatReq.Message)

	case JSONChatPrivate:
		// Отправляем личное сообщение указанному клиенту
		targetID := chatReq.TargetID

		// Находим соединение с целевым игроком
		var targetConn *TCPConnection
		c.server.mu.RLock()
		for _, conn := range c.server.connections {
			if conn.playerID == targetID {
				targetConn = conn
				break
			}
		}
		c.server.mu.RUnlock()

		if targetConn != nil {
			targetConn.sendMessage(MsgChat, respData)
			// Отправляем копию отправителю для подтверждения
			c.sendMessage(MsgChat, respData)
			log.Printf("Личное сообщение от игрока %d игроку %d: %s",
				c.playerID, targetID, chatReq.Message)
		} else {
			// Игрок не найден, отправляем ошибку
			errorResp := JSONChatResponse{
				Type:       JSONChatSystem,
				Message:    "Игрок не найден или не в сети",
				SenderID:   0,
				SenderName: "Система",
				Timestamp:  time.Now().Unix(),
			}

			errorData, _ := json.Marshal(errorResp)
			c.sendMessage(MsgChat, errorData)
		}
	}
}

// handleChunkRequest обрабатывает запрос на получение данных чанка
func (c *TCPConnection) handleChunkRequest(data []byte) {
	// Десериализуем запрос
	var chunkReq ChunkRequestMessage
	err := json.Unmarshal(data, &chunkReq)
	if err != nil {
		log.Printf("Ошибка разбора запроса чанка: %v", err)
		return
	}

	// Координаты запрашиваемого чанка
	chunkX := chunkReq.ChunkX
	chunkY := chunkReq.ChunkY

	log.Printf("Игрок %d запрашивает чанк (%d, %d)", c.playerID, chunkX, chunkY)

	// Получаем чанк из менеджера мира
	chunk := c.server.worldManager.GetChunk(vec.Vec2{X: chunkX, Y: chunkY})
	if chunk == nil {
		log.Printf("Чанк (%d, %d) не найден", chunkX, chunkY)
		// В реальной игре здесь будет генерация чанка, если он не найден
		return
	}

	// Преобразуем чанк в формат JSON
	chunkData := ConvertChunkToJSON(chunk)

	// Сериализуем ответ
	respData, err := json.Marshal(chunkData)
	if err != nil {
		log.Printf("Ошибка сериализации данных чанка: %v", err)
		return
	}

	// Отправляем ответ клиенту
	err = c.sendMessage(MsgChunkData, respData)
	if err != nil {
		log.Printf("Ошибка отправки данных чанка: %v", err)
		return
	}

	log.Printf("Отправлен чанк (%d, %d) игроку %d", chunkX, chunkY, c.playerID)
}

// broadcastMessage отправляет сообщение всем подключенным клиентам
func (s *TCPServer) broadcastMessage(msgType MessageType, data []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections {
		if conn.authorized {
			err := conn.sendMessage(msgType, data)
			if err != nil {
				log.Printf("Ошибка отправки сообщения клиенту %d: %v", conn.id, err)
			}
		}
	}
}
