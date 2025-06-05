package network

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"

	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/world"
	"google.golang.org/protobuf/proto"
)

// TCPServerPB представляет TCP сервер с поддержкой Protocol Buffers
type TCPServerPB struct {
	listener     net.Listener
	connections  map[string]*TCPConnectionPB
	worldManager *world.WorldManager
	gameHandler  *GameHandlerPB
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc
	serializer   *protocol.MessageSerializer
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
		listener:     listener,
		connections:  make(map[string]*TCPConnectionPB),
		worldManager: worldManager,
		ctx:          ctx,
		cancel:       cancel,
		serializer:   protocol.NewMessageSerializer(),
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
				log.Printf("Ошибка принятия соединения: %v", err)
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
	s.mu.Unlock()

	// Запускаем обработку сообщений
	go connection.readLoop()

	log.Printf("Новое TCP соединение: %s", connID)
}

// removeConnection удаляет соединение из списка
func (s *TCPServerPB) removeConnection(connID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.connections, connID)
	log.Printf("TCP соединение закрыто: %s", connID)
}

// broadcastMessage отправляет сообщение всем подключенным клиентам
func (s *TCPServerPB) broadcastMessage(msgType protocol.MsgType, payload proto.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, conn := range s.connections {
		conn.sendMessage(msgType, payload)
	}
}

// sendToPlayer отправляет сообщение конкретному игроку
func (s *TCPServerPB) sendToPlayer(playerID uint64, msgType protocol.MsgType, payload proto.Message) {
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
func (s *TCPServerPB) sendToClient(connID string, msgType protocol.MsgType, payload proto.Message) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Находим соединение по ID
	if conn, exists := s.connections[connID]; exists {
		conn.sendMessage(msgType, payload)
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
	// Десериализуем сообщение
	msg, err := c.serializer.DeserializeMessage(data)
	if err != nil {
		log.Printf("Ошибка десериализации сообщения: %v", err)
		return
	}

	// Проверяем, инициализирован ли обработчик игры
	if c.server.gameHandler == nil {
		log.Printf("Ошибка: gameHandler не инициализирован")
		return
	}

	// Аутентификация для сообщений, кроме MsgAuth
	if msg.Type != protocol.MsgAuth && c.playerID == 0 {
		log.Printf("Отклонено неаутентифицированное сообщение типа %d от %s",
			msg.Type, c.id)

		// Отправляем сообщение об ошибке аутентификации
		errorResponse := &protocol.AuthResponse{
			Success: false,
			Message: "Требуется аутентификация",
		}
		c.sendMessage(protocol.MsgAuthResponse, errorResponse)
		return
	}

	// Передаем сообщение в игровой обработчик
	c.server.gameHandler.HandleMessage(c.id, msg)

	// Устанавливаем playerID для соединения если это сообщение авторизации
	if msg.Type == protocol.MsgAuth {
		// После обработки авторизации получаем ID игрока из карты соединений
		c.server.mu.RLock()
		for id, conn := range c.server.connections {
			if id == c.id && conn.playerID != 0 {
				c.playerID = conn.playerID
				break
			}
		}
		c.server.mu.RUnlock()
	}
}

// sendMessage отправляет сообщение клиенту
func (c *TCPConnectionPB) sendMessage(msgType protocol.MsgType, payload proto.Message) {
	// Сериализуем сообщение
	data, err := c.serializer.SerializeMessage(msgType, payload)
	if err != nil {
		log.Printf("Ошибка сериализации сообщения: %v", err)
		return
	}

	// Отправляем размер сообщения (4 байта)
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))

	// Отправляем сообщение
	c.conn.Write(header)
	c.conn.Write(data)
}

// close закрывает соединение
func (c *TCPConnectionPB) close() {
	c.cancel()
	c.conn.Close()
}
