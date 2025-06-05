package network

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Конфигурация WebSocket
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // В продакшене следует ограничить доступ
	},
}

// Client представляет подключенного клиента
type Client struct {
	conn          *websocket.Conn // WebSocket соединение
	send          chan []byte     // Канал для отправки сообщений
	id            string          // Уникальный идентификатор
	playerID      uint64          // ID игрока в мире
	authenticated bool            // Флаг аутентификации
	lastActivity  time.Time       // Время последней активности
	sequence      uint32          // Счетчик отправленных сообщений
	ackSequence   uint32          // Последнее подтвержденное сообщение от клиента
	receivedAcks  map[uint32]bool // Полученные подтверждения
	mu            sync.Mutex      // Мьютекс для синхронизации
}

// GameServer обрабатывает сетевые соединения и игровую логику
type GameServer struct {
	clients        map[string]*Client // Карта подключенных клиентов
	broadcast      chan []byte        // Канал для широковещательных сообщений
	register       chan *Client       // Канал для регистрации клиентов
	unregister     chan *Client       // Канал для отключения клиентов
	messageHandler MessageHandler     // Обработчик игровых сообщений
	mu             sync.RWMutex       // Мьютекс для синхронизации
	tickRate       time.Duration      // Частота обновления (тиков)
	running        bool               // Флаг работы сервера
}

// MessageHandler определяет интерфейс для обработки игровых сообщений
type MessageHandler interface {
	HandleMessage(client *Client, message *Message) error
	OnClientConnect(client *Client)
	OnClientDisconnect(client *Client)
	Tick(dt float64)
}

// NewGameServer создает новый игровой сервер
func NewGameServer(tickRate time.Duration, handler MessageHandler) *GameServer {
	return &GameServer{
		clients:        make(map[string]*Client),
		broadcast:      make(chan []byte),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		messageHandler: handler,
		tickRate:       tickRate,
		running:        false,
	}
}

// Start запускает сервер
func (s *GameServer) Start() {
	s.running = true

	// Запускаем основной цикл обработки сообщений
	go s.run()

	// Запускаем игровой цикл
	go s.gameLoop()

	log.Println("Game server started")
}

// Stop останавливает сервер
func (s *GameServer) Stop() {
	s.running = false
	log.Println("Game server stopped")
}

// run обрабатывает регистрацию/отключение клиентов и широковещательные сообщения
func (s *GameServer) run() {
	for s.running {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client.id] = client
			s.mu.Unlock()
			s.messageHandler.OnClientConnect(client)
			log.Printf("Client connected: %s", client.id)

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client.id]; ok {
				close(client.send)
				delete(s.clients, client.id)
				s.messageHandler.OnClientDisconnect(client)
				log.Printf("Client disconnected: %s", client.id)
			}
			s.mu.Unlock()

		case message := <-s.broadcast:
			s.mu.RLock()
			for _, client := range s.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(s.clients, client.id)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// gameLoop запускает игровой цикл с фиксированной частотой
func (s *GameServer) gameLoop() {
	ticker := time.NewTicker(s.tickRate)
	defer ticker.Stop()

	lastTick := time.Now()

	for s.running {
		<-ticker.C

		now := time.Now()
		dt := now.Sub(lastTick).Seconds()
		lastTick = now

		// Обновляем игровую логику
		s.messageHandler.Tick(dt)

		// Проверяем таймауты соединений
		s.checkTimeouts()
	}
}

// checkTimeouts проверяет и закрывает неактивные соединения
func (s *GameServer) checkTimeouts() {
	timeout := 60 * time.Second // Таймаут в 60 секунд
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, client := range s.clients {
		if now.Sub(client.lastActivity) > timeout {
			log.Printf("Client timed out: %s", id)
			close(client.send)
			delete(s.clients, id)
			s.messageHandler.OnClientDisconnect(client)
		}
	}
}

// HandleConnection обрабатывает новое WebSocket подключение
func (s *GameServer) HandleConnection(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error upgrading connection:", err)
		return
	}

	clientID := r.RemoteAddr // В реальном коде лучше использовать UUID

	client := &Client{
		conn:          conn,
		send:          make(chan []byte, 256),
		id:            clientID,
		authenticated: false,
		lastActivity:  time.Now(),
		sequence:      0,
		ackSequence:   0,
		receivedAcks:  make(map[uint32]bool),
	}

	s.register <- client

	// Запускаем горутины для чтения и записи
	go s.readPump(client)
	go s.writePump(client)
}

// readPump асинхронно читает сообщения от клиента
func (s *GameServer) readPump(client *Client) {
	defer func() {
		s.unregister <- client
		client.conn.Close()
	}()

	client.conn.SetReadLimit(4096) // Ограничиваем размер сообщения
	client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	client.conn.SetPongHandler(func(string) error {
		client.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error reading message: %v", err)
			}
			break
		}

		// Обновляем время активности
		client.lastActivity = time.Now()

		// Декодируем сообщение
		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Error unmarshaling message: %v", err)
			continue
		}

		// Сохраняем подтверждения
		if msg.Ack > 0 {
			client.mu.Lock()
			client.ackSequence = msg.Ack
			// Обрабатываем битовую маску подтверждений
			if msg.AckBits > 0 {
				for i := uint32(0); i < 32; i++ {
					if (msg.AckBits & (1 << i)) != 0 {
						client.receivedAcks[msg.Ack-i-1] = true
					}
				}
			}
			client.mu.Unlock()
		}

		// Проверяем аутентификацию для всех сообщений, кроме аутентификации
		if !client.authenticated && msg.Type != MsgTypeAuth {
			// Отправляем ошибку аутентификации
			errorMsg, _ := NewMessage(MsgTypeServerMessage, ServerMessage{
				MessageType: "error",
				Content:     "Authentication required",
			})
			errorBytes, _ := json.Marshal(errorMsg)
			client.send <- errorBytes
			continue
		}

		// Устанавливаем ID клиента из сообщения
		msg.ClientID = client.id

		// Обрабатываем сообщение
		if err := s.messageHandler.HandleMessage(client, &msg); err != nil {
			log.Printf("Error handling message: %v", err)
		}
	}
}

// writePump асинхронно отправляет сообщения клиенту
func (s *GameServer) writePump(client *Client) {
	ticker := time.NewTicker(30 * time.Second) // Пинг каждые 30 секунд
	defer func() {
		ticker.Stop()
		client.conn.Close()
	}()

	for {
		select {
		case message, ok := <-client.send:
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				// Канал закрыт
				client.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			// Увеличиваем счетчик сообщений и добавляем его в сообщение
			client.mu.Lock()
			var msg Message
			json.Unmarshal(message, &msg)
			msg.Sequence = client.sequence
			client.sequence++

			// Добавляем подтверждения
			msg.Ack = client.ackSequence

			// Кодируем и отправляем сообщение
			updatedMessage, _ := json.Marshal(msg)
			client.mu.Unlock()

			w, err := client.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(updatedMessage)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			client.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// BroadcastMessage отправляет сообщение всем подключенным клиентам
func (s *GameServer) BroadcastMessage(message *Message) error {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	s.broadcast <- messageBytes
	return nil
}

// SendToClient отправляет сообщение конкретному клиенту
func (s *GameServer) SendToClient(clientID string, message *Message) error {
	s.mu.RLock()
	client, exists := s.clients[clientID]
	s.mu.RUnlock()

	if !exists {
		return nil // Клиент не найден
	}

	messageBytes, err := json.Marshal(message)
	if err != nil {
		return err
	}

	select {
	case client.send <- messageBytes:
	default:
		// Если канал переполнен, закрываем соединение
		s.mu.Lock()
		close(client.send)
		delete(s.clients, clientID)
		s.mu.Unlock()
	}

	return nil
}

// SendToPlayer отправляет сообщение клиенту по ID игрока
func (s *GameServer) SendToPlayer(playerID uint64, message *Message) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, client := range s.clients {
		if client.playerID == playerID {
			messageBytes, err := json.Marshal(message)
			if err != nil {
				return err
			}

			select {
			case client.send <- messageBytes:
			default:
				// Пропускаем отправку, если канал переполнен
			}

			return nil
		}
	}

	return nil // Игрок не найден
}

// GetConnectedClients возвращает количество подключенных клиентов
func (s *GameServer) GetConnectedClients() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// SetClientAuthenticated устанавливает статус аутентификации клиента
func (s *GameServer) SetClientAuthenticated(clientID string, playerID uint64, authenticated bool) {
	s.mu.RLock()
	client, exists := s.clients[clientID]
	s.mu.RUnlock()

	if exists {
		client.mu.Lock()
		client.authenticated = authenticated
		client.playerID = playerID
		client.mu.Unlock()
	}
}
