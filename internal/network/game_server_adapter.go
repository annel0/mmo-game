package network

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
)

// GameServerAdapter интегрирует NetChannel систему с существующим GameServer
type GameServerAdapter struct {
	server     GameServer      // Существующий интерфейс GameServer
	channelMgr *ChannelManager // Менеджер каналов
	factory    ChannelFactory  // Фабрика каналов
	logger     *logging.Logger // Логгер

	// Конфигурация
	config *ServerConfig // Конфигурация сервера

	// Активные соединения
	connections map[string]*ClientConnection
	connMu      sync.RWMutex

	// Обработчик игры
	gameHandler *GameHandlerAdapter

	// Контроль выполнения
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Состояние
	running   bool
	runningMu sync.RWMutex
}

// GameServer интерфейс существующего игрового сервера
type GameServer interface {
	Start(addr string) error
	Stop() error
	IsRunning() bool
	GetPlayerCount() int
	HandleConnection(conn net.Conn) error
}

// ServerConfig конфигурация сервера
type ServerConfig struct {
	// Сетевые настройки
	Address           string
	MaxConnections    int
	ConnectionTimeout time.Duration

	// Настройки каналов
	DefaultChannelType ChannelType
	ChannelConfig      *ChannelConfig

	// Безопасность
	EnableAuth        bool
	MaxMessageSize    int
	RateLimitEnabled  bool
	MaxMessagesPerSec int
}

// ClientConnection представляет соединение с клиентом
type ClientConnection struct {
	ID            string     // Уникальный ID соединения
	PlayerID      uint64     // ID игрока (если авторизован)
	Channel       NetChannel // Сетевой канал
	RemoteAddr    string     // Адрес клиента
	ConnectedAt   time.Time  // Время подключения
	LastActive    time.Time  // Последняя активность
	Authenticated bool       // Флаг авторизации

	// Ограничения
	messageCount  int64     // Счётчик сообщений
	lastRateReset time.Time // Последний сброс счётчика

	mu sync.RWMutex
}

// NewGameServerAdapter создаёт новый адаптер сервера
func NewGameServerAdapter(
	server GameServer,
	factory ChannelFactory,
	config *ServerConfig,
	logger *logging.Logger,
) (*GameServerAdapter, error) {
	if config == nil {
		config = DefaultServerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Создаём менеджер каналов
	channelMgr := NewChannelManager(factory, logger)

	adapter := &GameServerAdapter{
		server:      server,
		channelMgr:  channelMgr,
		factory:     factory,
		logger:      logger,
		config:      config,
		connections: make(map[string]*ClientConnection),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Создаём адаптер обработчика игры
	gameHandler, err := NewGameHandlerAdapter(
		&gameServerWrapper{server: server},
		channelMgr,
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create game handler adapter: %w", err)
	}
	adapter.gameHandler = gameHandler

	return adapter, nil
}

// Start запускает сервер с NetChannel поддержкой
func (gsa *GameServerAdapter) Start() error {
	gsa.runningMu.Lock()
	defer gsa.runningMu.Unlock()

	if gsa.running {
		return fmt.Errorf("server already running")
	}

	// Запускаем основные горутины
	gsa.wg.Add(3)
	go gsa.connectionAcceptor()
	go gsa.connectionMonitor()
	go gsa.metricsReporter()

	gsa.running = true
	gsa.logger.Info("NetChannel server started: address=%s", gsa.config.Address)
	return nil
}

// Stop останавливает сервер
func (gsa *GameServerAdapter) Stop() error {
	gsa.runningMu.Lock()
	defer gsa.runningMu.Unlock()

	if !gsa.running {
		return nil
	}

	// Останавливаем горутины
	gsa.cancel()

	// Закрываем все соединения
	gsa.connMu.Lock()
	for _, conn := range gsa.connections {
		conn.Channel.Close()
	}
	gsa.connections = make(map[string]*ClientConnection)
	gsa.connMu.Unlock()

	// Останавливаем менеджер каналов
	if err := gsa.channelMgr.Shutdown(); err != nil {
		gsa.logger.Error("Failed to shutdown channel manager: %v", err)
	}

	// Останавливаем адаптер обработчика
	if err := gsa.gameHandler.Shutdown(); err != nil {
		gsa.logger.Error("Failed to shutdown game handler: %v", err)
	}

	// Ждём завершения горутин
	gsa.wg.Wait()

	gsa.running = false
	gsa.logger.Info("NetChannel server stopped")
	return nil
}

// IsRunning проверяет состояние сервера
func (gsa *GameServerAdapter) IsRunning() bool {
	gsa.runningMu.RLock()
	defer gsa.runningMu.RUnlock()
	return gsa.running
}

// GetConnectionCount возвращает количество подключений
func (gsa *GameServerAdapter) GetConnectionCount() int {
	gsa.connMu.RLock()
	defer gsa.connMu.RUnlock()
	return len(gsa.connections)
}

// GetMetrics возвращает метрики сервера
func (gsa *GameServerAdapter) GetMetrics() *NetworkMetrics {
	return gsa.gameHandler.GetMetrics()
}

// AcceptConnection принимает новое соединение
func (gsa *GameServerAdapter) AcceptConnection(remoteAddr string) (string, error) {
	// Проверяем лимиты
	if gsa.GetConnectionCount() >= gsa.config.MaxConnections {
		return "", fmt.Errorf("max connections limit reached")
	}

	// Создаём канал
	channelID := gsa.generateConnectionID()
	channel, err := gsa.channelMgr.CreateChannel(channelID, gsa.config.ChannelConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create channel: %w", err)
	}

	// Подключаем канал
	if err := channel.Connect(gsa.ctx, remoteAddr); err != nil {
		gsa.channelMgr.CloseChannel(channelID)
		return "", fmt.Errorf("failed to connect channel: %w", err)
	}

	// Создаём соединение
	conn := &ClientConnection{
		ID:            channelID,
		Channel:       channel,
		RemoteAddr:    remoteAddr,
		ConnectedAt:   time.Now(),
		LastActive:    time.Now(),
		Authenticated: false,
		lastRateReset: time.Now(),
	}

	// Настраиваем обработчики канала
	gsa.setupChannelHandlers(channelID, channel)

	// Сохраняем соединение
	gsa.connMu.Lock()
	gsa.connections[channelID] = conn
	gsa.connMu.Unlock()

	gsa.logger.Info("Client connected: connection_id=%s remote_addr=%s", channelID, remoteAddr)
	return channelID, nil
}

// Приватные методы

func (gsa *GameServerAdapter) connectionAcceptor() {
	defer gsa.wg.Done()

	// Симулируем прием соединений через существующий GameServer
	// В реальной реализации здесь будет интеграция с listener'ом
	
	gsa.logger.Info("Connection acceptor started")
	
	for {
		select {
		case <-gsa.ctx.Done():
			gsa.logger.Info("Connection acceptor stopped")
			return
		default:
			// В реальной реализации здесь будет:
			// 1. Прослушивание listener'а
			// 2. Принятие новых соединений
			// 3. Создание каналов через factory
			// 4. Регистрация в channelMgr
			
			time.Sleep(100 * time.Millisecond) // Предотвращаем busy loop
		}
	}
}

func (gsa *GameServerAdapter) connectionMonitor() {
	defer gsa.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gsa.cleanupStaleConnections()
		case <-gsa.ctx.Done():
			return
		}
	}
}

func (gsa *GameServerAdapter) metricsReporter() {
	defer gsa.wg.Done()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gsa.reportMetrics()
		case <-gsa.ctx.Done():
			return
		}
	}
}

func (gsa *GameServerAdapter) setupChannelHandlers(channelID string, channel NetChannel) {
	// Обработчик сообщений
	channel.OnMessage(func(msg *protocol.NetGameMessage) {
		gsa.handleChannelMessage(channelID, msg)
	})

	// Обработчик отключения
	channel.OnDisconnect(func(err error) {
		gsa.handleChannelDisconnect(channelID, err)
	})

	// Обработчик ошибок
	channel.OnError(func(err error) {
		gsa.logger.Error("Channel error: connection_id=%s error=%v", channelID, err)
	})
}

func (gsa *GameServerAdapter) handleChannelMessage(channelID string, msg *protocol.NetGameMessage) {
	// Обновляем время активности
	gsa.updateConnectionActivity(channelID)

	// Проверяем rate limiting
	if !gsa.checkRateLimit(channelID) {
		gsa.logger.Warn("Rate limit exceeded: connection_id=%s", channelID)
		return
	}

	// Передаём в игровой обработчик
	if err := gsa.gameHandler.HandleIncomingMessage(channelID, msg); err != nil {
		gsa.logger.Error("Failed to handle message: connection_id=%s error=%v", channelID, err)
	}
}

func (gsa *GameServerAdapter) handleChannelDisconnect(channelID string, err error) {
	gsa.connMu.Lock()
	conn, exists := gsa.connections[channelID]
	if exists {
		delete(gsa.connections, channelID)
	}
	gsa.connMu.Unlock()

	if exists && conn.Authenticated {
		gsa.gameHandler.UnregisterPlayer(conn.PlayerID)
	}

	gsa.channelMgr.CloseChannel(channelID)

	if err != nil {
		gsa.logger.Warn("Client disconnected with error: connection_id=%s error=%v", channelID, err)
	} else {
		gsa.logger.Info("Client disconnected: connection_id=%s", channelID)
	}
}

func (gsa *GameServerAdapter) updateConnectionActivity(channelID string) {
	gsa.connMu.Lock()
	defer gsa.connMu.Unlock()

	if conn, exists := gsa.connections[channelID]; exists {
		conn.mu.Lock()
		conn.LastActive = time.Now()
		conn.mu.Unlock()
	}
}

func (gsa *GameServerAdapter) checkRateLimit(channelID string) bool {
	if !gsa.config.RateLimitEnabled {
		return true
	}

	gsa.connMu.RLock()
	conn, exists := gsa.connections[channelID]
	gsa.connMu.RUnlock()

	if !exists {
		return false
	}

	conn.mu.Lock()
	defer conn.mu.Unlock()

	now := time.Now()
	// Сбрасываем счётчик каждую секунду
	if now.Sub(conn.lastRateReset) >= time.Second {
		conn.messageCount = 0
		conn.lastRateReset = now
	}

	conn.messageCount++
	return conn.messageCount <= int64(gsa.config.MaxMessagesPerSec)
}

func (gsa *GameServerAdapter) cleanupStaleConnections() {
	timeout := gsa.config.ConnectionTimeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	now := time.Now()
	var staleConnections []string

	gsa.connMu.RLock()
	for id, conn := range gsa.connections {
		conn.mu.RLock()
		if now.Sub(conn.LastActive) > timeout {
			staleConnections = append(staleConnections, id)
		}
		conn.mu.RUnlock()
	}
	gsa.connMu.RUnlock()

	// Закрываем устаревшие соединения
	for _, connectionID := range staleConnections {
		gsa.handleChannelDisconnect(connectionID, fmt.Errorf("connection timeout"))
	}

	if len(staleConnections) > 0 {
		gsa.logger.Info("Cleaned up stale connections: count=%d", len(staleConnections))
	}
}

func (gsa *GameServerAdapter) reportMetrics() {
	metrics := gsa.GetMetrics()
	gsa.logger.Info("Server metrics: connections=%d total_messages=%d total_bytes=%d",
		gsa.GetConnectionCount(),
		metrics.TotalMessages,
		metrics.TotalBytes,
	)
}

func (gsa *GameServerAdapter) generateConnectionID() string {
	return fmt.Sprintf("conn_%d", time.Now().UnixNano())
}

// DefaultServerConfig возвращает конфигурацию сервера по умолчанию
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Address:            ":8080",
		MaxConnections:     1000,
		ConnectionTimeout:  5 * time.Minute,
		DefaultChannelType: ChannelKCP,
		ChannelConfig:      DefaultChannelConfig(ChannelKCP),
		EnableAuth:         true,
		MaxMessageSize:     65536,
		RateLimitEnabled:   true,
		MaxMessagesPerSec:  100,
	}
}

// gameServerWrapper оборачивает существующий GameServer в интерфейс GameHandler
type gameServerWrapper struct {
	server GameServer
}

func (gsw *gameServerWrapper) HandleMessage(playerID uint64, msg *protocol.GameMessage) error {
	// Адаптируем к существующему API GameServer
	// В реальной реализации здесь будет преобразование протокола
	logging.Debug("GameServerWrapper: handling message for player %d, type %v", playerID, msg.Type)
	return nil
}

func (gsw *gameServerWrapper) OnPlayerConnect(playerID uint64) error {
	// Уведомляем GameServer о подключении игрока
	logging.Info("GameServerWrapper: player %d connected", playerID)
	// В реальной реализации здесь будет вызов методов GameServer
	return nil
}

func (gsw *gameServerWrapper) OnPlayerDisconnect(playerID uint64) error {
	// Уведомляем GameServer об отключении игрока
	logging.Info("GameServerWrapper: player %d disconnected", playerID)
	// В реальной реализации здесь будет вызов методов GameServer
	return nil
}

func (gsw *gameServerWrapper) SendToPlayer(playerID uint64, msg *protocol.GameMessage) error {
	// Отправляем сообщение конкретному игроку через GameServer
	logging.Debug("GameServerWrapper: sending message to player %d, type %v", playerID, msg.Type)
	// В реальной реализации здесь будет поиск соединения и отправка
	return nil
}

func (gsw *gameServerWrapper) BroadcastMessage(msg *protocol.GameMessage) error {
	// Отправляем сообщение всем подключенным игрокам
	logging.Debug("GameServerWrapper: broadcasting message type %v", msg.Type)
	// В реальной реализации здесь будет перебор всех соединений
	return nil
}
