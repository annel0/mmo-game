package replay

import (
	"context"
	"fmt"
	"time"

	"github.com/annel0/mmo-game/internal/protocol/events"
)

// EventStore представляет интерфейс хранилища событий
type EventStore interface {
	QueryEvents(ctx context.Context, query EventQuery) ([]*EventEnvelope, error)
	GetEventStats(ctx context.Context, query EventQuery) (*EventStats, error)
	GetEventTypes(ctx context.Context) ([]string, error)
}

// ReplayFilter определяет фильтры для воспроизведения
type ReplayFilter struct {
	EventTypes []events.EventType `json:"event_types"`
	StartTime  *time.Time         `json:"start_time,omitempty"`
	EndTime    *time.Time         `json:"end_time,omitempty"`
	Region     string             `json:"region,omitempty"`
	PlayerID   uint64             `json:"player_id,omitempty"`
}

// EventQuery представляет запрос к хранилищу событий
type EventQuery struct {
	EventTypes []string   `json:"event_types"`
	StartTime  *time.Time `json:"start_time,omitempty"`
	EndTime    *time.Time `json:"end_time,omitempty"`
	Region     string     `json:"region,omitempty"`
	PlayerID   uint64     `json:"player_id,omitempty"`
	Limit      int        `json:"limit,omitempty"`
}

// EventEnvelope представляет обертку события
type EventEnvelope struct {
	EventID    string                 `json:"event_id"`
	EventType  string                 `json:"event_type"`
	Timestamp  time.Time              `json:"timestamp"`
	RegionID   string                 `json:"region_id"`
	SourceNode string                 `json:"source_node"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// EventStats представляет статистику событий
type EventStats struct {
	TotalEvents int64                  `json:"total_events"`
	EventTypes  map[string]int         `json:"event_types"`
	TimeRange   map[string]interface{} `json:"time_range"`
}

// ReplayService представляет сервис воспроизведения событий
type ReplayService struct {
	eventStore EventStore
}

// NewReplayService создает новый сервис воспроизведения
func NewReplayService(eventStore EventStore) *ReplayService {
	return &ReplayService{
		eventStore: eventStore,
	}
}

// StreamEvents возвращает поток событий по фильтру
func (s *ReplayService) StreamEvents(ctx context.Context, filter *ReplayFilter) ([]events.Event, error) {
	if s.eventStore == nil {
		return nil, fmt.Errorf("event store not configured")
	}

	// Конвертируем фильтр в параметры запроса
	query := EventQuery{
		EventTypes: make([]string, len(filter.EventTypes)),
		Region:     filter.Region,
		PlayerID:   filter.PlayerID,
		StartTime:  filter.StartTime,
		EndTime:    filter.EndTime,
	}

	for i, t := range filter.EventTypes {
		query.EventTypes[i] = string(t)
	}

	// Получаем события из хранилища
	eventEnvelopes, err := s.eventStore.QueryEvents(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}

	// Конвертируем в простые события
	result := make([]events.Event, len(eventEnvelopes))
	for i, envelope := range eventEnvelopes {
		result[i] = events.Event{
			Type:      events.EventType(envelope.EventType),
			Timestamp: envelope.Timestamp.Unix(),
			Data:      envelope.Metadata,
		}
	}

	return result, nil
}

// GetEventStats возвращает статистику событий
func (s *ReplayService) GetEventStats(ctx context.Context, filter *ReplayFilter) (map[string]interface{}, error) {
	if s.eventStore == nil {
		return nil, fmt.Errorf("event store not configured")
	}

	query := EventQuery{
		Region:   filter.Region,
		PlayerID: filter.PlayerID,
	}

	stats, err := s.eventStore.GetEventStats(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	return map[string]interface{}{
		"total_events": stats.TotalEvents,
		"event_types":  stats.EventTypes,
		"time_range":   stats.TimeRange,
	}, nil
}

// GetEventTypes возвращает доступные типы событий
func (s *ReplayService) GetEventTypes(ctx context.Context) ([]string, error) {
	if s.eventStore == nil {
		return nil, fmt.Errorf("event store not configured")
	}

	types, err := s.eventStore.GetEventTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get event types: %w", err)
	}

	return types, nil
}
