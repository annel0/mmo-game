package replay

import (
	"context"
	"time"

	"github.com/annel0/mmo-game/internal/protocol/events"
	"github.com/annel0/mmo-game/internal/protocol/replay"
)

// EventStore интерфейс для хранения и запроса событий
type EventStore interface {
	// WriteEvent записывает событие в хранилище
	WriteEvent(ctx context.Context, event *events.EventEnvelope) error

	// WriteBatch записывает несколько событий в одной транзакции
	WriteBatch(ctx context.Context, events []*events.EventEnvelope) error

	// QueryEvents возвращает события по фильтрам
	QueryEvents(ctx context.Context, req *replay.ReplayRequest) ([]*events.EventEnvelope, string, error)

	// StreamEvents возвращает канал для стриминга событий
	StreamEvents(ctx context.Context, req *replay.ReplayRequest) (<-chan *events.EventEnvelope, <-chan error)

	// GetEventStats возвращает статистику событий
	GetEventStats(ctx context.Context, req *replay.EventStatsRequest) (*replay.EventStatsResponse, error)

	// GetEventTypes возвращает доступные типы событий
	GetEventTypes(ctx context.Context, req *replay.EventTypesRequest) (*replay.EventTypesResponse, error)

	// Close закрывает соединение с хранилищем
	Close() error
}

// EventFilter содержит фильтры для запроса событий
type EventFilter struct {
	StartTime  time.Time
	EndTime    time.Time
	EventTypes []string
	RegionIDs  []string
	PlayerIDs  []string
	WorldID    string
	Limit      int32
	Cursor     string
	SortOrder  replay.ReplayRequest_SortOrder
}

// EventStoreConfig конфигурация для EventStore
type EventStoreConfig struct {
	// MariaDB настройки
	MariaDB struct {
		DSN            string
		MaxConnections int
		MaxIdleTime    time.Duration
		TableName      string
		PartitionByDay bool
	}

	// Кеширование
	Cache struct {
		Enabled    bool
		TTL        time.Duration
		MaxEntries int
	}

	// Метрики
	Metrics struct {
		Enabled bool
		Prefix  string
	}
}
