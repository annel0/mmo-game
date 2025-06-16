package network

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/world"
	"google.golang.org/protobuf/proto"
)

const (
	// MaxConnections - максимальное количество одновременных подключений
	MaxConnections = 1000
	// MaxConnectionsPerIP - максимальное количество подключений с одного IP
	MaxConnectionsPerIP = 5
	// ConnectionTimeout - таймаут для неактивных подключений
	ConnectionTimeout = 5 * time.Minute
)

// TCPServerPB представляет TCP сервер с поддержкой Protocol Buffers
type TCPServerPB struct {
	listener         net.Listener
	connections      map[string]*TCPConnectionPB
	connectionsByIP  map[string]int32 // IP -> count of connections
	totalConnections int32
	worldManager     *world.WorldManager
	gameHandler      *GameHandlerPB
	mu               sync.RWMutex
	ctx              context.Context
	cancel           context.CancelFunc
	serializer       *protocol.MessageSerializer
}

// TCPConnectionPB представляет подключение клиента по TCP
type TCPConnectionPB struct {
	id         string
	conn       net.Conn
	server     *TCPServerPB
	playerID   uint64
	ctx        context.Context
	cancel     context.CancelFunc
	serializer *protocol.MessageSerializer
}

// NewTCPServerPB создает новый TCP сервер с поддержкой Protocol Buffers
func NewTCPServerPB(address string, worldManager *world.WorldManager) (*TCPServerPB, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &TCPServerPB{
		listener:        listener,
		connections:     make(map[string]*TCPConnectionPB),
		connectionsByIP: make(map[string]int32),
		worldManager:    worldManager,
		ctx:             ctx,
		cancel:          cancel,
		serializer:      protocol.NewMessageSerializer(),
	}, nil
}

// Start запускает TCP сервер
func (s *TCPServerPB) Start() {
	go s.acceptLoop()
}

// Stop останавливает TCP сервер
func (s *TCPServerPB) Stop() {
	s.cancel()
	s.mu.Lock()
	defer s.mu.Unlock()

	// Закрываем все соединения
	for _, conn := range s.connections {
		conn.close()
	}

	// Закрываем слушатель
	s.listener.Close()
}

// SetGameHandler устанавливает обработчик игры
func (s *TCPServerPB) SetGameHandler(handler *GameHandlerPB) {
	s.gameHandler = handler
}

// acceptLoop принимает входящие соединения
func (s *TCPServerPB) acceptLoop() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			conn, err := s.listener.Accept()
			if err != nil {
				logging.Error("Ошибка принятия соединения: %v", err)
				log.Printf("Ошибка принятия соединения: %v", err)
				continue
			}

			// Проверяем лимиты перед принятием соединения
			if !s.canAcceptConnection(conn) {
				logging.Warn("Соединение отклонено из-за лимитов: %s", conn.RemoteAddr().String())
				conn.Close()
				continue
			}

			s.handleConnection(conn)
		}
	}
}

// handleConnection обрабатывает новое соединение
func (s *TCPServerPB) handleConnection(conn net.Conn) {
	// Создаем контекст для соединения
	ctx, cancel := context.WithCancel(s.ctx)

	// Создаем объект соединения
	connID := conn.RemoteAddr().String()
	connection := &TCPConnectionPB{
		id:         connID,
		conn:       conn,
		server:     s,
		ctx:        ctx,
		cancel:     cancel,
		serializer: s.serializer,
	}

	// Добавляем соединение в карту
	s.mu.Lock()
	s.connections[connID] = connection

	// Обновляем счетчики
	ip := getIPFromAddr(conn.RemoteAddr())
	s.connectionsByIP[ip]++
	atomic.AddInt32(&s.totalConnections, 1)
	s.mu.Unlock()

	// Запускаем обработку сообщений
	go connection.readLoop()

	totalConns := atomic.LoadInt32(&s.totalConnections)
	logging.Info("Новое TCP соединение: %s (всего: %d)", connID, totalConns)
	log.Printf("Новое TCP соединение: %s (всего: %d)", connID, totalConns)
}

// removeConnection удаляет соединение из списка
func (s *TCPServerPB) removeConnection(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if conn, exists := s.connections[connID]; exists {
		// Обновляем счетчики
		ip := getIPFromAddr(conn.conn.RemoteAddr())
		if count := s.connectionsByIP[ip]; count > 0 {
			s.connectionsByIP[ip]--
			if s.connectionsByIP[ip] == 0 {
				delete(s.connectionsByIP, ip)
			}
		}
		atomic.AddInt32(&s.totalConnections, -1)

		delete(s.connections, connID)
		remaining := atomic.LoadInt32(&s.totalConnections)
		logging.Info("TCP соединение закрыто: %s (осталось: %d)", connID, remaining)
		log.Printf("TCP соединение закрыто: %s (осталось: %d)", connID, remaining)

		// Оповещаем игровой обработчик о разрыве соединения, чтобы очистить карты playerEntities и т.д.
		// Вызываем вне блокировки s.mu, чтобы избежать возможных дедлоков.
		go func(handler *GameHandlerPB, id string) {
			if handler != nil {
				handler.OnClientDisconnect(id)
			}
		}(s.gameHandler, connID)
	}
}

// broadcastMessage отправляет сообщение всем подключенным клиентам
func (s *TCPServerPB) broadcastMessage(msgType protocol.MessageType, payload proto.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections {
		conn.sendMessage(msgType, payload)
	}
}

// sendToPlayer отправляет сообщение конкретному игроку
func (s *TCPServerPB) sendToPlayer(playerID uint64, msgType protocol.MessageType, payload proto.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections {
		if conn.playerID == playerID {
			conn.sendMessage(msgType, payload)
			return
		}
	}
}

// sendToClient отправляет сообщение конкретному клиенту по ID соединения
func (s *TCPServerPB) sendToClient(connID string, msgType protocol.MessageType, payload proto.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Находим соединение по ID
	if conn, exists := s.connections[connID]; exists {

		conn.sendMessage(msgType, payload)
	} else {
		log.Printf("❌ TCP: Соединение %s не найдено!", connID)
	}
}

// readLoop обрабатывает входящие сообщения от клиента
func (c *TCPConnectionPB) readLoop() {
	defer func() {
		c.close()
		c.server.removeConnection(c.id)
	}()

	headerBuffer := make([]byte, 4) // 4 байта для размера сообщения

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Читаем размер сообщения (4 байта)
			_, err := io.ReadFull(c.conn, headerBuffer)
			if err != nil {
				if err != io.EOF {
					log.Printf("Ошибка чтения заголовка: %v", err)
				}
				return
			}

			// Получаем размер сообщения
			messageSize := binary.BigEndian.Uint32(headerBuffer)
			if messageSize > 10*1024*1024 { // Ограничиваем размер 10MB
				log.Printf("Слишком большое сообщение: %d байт", messageSize)
				return
			}

			// Читаем тело сообщения
			messageBuffer := make([]byte, messageSize)
			_, err = io.ReadFull(c.conn, messageBuffer)
			if err != nil {
				log.Printf("Ошибка чтения тела сообщения: %v", err)
				return
			}

			// Обрабатываем сообщение
			go c.handleMessage(messageBuffer)
		}
	}
}

// handleMessage обрабатывает полученное сообщение
func (c *TCPConnectionPB) handleMessage(data []byte) {
	// Логируем получение сообщения
	logging.LogMessage("RECEIVED", protocol.MessageType_AUTH, data, c.id)

	// Десериализуем сообщение
	msg, err := c.serializer.DeserializeMessage(data)
	if err != nil {
		logging.LogProtocolError("TCP Deserialization", err, data)
		log.Printf("Ошибка десериализации сообщения: %v", err)
		return
	}

	logging.Debug("TCP: Обработка сообщения %v от %s", msg.Type, c.id)

	// Проверяем, инициализирован ли обработчик игры
	if c.server.gameHandler == nil {
		logging.Error("gameHandler не инициализирован для %s", c.id)
		log.Printf("Ошибка: gameHandler не инициализирован")
		return
	}

	// Проверяем, что соединение авторизовано и токен валиден
	if msg.Type != protocol.MessageType_AUTH {
		if !c.server.gameHandler.IsSessionValid(c.id) {
			logging.Debug("Недействительная сессия для %s, тип сообщения: %v", c.id, msg.Type)
			log.Printf("Недействительная или отсутствующая сессия для %s", c.id)
			errorResponse := &protocol.AuthResponse{Success: false, Message: "invalid session"}
			c.sendMessage(protocol.MessageType_AUTH_RESPONSE, errorResponse)
			return
		}
	}

	// Логируем специальные типы сообщений
	switch msg.Type {
	case protocol.MessageType_AUTH:
		logging.Info("TCP: Запрос аутентификации от %s", c.id)
	case protocol.MessageType_CHUNK_REQUEST:
		logging.Debug("TCP: Запрос чанка от %s", c.id)
	}

	// Передаем сообщение в игровой обработчик
	c.server.gameHandler.HandleMessage(c.id, msg)

	// Устанавливаем playerID для соединения если это сообщение авторизации
	if msg.Type == protocol.MessageType_AUTH {
		// После обработки авторизации получаем ID игрока из карты соединений
		c.server.mu.RLock()
		for id, conn := range c.server.connections {
			if id == c.id && conn.playerID != 0 {
				c.playerID = conn.playerID
				logging.Info("TCP: Установлен playerID %d для соединения %s", conn.playerID, c.id)
				break
			}
		}
		c.server.mu.RUnlock()
	}
}

// sendMessage отправляет сообщение клиенту
func (c *TCPConnectionPB) sendMessage(msgType protocol.MessageType, payload proto.Message) {
	// Сериализуем сообщение
	data, err := c.serializer.SerializeMessage(msgType, payload)
	if err != nil {
		logging.Error("❌ TCP: Ошибка сериализации сообщения %v для %s: %v", msgType, c.id, err)
		log.Printf("❌ TCP: Ошибка сериализации сообщения: %v", err)
		return
	}

	// Логируем отправку сообщения
	logging.LogMessage("SENDING", msgType, data, c.id)

	// Отправляем размер сообщения (4 байта)
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	// Отправляем сообщение
	_, err1 := c.conn.Write(header)
	_, err2 := c.conn.Write(data)

	if err1 != nil || err2 != nil {
		logging.Error("❌ TCP: Ошибка отправки сообщения %v клиенту %s: %v, %v", msgType, c.id, err1, err2)
		log.Printf("❌ TCP: Ошибка отправки сообщения клиенту %s: %v, %v", c.id, err1, err2)
		return
	}

	logging.Debug("✅ TCP: Сообщение %v отправлено клиенту %s", msgType, c.id)
}

// close закрывает соединение
func (c *TCPConnectionPB) close() {
	c.cancel()
	c.conn.Close()
}

// canAcceptConnection проверяет, можем ли мы принять новое соединение
func (s *TCPServerPB) canAcceptConnection(conn net.Conn) bool {
	// Проверяем общий лимит подключений
	if atomic.LoadInt32(&s.totalConnections) >= MaxConnections {
		log.Printf("Превышен лимит подключений: %d", MaxConnections)
		return false
	}

	// Проверяем лимит подключений с одного IP
	ip := getIPFromAddr(conn.RemoteAddr())
	s.mu.RLock()
	count := s.connectionsByIP[ip]
	s.mu.RUnlock()

	if count >= MaxConnectionsPerIP {
		log.Printf("Превышен лимит подключений с IP %s: %d", ip, MaxConnectionsPerIP)
		return false
	}

	return true
}

// getIPFromAddr извлекает IP адрес из net.Addr
func getIPFromAddr(addr net.Addr) string {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return tcpAddr.IP.String()
	}
	// Fallback для других типов адресов
	host, _, _ := net.SplitHostPort(addr.String())
	return host
}
