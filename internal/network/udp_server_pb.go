package network

import (
	"context"
	"encoding/binary"
	"log"
	"net"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/world"
)

// UDPServerPB представляет UDP сервер с поддержкой Protocol Buffers
type UDPServerPB struct {
	conn         *net.UDPConn
	clients      map[uint64]*UDPClientPB
	worldManager *world.WorldManager // Должен реализовать необходимые методы
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	serializer   *protocol.MessageSerializer
	gameHandler  *GameHandlerPB
}

// UDPClientPB представляет клиента, подключенного через UDP
type UDPClientPB struct {
	id       uint64
	addr     *net.UDPAddr
	playerID uint64
	lastSeen time.Time
}

// NewUDPServerPB создает новый UDP сервер с поддержкой Protocol Buffers
func NewUDPServerPB(address string, worldManager *world.WorldManager) (*UDPServerPB, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &UDPServerPB{
		conn:         conn,
		clients:      make(map[uint64]*UDPClientPB),
		worldManager: worldManager,
		ctx:          ctx,
		cancel:       cancel,
		serializer:   protocol.NewMessageSerializer(),
	}, nil
}

// Start запускает UDP сервер
func (s *UDPServerPB) Start() {
	go s.readLoop()
	go s.cleanupLoop()
}

// Stop останавливает UDP сервер
func (s *UDPServerPB) Stop() {
	s.cancel()
	s.conn.Close()
}

// readLoop обрабатывает входящие UDP-пакеты
func (s *UDPServerPB) readLoop() {
	buffer := make([]byte, 2048) // Размер буфера для UDP-пакетов

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			n, addr, err := s.conn.ReadFromUDP(buffer)
			if err != nil {
				logging.LogError("Ошибка чтения UDP: %v", err)
				log.Printf("Ошибка чтения UDP: %v", err)
				continue
			}

			if n < 4 {
				logging.LogWarn("Слишком короткий UDP-пакет от %s: %d байт", addr.String(), n)
				log.Printf("Слишком короткий UDP-пакет: %d байт", n)
				continue
			}

			logging.LogDebug("UDP: Получен пакет от %s размером %d байт", addr.String(), n)

			// Первые 4 байта содержат playerID
			playerID := binary.BigEndian.Uint64(buffer[:8])

			// Обновляем или создаем клиента
			s.mu.Lock()
			client, exists := s.findClientByPlayerID(playerID)
			if !exists {
				client = &UDPClientPB{
					id:       uint64(time.Now().UnixNano()),
					addr:     addr,
					playerID: playerID,
					lastSeen: time.Now(),
				}
				s.clients[client.id] = client
				logging.LogInfo("UDP: Новый клиент %d (playerID: %d) подключен с %s", client.id, playerID, addr.String())
			} else {
				client.lastSeen = time.Now()
				client.addr = addr // Обновляем адрес на случай, если он изменился
			}
			s.mu.Unlock()

			// Обрабатываем пакет асинхронно
			go s.handlePacket(client, buffer[8:n])
		}
	}
}

// findClientByPlayerID находит клиента по ID игрока
func (s *UDPServerPB) findClientByPlayerID(playerID uint64) (*UDPClientPB, bool) {
	for _, client := range s.clients {
		if client.playerID == playerID {
			return client, true
		}
	}
	return nil, false
}

// cleanupLoop удаляет неактивных клиентов
func (s *UDPServerPB) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			for id, client := range s.clients {
				if time.Since(client.lastSeen) > 2*time.Minute {
					delete(s.clients, id)
					logging.LogInfo("UDP клиент %d (playerID: %d) удален из-за таймаута", id, client.playerID)
					log.Printf("UDP клиент %d удален из-за таймаута", id)
				}
			}
			s.mu.Unlock()
		}
	}
}

// handlePacket обрабатывает входящий UDP-пакет
func (s *UDPServerPB) handlePacket(client *UDPClientPB, data []byte) {
	// Логируем получение пакета
	logging.LogMessage(client.addr.String(), "RECEIVED", "UDP", data)

	// Десериализуем сообщение
	msg, err := s.serializer.DeserializeMessage(data)
	if err != nil {
		logging.LogProtocolError(client.addr.String(), err, data)
		log.Printf("Ошибка десериализации UDP-сообщения: %v", err)
		return
	}

	logging.LogDebug("UDP: Получен пакет от игрока %d типа %v (%d байт)", client.playerID, msg.Type, len(data))
	log.Printf("Получен UDP-пакет от игрока %d типа %d (%d байт)", client.playerID, msg.Type, len(data))

	// Обрабатываем в зависимости от типа сообщения
	switch msg.Type {
	case protocol.MessageType_PING:
		logging.LogTrace("UDP: Обработка PING от игрока %d", client.playerID)
		s.handlePing(client, msg)
	case protocol.MessageType_ENTITY_MOVE:
		logging.LogTrace("UDP: Обработка ENTITY_MOVE от игрока %d", client.playerID)
		s.handleEntityMove(client, msg)
	case protocol.MessageType_CHUNK_REQUEST:
		logging.LogDebug("UDP: Обработка CHUNK_REQUEST от игрока %d", client.playerID)
		s.handleChunkRequest(client, msg)
	default:
		// Для остальных типов сообщений логируем без обработки
		logging.LogWarn("UDP: Неподдерживаемый тип сообщения %v от игрока %d", msg.Type, client.playerID)
		log.Printf("Неподдерживаемый тип UDP-сообщения: %d", msg.Type)
	}
}

// handlePing обрабатывает пинг-сообщения
func (s *UDPServerPB) handlePing(client *UDPClientPB, msg *protocol.GameMessage) {
	// Десериализуем пинг
	ping := &protocol.PingMessage{}
	if err := s.serializer.DeserializePayload(msg, ping); err != nil {
		log.Printf("Ошибка десериализации Ping: %v", err)
		return
	}

	// Отправляем понг с тем же временем
	pong := &protocol.PongMessage{
		ClientTimestamp: ping.ClientTimestamp,
		ServerTimestamp: time.Now().UnixNano(),
		ClientCount:     int32(len(s.clients)),
	}

	// Сериализуем и отправляем ответ
	data, err := s.serializer.SerializeMessage(protocol.MessageType_PING, pong)
	if err != nil {
		log.Printf("Ошибка сериализации Pong: %v", err)
		return
	}

	// Создаем заголовок с ID игрока
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, client.playerID)

	// Объединяем заголовок и данные
	packet := append(header, data...)

	// Отправляем данные
	_, err = s.conn.WriteToUDP(packet, client.addr)
	if err != nil {
		log.Printf("Ошибка отправки Pong игроку %d: %v", client.playerID, err)
	}
}

// handleEntityMove обрабатывает сообщения о перемещении сущностей
func (s *UDPServerPB) handleEntityMove(client *UDPClientPB, msg *protocol.GameMessage) {
	// Поскольку обработка движения требует доступа к мировым данным,
	// перенаправляем сообщение в игровой обработчик, если он существует
	if s.gameHandler != nil {
		// Находим ID соединения по playerID
		connID := s.findConnectionIDByPlayerID(client.playerID)
		if connID == "" {
			logging.LogError("UDP: Не найдено TCP-соединение для игрока %d", client.playerID)
			log.Printf("Не найдено TCP-соединение для игрока %d", client.playerID)
			return
		}

		logging.LogDebug("UDP: Перенаправление ENTITY_MOVE от игрока %d в GameHandler", client.playerID)
		// Передаем сообщение в обработчик игры
		s.gameHandler.HandleMessage(connID, msg)
	} else {
		logging.LogError("UDP: GameHandler не инициализирован, пропуск обработки движения для игрока %d", client.playerID)
		log.Printf("GameHandler не инициализирован, пропуск обработки движения")
	}
}

// findConnectionIDByPlayerID находит ID соединения по ID игрока
func (s *UDPServerPB) findConnectionIDByPlayerID(playerID uint64) string {
	if s.gameHandler == nil {
		return ""
	}

	s.gameHandler.mu.RLock()
	defer s.gameHandler.mu.RUnlock()

	for connID, pID := range s.gameHandler.playerEntities {
		if pID == playerID {
			return connID
		}
	}
	return ""
}

// SetGameHandler устанавливает обработчик игры
func (s *UDPServerPB) SetGameHandler(handler *GameHandlerPB) {
	s.gameHandler = handler
}

// SendEntityUpdatesPB отправляет обновления сущностей клиенту через UDP
func (s *UDPServerPB) SendEntityUpdatesPB(playerID uint64, entities []world.EntityData) {
	// Находим клиента
	s.mu.RLock()
	client, exists := s.findClientByPlayerID(playerID)
	s.mu.RUnlock()

	if !exists {
		log.Printf("Не удалось найти UDP-клиента для игрока %d", playerID)
		return
	}

	// Создаем данные сущностей для protobuf
	entityDataList := make([]*protocol.EntityData, 0, len(entities))
	for _, entity := range entities {
		entityData := &protocol.EntityData{
			Id: entity.ID,
			Position: &protocol.Vec2{
				X: int32(entity.Position.X),
				Y: int32(entity.Position.Y),
			},
			Type:   protocol.EntityType_ENTITY_UNKNOWN, // Используем дефолтный тип
			Active: true,
		}
		entityDataList = append(entityDataList, entityData)
	}

	// Создаем protobuf сообщение о перемещении
	moveMessage := &protocol.EntityMoveMessage{
		Entities: entityDataList,
	}

	// Сериализуем сообщение для отправки
	data, err := s.serializer.SerializeMessage(protocol.MessageType_ENTITY_MOVE, moveMessage)
	if err != nil {
		log.Printf("Ошибка сериализации сообщения о перемещении: %v", err)
		return
	}

	// Создаем заголовок с ID игрока
	header := make([]byte, 8)
	binary.BigEndian.PutUint64(header, playerID)

	// Объединяем заголовок и данные
	packet := append(header, data...)

	// Отправляем данные
	_, err = s.conn.WriteToUDP(packet, client.addr)
	if err != nil {
		log.Printf("Ошибка отправки UDP-пакета игроку %d: %v", playerID, err)
	}
}

// handleError обрабатывает сообщения об ошибках
func (s *UDPServerPB) handleError(client *UDPClientPB, msg *protocol.GameMessage) {
	log.Printf("Получено сообщение об ошибке от игрока %d", client.playerID)
	// В простейшем случае просто логируем
}

// handleRegister обрабатывает сообщения регистрации
func (s *UDPServerPB) handleRegister(client *UDPClientPB, msg *protocol.GameMessage) {
	// Перенаправляем сообщение в игровой обработчик, если он существует
	if s.gameHandler != nil {
		// Находим ID соединения по playerID
		connID := s.findConnectionIDByPlayerID(client.playerID)
		if connID == "" {
			log.Printf("Не найдено TCP-соединение для игрока %d", client.playerID)
			return
		}

		// Передаем сообщение в обработчик игры
		s.gameHandler.HandleMessage(connID, msg)
	} else {
		log.Printf("GameHandler не инициализирован, пропуск обработки регистрации")
	}
}

// handleChunkRequest обрабатывает запросы чанков
func (s *UDPServerPB) handleChunkRequest(client *UDPClientPB, msg *protocol.GameMessage) {
	// Перенаправляем сообщение в игровой обработчик, если он существует
	if s.gameHandler != nil {
		// Находим ID соединения по playerID
		connID := s.findConnectionIDByPlayerID(client.playerID)
		if connID == "" {
			log.Printf("Не найдено TCP-соединение для игрока %d", client.playerID)
			return
		}

		// Передаем сообщение в обработчик игры
		s.gameHandler.HandleMessage(connID, msg)
	} else {
		log.Printf("GameHandler не инициализирован, пропуск обработки запроса чанка")
	}
}

// handlePong обрабатывает ответные пинг-сообщения
func (s *UDPServerPB) handlePong(client *UDPClientPB, msg *protocol.GameMessage) {
	// Десериализуем ответное пинг
	pong := &protocol.PongMessage{}
	if err := s.serializer.DeserializePayload(msg, pong); err != nil {
		log.Printf("Ошибка десериализации Pong: %v", err)
		return
	}

	log.Printf("Получено ответное пинг-сообщение от игрока %d", client.playerID)
}
