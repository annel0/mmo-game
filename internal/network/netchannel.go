// Package network предоставляет унифицированный интерфейс для сетевых каналов
package network

import (
	"context"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
)

// ChannelFlags определяет флаги для отправки сообщений
type ChannelFlags uint8

const (
	// FlagReliable гарантирует доставку сообщения
	FlagReliable ChannelFlags = 1 << iota
	// FlagOrdered гарантирует порядок доставки сообщений
	FlagOrdered
	// FlagUnsequenced отправляет без гарантии порядка (для позиций)
	FlagUnsequenced
)

// ChannelType определяет тип канала связи
type ChannelType int

const (
	ChannelTCP ChannelType = iota
	ChannelUDP
	ChannelKCP
	ChannelWebSocket
)

// ConnectionStats содержит статистику соединения
type ConnectionStats struct {
	RTT             time.Duration // Round-trip time
	PacketsSent     uint64        // Отправлено пакетов
	PacketsReceived uint64        // Получено пакетов
	PacketsLost     uint64        // Потеряно пакетов
	BytesSent       uint64        // Отправлено байт
	BytesReceived   uint64        // Получено байт
	LastActivity    time.Time     // Последняя активность
	Connected       bool          // Статус соединения
	RemoteAddr      string        // Адрес удалённого узла
}

// MessagePriority определяет приоритет сообщения
type MessagePriority int

const (
	PriorityLow MessagePriority = iota
	PriorityNormal
	PriorityHigh
	PriorityCritical
)

// SendOptions настройки отправки сообщения
type SendOptions struct {
	Priority    MessagePriority          // Приоритет сообщения
	Flags       protocol.NetFlags        // Флаги надёжности
	Compression protocol.CompressionType // Тип сжатия
	Timeout     time.Duration            // Таймаут отправки
}

// NetChannel представляет унифицированный интерфейс для сетевого канала
type NetChannel interface {
	// Основные операции
	Send(ctx context.Context, msg *protocol.NetGameMessage, opts *SendOptions) error
	Receive(ctx context.Context) (*protocol.NetGameMessage, error)
	Close() error

	// Управление соединением
	Connect(ctx context.Context, addr string) error
	IsConnected() bool
	RemoteAddr() string

	// Статистика и мониторинг
	Stats() ConnectionStats
	RTT() time.Duration

	// Настройки канала
	SetBufferSize(size int) error
	SetTimeout(timeout time.Duration) error
	SetKeepAlive(interval time.Duration) error

	// События
	OnMessage(handler func(*protocol.NetGameMessage)) error
	OnConnect(handler func()) error
	OnDisconnect(handler func(error)) error
	OnError(handler func(error)) error
}

// ChannelConfig содержит конфигурацию канала
type ChannelConfig struct {
	Type            ChannelType
	BufferSize      int
	Timeout         time.Duration
	KeepAlive       time.Duration
	CompressionType protocol.CompressionType
	MaxRetries      int
	RetryInterval   time.Duration
}

// DefaultChannelConfig возвращает конфигурацию канала по умолчанию
func DefaultChannelConfig(channelType ChannelType) *ChannelConfig {
	return &ChannelConfig{
		Type:            channelType,
		BufferSize:      1024,
		Timeout:         30 * time.Second,
		KeepAlive:       10 * time.Second,
		CompressionType: protocol.CompressionType_NONE,
		MaxRetries:      3,
		RetryInterval:   time.Second,
	}
}

// ChannelFactory создаёт каналы разных типов
type ChannelFactory interface {
	CreateChannel(config *ChannelConfig) (NetChannel, error)
	SupportedTypes() []ChannelType
}

// ChannelStats содержит статистику канала
type ChannelStats struct {
	PacketsSent     uint64
	PacketsReceived uint64
	BytesSent       uint64
	BytesReceived   uint64
	PacketsLost     uint64
	RTT             uint32 // в миллисекундах
	Jitter          uint32 // в миллисекундах
}
