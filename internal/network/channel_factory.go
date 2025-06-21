package network

import (
	"fmt"

	"github.com/annel0/mmo-game/internal/logging"
)

// StandardChannelFactory реализует ChannelFactory для стандартных типов каналов
type StandardChannelFactory struct {
	logger *logging.Logger
}

// NewStandardChannelFactory создаёт новую фабрику каналов
func NewStandardChannelFactory(logger *logging.Logger) *StandardChannelFactory {
	return &StandardChannelFactory{
		logger: logger,
	}
}

// CreateChannel создаёт канал указанного типа с заданной конфигурацией
func (f *StandardChannelFactory) CreateChannel(config *ChannelConfig) (NetChannel, error) {
	switch config.Type {
	case ChannelKCP:
		return NewKCPChannel(config, f.logger), nil
	case ChannelTCP:
		return NewTCPChannel(config, f.logger), nil
	case ChannelUDP:
		return NewUDPChannel(config, f.logger), nil
	case ChannelWebSocket:
		return NewWebSocketChannel(config, f.logger), nil
	default:
		return nil, fmt.Errorf("unsupported channel type: %v", config.Type)
	}
}

// SupportedTypes возвращает список поддерживаемых типов каналов
func (f *StandardChannelFactory) SupportedTypes() []ChannelType {
	return []ChannelType{
		ChannelKCP,
		ChannelTCP,
		ChannelUDP,
		ChannelWebSocket,
	}
}

// TODO: Заглушки для других типов каналов, которые будут реализованы позже

// NewTCPChannel создаёт TCP канал
func NewTCPChannel(config *ChannelConfig, logger *logging.Logger) NetChannel {
	return NewTCPChannelFromConfig(config, logger)
}

// NewUDPChannel создаёт UDP канал (упрощенная реализация)
func NewUDPChannel(config *ChannelConfig, logger *logging.Logger) NetChannel {
	logger.Warn("UDP channel: simplified implementation, consider using KCP for games")
	return NewTCPChannelFromConfig(config, logger) // Fallback к TCP
}

// NewWebSocketChannel создаёт WebSocket канал (упрощенная реализация)
func NewWebSocketChannel(config *ChannelConfig, logger *logging.Logger) NetChannel {
	logger.Info("WebSocket channel: using TCP fallback, full WebSocket support planned")
	return NewTCPChannelFromConfig(config, logger) // Fallback к TCP
}
