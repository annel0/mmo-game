package replay

import (
	"context"
	"time"

	"github.com/annel0/mmo-game/internal/protocol/events"
)

// MockReplayService представляет мок-сервис для тестирования
type MockReplayService struct{}

// NewMockReplayService создает новый мок-сервис
func NewMockReplayService() *MockReplayService {
	return &MockReplayService{}
}

// StreamEvents возвращает тестовые события
func (m *MockReplayService) StreamEvents(ctx context.Context, filter *ReplayFilter) ([]events.Event, error) {
	now := time.Now()

	return []events.Event{
		{
			Type:      events.EventTypeSystem,
			Timestamp: now.Add(-3 * time.Hour).Unix(),
			Data: map[string]interface{}{
				"component": "server",
				"action":    "started",
				"version":   "v0.3.0-alpha",
			},
		},
		{
			Type:      events.EventTypeWorld,
			Timestamp: now.Add(-2 * time.Hour).Unix(),
			Data: map[string]interface{}{
				"chunk_x": 0,
				"chunk_y": 0,
				"action":  "loaded",
				"region":  "eu-west",
			},
		},
		{
			Type:      events.EventTypeBlock,
			Timestamp: now.Add(-1 * time.Hour).Unix(),
			Data: map[string]interface{}{
				"x":         10,
				"y":         20,
				"z":         0,
				"block_id":  1,
				"action":    "placed",
				"player_id": uint64(123),
			},
		},
		{
			Type:      events.EventTypeChat,
			Timestamp: now.Add(-30 * time.Minute).Unix(),
			Data: map[string]interface{}{
				"player_id": uint64(123),
				"message":   "Hello world!",
				"channel":   "global",
			},
		},
	}, nil
}

// GetEventStats возвращает тестовую статистику
func (m *MockReplayService) GetEventStats(ctx context.Context, filter *ReplayFilter) (map[string]interface{}, error) {
	return map[string]interface{}{
		"total_events": 1234,
		"event_types": map[string]int{
			"system": 45,
			"world":  567,
			"block":  890,
			"chat":   234,
		},
		"time_range": map[string]interface{}{
			"start": time.Now().Add(-24 * time.Hour).Format(time.RFC3339),
			"end":   time.Now().Format(time.RFC3339),
		},
		"regions": []string{"eu-west", "us-east"},
		"players": 42,
	}, nil
}

// GetEventTypes возвращает доступные типы событий
func (m *MockReplayService) GetEventTypes(ctx context.Context) ([]string, error) {
	return []string{
		string(events.EventTypeSystem),
		string(events.EventTypeWorld),
		string(events.EventTypeBlock),
		string(events.EventTypeChat),
	}, nil
}
