package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/xtaci/kcp-go/v5"
)

// ChannelServer представляет сервер каналов
type ChannelServer struct {
	addr     string
	listener net.Listener
	config   *ChannelConfig

	// Клиенты
	clients   map[string]*ClientChannel
	clientsMu sync.RWMutex

	// Обработчики
	onConnect    func(clientID string, channel NetChannel)
	onDisconnect func(clientID string)
	onMessage    func(clientID string, msg *protocol.GameMessage)

	// Конвертер сообщений
	converter *MessageConverter

	// Состояние
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Логгер
	logger *logging.Logger
}

// ClientChannel хранит информацию о клиентском канале
type ClientChannel struct {
	ID        string
	Channel   NetChannel
	Connected bool
	LastSeen  time.Time
}

// NewChannelServer создаёт новый сервер
func NewChannelServer(addr string, config *ChannelConfig) *ChannelServer {
	logger := logging.GetNetworkLogger()

	converter, err := NewMessageConverter()
	if err != nil {
		logger.Error("Failed to create message converter: %v", err)
		return nil
	}

	return &ChannelServer{
		addr:      addr,
		config:    config,
		clients:   make(map[string]*ClientChannel),
		converter: converter,
		logger:    logger,
	}
}

// SetHandlers устанавливает обработчики событий
func (cs *ChannelServer) SetHandlers(
	onConnect func(string, NetChannel),
	onDisconnect func(string),
	onMessage func(string, *protocol.GameMessage),
) {
	cs.onConnect = onConnect
	cs.onDisconnect = onDisconnect
	cs.onMessage = onMessage
}

// Start запускает сервер
func (cs *ChannelServer) Start() error {
	listener, err := kcp.ListenWithOptions(cs.addr, nil, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", cs.addr, err)
	}

	cs.listener = listener
	cs.ctx, cs.cancel = context.WithCancel(context.Background())

	// Запускаем горутины
	cs.wg.Add(2)
	go cs.acceptLoop()
	go cs.timeoutLoop()

	cs.logger.Info("🚀 Channel server started on %s", cs.addr)
	return nil
}

// Stop останавливает сервер
func (cs *ChannelServer) Stop() error {
	if cs.cancel != nil {
		cs.cancel()
	}

	if cs.listener != nil {
		cs.listener.Close()
	}

	// Ждем завершения горутин
	cs.wg.Wait()

	// Отключаем всех клиентов
	cs.clientsMu.Lock()
	for id, client := range cs.clients {
		client.Channel.Close()
		delete(cs.clients, id)
	}
	cs.clientsMu.Unlock()

	// Закрываем конвертер
	if err := cs.converter.Close(); err != nil {
		cs.logger.Error("Failed to close converter: %v", err)
	}

	cs.logger.Info("🛑 Channel server stopped")
	return nil
}

// acceptLoop принимает входящие соединения
func (cs *ChannelServer) acceptLoop() {
	defer cs.wg.Done()

	for {
		select {
		case <-cs.ctx.Done():
			return
		default:
		}

		conn, err := cs.listener.Accept()
		if err != nil {
			select {
			case <-cs.ctx.Done():
				return // Сервер останавливается
			default:
				cs.logger.Error("Failed to accept connection: %v", err)
				continue
			}
		}

		cs.wg.Add(1)
		go cs.handleConnection(conn)
	}
}

// handleConnection обрабатывает новое соединение
func (cs *ChannelServer) handleConnection(conn net.Conn) {
	defer cs.wg.Done()

	// Проверяем тип соединения (KCP)
	kcpConn, ok := conn.(*kcp.UDPSession)
	if !ok {
		cs.logger.Error("Invalid connection type")
		conn.Close()
		return
	}

	// Создаём канал и подключаем KCP соединение
	logger := logging.GetNetworkLogger()
	channel := NewKCPChannelFromConn(kcpConn, cs.config, logger)

	// Генерируем ID клиента
	clientID := fmt.Sprintf("client-%s-%d", conn.RemoteAddr(), time.Now().UnixNano())

	// Сохраняем клиента
	client := &ClientChannel{
		ID:        clientID,
		Channel:   channel,
		Connected: true,
		LastSeen:  time.Now(),
	}

	cs.clientsMu.Lock()
	cs.clients[clientID] = client
	cs.clientsMu.Unlock()

	// Вызываем обработчик подключения
	if cs.onConnect != nil {
		cs.onConnect(clientID, channel)
	}

	// Читаем сообщения
	cs.readLoop(client)

	// Отключаем клиента
	cs.disconnectClient(clientID)
}

// readLoop читает сообщения от клиента
func (cs *ChannelServer) readLoop(client *ClientChannel) {
	for {
		select {
		case <-cs.ctx.Done():
			return
		default:
		}

		// Читаем сообщение с таймаутом (временная заглушка)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		netMsg, err := client.Channel.Receive(ctx)
		cancel()

		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue // Таймаут чтения, продолжаем
			}
			// Ошибка чтения, отключаем клиента
			return
		}

		// Конвертируем NetGameMessage в GameMessage
		msg, convertErr := cs.converter.NetToGame(netMsg)
		if convertErr != nil {
			cs.logger.Error("Failed to convert message: %v", convertErr)
			continue
		}

		// Обновляем время последней активности
		cs.clientsMu.Lock()
		client.LastSeen = time.Now()
		cs.clientsMu.Unlock()

		// Вызываем обработчик сообщения
		if cs.onMessage != nil {
			cs.onMessage(client.ID, msg)
		}
	}
}

// timeoutLoop проверяет таймауты клиентов
func (cs *ChannelServer) timeoutLoop() {
	defer cs.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cs.ctx.Done():
			return
		case <-ticker.C:
			cs.checkTimeouts()
		}
	}
}

// checkTimeouts проверяет и отключает неактивных клиентов
func (cs *ChannelServer) checkTimeouts() {
	timeout := 30 * time.Second
	now := time.Now()

	cs.clientsMu.Lock()
	defer cs.clientsMu.Unlock()

	for id, client := range cs.clients {
		if now.Sub(client.LastSeen) > timeout {
			cs.logger.Warn("⏱️ Client %s timed out", id)
			cs.wg.Add(1)
			go cs.disconnectClient(id)
		}
	}
}

// disconnectClient отключает клиента
func (cs *ChannelServer) disconnectClient(clientID string) {
	defer cs.wg.Done()

	cs.clientsMu.Lock()
	client, exists := cs.clients[clientID]
	if !exists {
		cs.clientsMu.Unlock()
		return
	}
	delete(cs.clients, clientID)
	cs.clientsMu.Unlock()

	// Закрываем канал
	client.Channel.Close()

	// Вызываем обработчик отключения
	if cs.onDisconnect != nil {
		cs.onDisconnect(clientID)
	}

	cs.logger.Info("👋 Client %s disconnected", clientID)
}

// SendToClient отправляет сообщение клиенту
func (cs *ChannelServer) SendToClient(clientID string, msg *protocol.GameMessage, flags ChannelFlags) error {
	cs.clientsMu.RLock()
	client, exists := cs.clients[clientID]
	cs.clientsMu.RUnlock()

	if !exists {
		return errors.New("client not found")
	}

	// Конвертируем GameMessage в NetGameMessage
	netMsg, err := cs.converter.GameToNet(msg)
	if err != nil {
		return fmt.Errorf("failed to convert message: %w", err)
	}

	// Получаем опции отправки
	opts := cs.converter.GetSendOptions(msg)
	_ = flags // TODO: интегрировать flags с SendOptions

	return client.Channel.Send(context.Background(), netMsg, opts)
}

// Broadcast отправляет сообщение всем клиентам
func (cs *ChannelServer) Broadcast(msg *protocol.GameMessage, flags ChannelFlags) {
	cs.clientsMu.RLock()
	clients := make([]*ClientChannel, 0, len(cs.clients))
	for _, client := range cs.clients {
		clients = append(clients, client)
	}
	cs.clientsMu.RUnlock()

	// Отправляем сообщения параллельно
	var wg sync.WaitGroup
	for _, client := range clients {
		wg.Add(1)
		go func(c *ClientChannel) {
			defer wg.Done()
			// Конвертируем GameMessage в NetGameMessage
			netMsg, err := cs.converter.GameToNet(msg)
			if err != nil {
				cs.logger.Error("Failed to convert message for %s: %v", c.ID, err)
				return
			}

			// Получаем опции отправки
			opts := cs.converter.GetSendOptions(msg)
			_ = flags // TODO: интегрировать flags с SendOptions

			if err := c.Channel.Send(context.Background(), netMsg, opts); err != nil {
				cs.logger.Error("Failed to send to %s: %v", c.ID, err)
			}
		}(client)
	}
	wg.Wait()
}

// GetClientCount возвращает количество подключенных клиентов
func (cs *ChannelServer) GetClientCount() int {
	cs.clientsMu.RLock()
	defer cs.clientsMu.RUnlock()
	return len(cs.clients)
}
