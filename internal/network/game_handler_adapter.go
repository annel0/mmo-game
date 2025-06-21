package network

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/world/entity"
)

// GameHandlerAdapter адаптирует существующий GameHandler для работы с NetChannel
type GameHandlerAdapter struct {
	handler    GameHandler                 // Существующий интерфейс GameHandler
	channelMgr *ChannelManager             // Менеджер каналов
	serializer *protocol.MessageSerializer // Сериализатор сообщений
	logger     *logging.Logger             // Логгер
	metrics    *NetworkMetrics             // Метрики

	// Карта соединений: player_id -> channel_id
	connections   map[uint64]string
	connectionsMu sync.RWMutex

	// Контроль выполнения
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Игроки
	players   map[uint64]*entity.Player
	playersMu sync.RWMutex

	// Карта clientID -> playerID
	clientToPlayer map[string]uint64
	clientMapMu    sync.RWMutex

	// Статистика
	messagesReceived atomic.Uint64
	messagesSent     atomic.Uint64
	errors           atomic.Uint64
}

// GameHandler интерфейс существующего обработчика игры
type GameHandler interface {
	HandleMessage(playerID uint64, msg *protocol.GameMessage) error
	OnPlayerConnect(playerID uint64) error
	OnPlayerDisconnect(playerID uint64) error
	SendToPlayer(playerID uint64, msg *protocol.GameMessage) error
	BroadcastMessage(msg *protocol.GameMessage) error
}

// NewGameHandlerAdapter создаёт новый адаптер
func NewGameHandlerAdapter(
	handler GameHandler,
	channelMgr *ChannelManager,
	logger *logging.Logger,
) (*GameHandlerAdapter, error) {
	serializer := createMessageSerializer()

	ctx, cancel := context.WithCancel(context.Background())

	adapter := &GameHandlerAdapter{
		handler:        handler,
		channelMgr:     channelMgr,
		serializer:     serializer,
		logger:         logger,
		metrics:        NewNetworkMetrics(),
		connections:    make(map[uint64]string),
		ctx:            ctx,
		cancel:         cancel,
		players:        make(map[uint64]*entity.Player),
		clientToPlayer: make(map[string]uint64),
	}

	// Запускаем метрики
	adapter.wg.Add(1)
	go adapter.metricsLoop()

	return adapter, nil
}

// RegisterPlayer регистрирует игрока с каналом
func (ga *GameHandlerAdapter) RegisterPlayer(playerID uint64, channelID string) error {
	ga.connectionsMu.Lock()
	defer ga.connectionsMu.Unlock()

	// Проверяем, что канал существует
	channel, exists := ga.channelMgr.GetChannel(channelID)
	if !exists {
		return fmt.Errorf("channel %s not found", channelID)
	}

	if !channel.IsConnected() {
		return fmt.Errorf("channel %s not connected", channelID)
	}

	// Сохраняем связь
	ga.connections[playerID] = channelID

	// Уведомляем игровой обработчик
	if err := ga.handler.OnPlayerConnect(playerID); err != nil {
		delete(ga.connections, playerID)
		return fmt.Errorf("handler rejected player connection: %w", err)
	}

	ga.logger.Info("Player registered: player_id=%d channel_id=%s", playerID, channelID)
	return nil
}

// UnregisterPlayer отключает игрока
func (ga *GameHandlerAdapter) UnregisterPlayer(playerID uint64) error {
	ga.connectionsMu.Lock()
	channelID, exists := ga.connections[playerID]
	if exists {
		delete(ga.connections, playerID)
	}
	ga.connectionsMu.Unlock()

	if !exists {
		return fmt.Errorf("player %d not registered", playerID)
	}

	// Уведомляем игровой обработчик
	if err := ga.handler.OnPlayerDisconnect(playerID); err != nil {
		ga.logger.Error("Handler disconnect error: player_id=%d error=%v", playerID, err)
	}

	ga.logger.Info("Player unregistered: player_id=%d channel_id=%s", playerID, channelID)
	return nil
}

// HandleIncomingMessage обрабатывает входящее сообщение из канала
func (ga *GameHandlerAdapter) HandleIncomingMessage(channelID string, netMsg *protocol.NetGameMessage) error {
	start := time.Now()

	// Находим игрока по каналу
	playerID, err := ga.findPlayerByChannel(channelID)
	if err != nil {
		return fmt.Errorf("failed to find player for channel %s: %w", channelID, err)
	}

	// Конвертируем NetGameMessage в GameMessage
	gameMsg, err := ga.convertNetToGameMessage(netMsg)
	if err != nil {
		ga.metrics.RecordError("serialization", err.Error(), channelID)
		return fmt.Errorf("failed to convert message: %w", err)
	}

	// Передаём в игровой обработчик
	if err := ga.handler.HandleMessage(playerID, gameMsg); err != nil {
		ga.logger.Error("Handler error: player_id=%d error=%v", playerID, err)
		return fmt.Errorf("handler error: %w", err)
	}

	// Записываем метрики
	processingTime := time.Since(start)
	messageSize := ga.calculateMessageSize(netMsg)
	ga.metrics.RecordMessage(ChannelKCP, netMsg, messageSize, false, processingTime)

	return nil
}

// SendToPlayer отправляет сообщение конкретному игроку
func (ga *GameHandlerAdapter) SendToPlayer(playerID uint64, gameMsg *protocol.GameMessage) error {
	// Находим канал игрока
	ga.connectionsMu.RLock()
	channelID, exists := ga.connections[playerID]
	ga.connectionsMu.RUnlock()

	if !exists {
		return fmt.Errorf("player %d not connected", playerID)
	}

	// Получаем канал
	channel, exists := ga.channelMgr.GetChannel(channelID)
	if !exists {
		return fmt.Errorf("channel %s not found", channelID)
	}

	// Конвертируем GameMessage в NetGameMessage
	netMsg, err := ga.convertGameToNetMessage(gameMsg)
	if err != nil {
		return fmt.Errorf("failed to convert message: %w", err)
	}

	// Определяем опции отправки в зависимости от типа сообщения
	opts := ga.getSendOptions(gameMsg)

	// Отправляем
	if err := channel.Send(ga.ctx, netMsg, opts); err != nil {
		ga.metrics.RecordError("connection", err.Error(), channelID)
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// BroadcastMessage отправляет сообщение всем подключённым игрокам
func (ga *GameHandlerAdapter) BroadcastMessage(gameMsg *protocol.GameMessage) error {
	// Конвертируем сообщение
	netMsg, err := ga.convertGameToNetMessage(gameMsg)
	if err != nil {
		return fmt.Errorf("failed to convert message: %w", err)
	}

	// Определяем опции
	opts := ga.getSendOptions(gameMsg)

	// Отправляем через менеджер каналов
	if err := ga.channelMgr.BroadcastMessage(netMsg, opts); err != nil {
		ga.logger.Error("Broadcast error: %v", err)
		return fmt.Errorf("broadcast failed: %w", err)
	}

	return nil
}

// GetConnectedPlayers возвращает список подключённых игроков
func (ga *GameHandlerAdapter) GetConnectedPlayers() []uint64 {
	ga.connectionsMu.RLock()
	defer ga.connectionsMu.RUnlock()

	players := make([]uint64, 0, len(ga.connections))
	for playerID := range ga.connections {
		players = append(players, playerID)
	}

	return players
}

// GetPlayerChannel возвращает ID канала для игрока
func (ga *GameHandlerAdapter) GetPlayerChannel(playerID uint64) (string, bool) {
	ga.connectionsMu.RLock()
	defer ga.connectionsMu.RUnlock()

	channelID, exists := ga.connections[playerID]
	return channelID, exists
}

// GetMetrics возвращает метрики сети
func (ga *GameHandlerAdapter) GetMetrics() *NetworkMetrics {
	return ga.metrics.GetSnapshot()
}

// Shutdown останавливает адаптер
func (ga *GameHandlerAdapter) Shutdown() error {
	ga.cancel()
	ga.wg.Wait()

	if err := ga.serializer.Close(); err != nil {
		ga.logger.Error("Failed to close serializer: %v", err)
	}

	ga.logger.Info("GameHandlerAdapter shutdown complete")
	return nil
}

// Приватные методы

func (ga *GameHandlerAdapter) findPlayerByChannel(channelID string) (uint64, error) {
	ga.connectionsMu.RLock()
	defer ga.connectionsMu.RUnlock()

	for playerID, playerChannelID := range ga.connections {
		if playerChannelID == channelID {
			return playerID, nil
		}
	}

	return 0, fmt.Errorf("no player found for channel %s", channelID)
}

func (ga *GameHandlerAdapter) convertNetToGameMessage(netMsg *protocol.NetGameMessage) (*protocol.GameMessage, error) {
	// Создаём GameMessage из NetGameMessage
	gameMsg := &protocol.GameMessage{
		Type:      protocol.MessageType_UNKNOWN, // Будет определён ниже
		Timestamp: time.Now().UnixNano(),
		Sequence:  netMsg.Sequence,
		Ack:       &netMsg.Ack,
		AckBits:   &netMsg.AckBits,
	}

	// Определяем тип и сериализуем payload
	var err error
	switch payload := netMsg.Payload.(type) {
	case *protocol.NetGameMessage_AuthRequest:
		gameMsg.Type = protocol.MessageType_AUTH
		gameMsg.Payload, err = ga.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_ChunkRequest:
		gameMsg.Type = protocol.MessageType_CHUNK_REQUEST
		gameMsg.Payload, err = ga.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_BlockUpdate:
		gameMsg.Type = protocol.MessageType_BLOCK_UPDATE
		gameMsg.Payload, err = ga.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_EntityMove:
		gameMsg.Type = protocol.MessageType_ENTITY_MOVE
		gameMsg.Payload, err = ga.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_Chat:
		gameMsg.Type = protocol.MessageType_CHAT
		gameMsg.Payload, err = ga.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	case *protocol.NetGameMessage_Ping:
		gameMsg.Type = protocol.MessageType_PING
		gameMsg.Payload, err = ga.serializer.Serialize(&protocol.NetGameMessage{Payload: payload})
	default:
		return nil, fmt.Errorf("unsupported message type: %T", payload)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to serialize payload: %w", err)
	}

	return gameMsg, nil
}

func (ga *GameHandlerAdapter) convertGameToNetMessage(gameMsg *protocol.GameMessage) (*protocol.NetGameMessage, error) {
	netMsg := &protocol.NetGameMessage{
		Sequence: gameMsg.Sequence,
		Flags:    protocol.NetFlags_RELIABLE_ORDERED, // По умолчанию
	}

	if gameMsg.Ack != nil {
		netMsg.Ack = *gameMsg.Ack
	}
	if gameMsg.AckBits != nil {
		netMsg.AckBits = *gameMsg.AckBits
	}

	// Десериализуем payload и определяем тип
	tempNetMsg, err := ga.serializer.Deserialize(gameMsg.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize payload: %w", err)
	}

	netMsg.Payload = tempNetMsg.Payload
	return netMsg, nil
}

func (ga *GameHandlerAdapter) getSendOptions(gameMsg *protocol.GameMessage) *SendOptions {
	opts := &SendOptions{
		Priority: PriorityNormal,
		Flags:    protocol.NetFlags_RELIABLE_ORDERED,
	}

	// Настраиваем опции в зависимости от типа сообщения
	switch gameMsg.Type {
	case protocol.MessageType_PING:
		opts.Priority = PriorityCritical
		opts.Flags = protocol.NetFlags_UNRELIABLE_UNORDERED
	case protocol.MessageType_ENTITY_MOVE:
		opts.Priority = PriorityHigh
		opts.Flags = protocol.NetFlags_UNRELIABLE_UNORDERED
	case protocol.MessageType_CHUNK_DATA:
		opts.Compression = protocol.CompressionType_ZSTD
	case protocol.MessageType_CHAT:
		opts.Compression = protocol.CompressionType_ZSTD
	}

	return opts
}

func (ga *GameHandlerAdapter) metricsLoop() {
	defer ga.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ga.metrics.UpdateBandwidth()
		case <-ga.ctx.Done():
			return
		}
	}
}

// calculateMessageSize вычисляет размер сообщения в байтах
func (ga *GameHandlerAdapter) calculateMessageSize(netMsg *protocol.NetGameMessage) int {
	// Базовый размер заголовка NetGameMessage
	baseSize := 16 // sequence (4) + ack (4) + ackBits (4) + flags (4)

	// Размер payload в зависимости от типа
	var payloadSize int
	switch payload := netMsg.Payload.(type) {
	case *protocol.NetGameMessage_AuthRequest:
		if payload.AuthRequest != nil {
			payloadSize = len(payload.AuthRequest.Username) + 8
			if payload.AuthRequest.Token != nil {
				payloadSize += len(*payload.AuthRequest.Token)
			}
		}
	case *protocol.NetGameMessage_ChunkRequest:
		if payload.ChunkRequest != nil {
			payloadSize = 16 // x (4) + y (4) + radius (4) + flags (4)
		}
	case *protocol.NetGameMessage_BlockUpdate:
		if payload.BlockUpdate != nil {
			payloadSize = 20 // position (8) + blockId (4) + layer (4) + flags (4)
		}
	case *protocol.NetGameMessage_EntityMove:
		if payload.EntityMove != nil {
			payloadSize = 32 // entityId (8) + position (16) + velocity (8)
		}
	case *protocol.NetGameMessage_Chat:
		if payload.Chat != nil {
			payloadSize = len(payload.Chat.Message) + 16
		}
	case *protocol.NetGameMessage_Ping:
		if payload.Ping != nil {
			payloadSize = 16 // timestamp (8) + sequence (8)
		}
	case *protocol.NetGameMessage_ChunkData:
		if payload.ChunkData != nil {
			payloadSize = 64 // приблизительный размер данных чанка
		}
	default:
		// Для неизвестных типов используем приблизительную оценку
		payloadSize = 64
	}

	return baseSize + payloadSize
}
