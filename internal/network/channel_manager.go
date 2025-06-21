package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/annel0/mmo-game/internal/protocol"
)

// ChannelManager управляет множественными сетевыми каналами
type ChannelManager struct {
	channels   map[string]NetChannel
	channelsMu sync.RWMutex
	factory    ChannelFactory
	logger     *logging.Logger
	ctx        context.Context
	cancel     context.CancelFunc
	metrics    *ChannelMetrics
}

// ChannelMetrics содержит метрики для всех каналов
type ChannelMetrics struct {
	TotalChannels    int64
	ActiveChannels   int64
	MessagesSent     int64
	MessagesReceived int64
	BytesSent        int64
	BytesReceived    int64
	ConnectionErrors int64
	LastUpdate       time.Time
	mu               sync.RWMutex
}

// NewChannelManager создаёт новый менеджер каналов
func NewChannelManager(factory ChannelFactory, logger *logging.Logger) *ChannelManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &ChannelManager{
		channels: make(map[string]NetChannel),
		factory:  factory,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		metrics:  &ChannelMetrics{},
	}
}

// CreateChannel создаёт новый канал с указанным ID
func (cm *ChannelManager) CreateChannel(channelID string, config *ChannelConfig) (NetChannel, error) {
	cm.channelsMu.Lock()
	defer cm.channelsMu.Unlock()

	// Проверяем, что канал с таким ID не существует
	if _, exists := cm.channels[channelID]; exists {
		return nil, fmt.Errorf("channel with ID %s already exists", channelID)
	}

	// Создаём канал через фабрику
	channel, err := cm.factory.CreateChannel(config)
	if err != nil {
		cm.updateMetrics(func(m *ChannelMetrics) {
			m.ConnectionErrors++
		})
		return nil, fmt.Errorf("failed to create channel: %w", err)
	}

	// Настраиваем обработчики событий
	if err := cm.setupChannelHandlers(channelID, channel); err != nil {
		channel.Close()
		return nil, fmt.Errorf("failed to setup channel handlers: %w", err)
	}

	// Сохраняем канал
	cm.channels[channelID] = channel

	cm.updateMetrics(func(m *ChannelMetrics) {
		m.TotalChannels++
		m.ActiveChannels++
	})

	cm.logger.Info("Created channel: id=%s type=%v", channelID, config.Type)
	return channel, nil
}

// GetChannel возвращает канал по ID
func (cm *ChannelManager) GetChannel(channelID string) (NetChannel, bool) {
	cm.channelsMu.RLock()
	defer cm.channelsMu.RUnlock()

	channel, exists := cm.channels[channelID]
	return channel, exists
}

// CloseChannel закрывает канал по ID
func (cm *ChannelManager) CloseChannel(channelID string) error {
	cm.channelsMu.Lock()
	defer cm.channelsMu.Unlock()

	channel, exists := cm.channels[channelID]
	if !exists {
		return fmt.Errorf("channel %s not found", channelID)
	}

	if err := channel.Close(); err != nil {
		cm.logger.Error("Error closing channel: id=%s error=%v", channelID, err)
	}

	delete(cm.channels, channelID)

	cm.updateMetrics(func(m *ChannelMetrics) {
		m.ActiveChannels--
	})

	cm.logger.Info("Closed channel: id=%s", channelID)
	return nil
}

// BroadcastMessage отправляет сообщение всем активным каналам
func (cm *ChannelManager) BroadcastMessage(msg *protocol.NetGameMessage, opts *SendOptions) error {
	cm.channelsMu.RLock()
	channels := make([]NetChannel, 0, len(cm.channels))
	for _, channel := range cm.channels {
		if channel.IsConnected() {
			channels = append(channels, channel)
		}
	}
	cm.channelsMu.RUnlock()

	var lastErr error
	for _, channel := range channels {
		if err := channel.Send(cm.ctx, msg, opts); err != nil {
			lastErr = err
			cm.logger.Error("Failed to broadcast message: %v", err)
		}
	}

	return lastErr
}

// GetActiveChannels возвращает список активных каналов
func (cm *ChannelManager) GetActiveChannels() []string {
	cm.channelsMu.RLock()
	defer cm.channelsMu.RUnlock()

	active := make([]string, 0, len(cm.channels))
	for id, channel := range cm.channels {
		if channel.IsConnected() {
			active = append(active, id)
		}
	}

	return active
}

// GetMetrics возвращает текущие метрики
func (cm *ChannelManager) GetMetrics() *ChannelMetrics {
	cm.metrics.mu.RLock()
	defer cm.metrics.mu.RUnlock()

	// Создаём копию без мьютекса
	snapshot := &ChannelMetrics{
		TotalChannels:    cm.metrics.TotalChannels,
		ActiveChannels:   cm.metrics.ActiveChannels,
		MessagesSent:     cm.metrics.MessagesSent,
		MessagesReceived: cm.metrics.MessagesReceived,
		BytesSent:        cm.metrics.BytesSent,
		BytesReceived:    cm.metrics.BytesReceived,
		ConnectionErrors: cm.metrics.ConnectionErrors,
		LastUpdate:       cm.metrics.LastUpdate,
	}

	return snapshot
}

// Shutdown закрывает все каналы и останавливает менеджер
func (cm *ChannelManager) Shutdown() error {
	cm.cancel()

	cm.channelsMu.Lock()
	defer cm.channelsMu.Unlock()

	var lastErr error
	for id, channel := range cm.channels {
		if err := channel.Close(); err != nil {
			lastErr = err
			cm.logger.Error("Error closing channel during shutdown: id=%s error=%v", id, err)
		}
	}

	cm.channels = make(map[string]NetChannel)
	cm.logger.Info("Channel manager shutdown complete")
	return lastErr
}

// setupChannelHandlers настраивает обработчики событий для канала
func (cm *ChannelManager) setupChannelHandlers(channelID string, channel NetChannel) error {
	// Обработчик сообщений
	if err := channel.OnMessage(func(msg *protocol.NetGameMessage) {
		cm.updateMetrics(func(m *ChannelMetrics) {
			m.MessagesReceived++
		})
		cm.logger.Debug("Received message: channel=%s type=%v", channelID, msg.Payload)
	}); err != nil {
		return err
	}

	// Обработчик подключения
	if err := channel.OnConnect(func() {
		cm.logger.Info("Channel connected: id=%s", channelID)
	}); err != nil {
		return err
	}

	// Обработчик отключения
	if err := channel.OnDisconnect(func(err error) {
		cm.logger.Warn("Channel disconnected: id=%s error=%v", channelID, err)
		cm.updateMetrics(func(m *ChannelMetrics) {
			if err != nil {
				m.ConnectionErrors++
			}
		})
	}); err != nil {
		return err
	}

	// Обработчик ошибок
	if err := channel.OnError(func(err error) {
		cm.logger.Error("Channel error: id=%s error=%v", channelID, err)
		cm.updateMetrics(func(m *ChannelMetrics) {
			m.ConnectionErrors++
		})
	}); err != nil {
		return err
	}

	return nil
}

// updateMetrics обновляет метрики потокобезопасно
func (cm *ChannelManager) updateMetrics(updateFunc func(*ChannelMetrics)) {
	cm.metrics.mu.Lock()
	defer cm.metrics.mu.Unlock()

	updateFunc(cm.metrics)
	cm.metrics.LastUpdate = time.Now()
}
