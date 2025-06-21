package replay

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/annel0/mmo-game/internal/protocol/events"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MockReplayService упрощенная реализация для демонстрации
type MockReplayService struct {
	events []*events.EventEnvelope
}

// NewMockReplayService создает новый mock сервис
func NewMockReplayService() *MockReplayService {
	service := &MockReplayService{
		events: make([]*events.EventEnvelope, 0),
	}

	// Добавляем тестовые события
	service.addSampleEvents()

	return service
}

// AddEvent добавляет событие в mock хранилище
func (s *MockReplayService) AddEvent(event *events.EventEnvelope) {
	s.events = append(s.events, event)
	log.Printf("📝 Added event: %s (%s)", event.EventType, event.EventId)
}

// QueryEvents возвращает события по фильтрам (упрощенная версия)
func (s *MockReplayService) QueryEvents(ctx context.Context, eventTypes []string, regionIDs []string, limit int32) ([]*events.EventEnvelope, error) {
	log.Printf("🔍 Query events: types=%v, regions=%v, limit=%d", eventTypes, regionIDs, limit)

	result := make([]*events.EventEnvelope, 0)
	count := int32(0)

	for _, event := range s.events {
		// Проверяем лимит
		if limit > 0 && count >= limit {
			break
		}

		// Фильтр по типам событий
		if len(eventTypes) > 0 {
			found := false
			for _, eventType := range eventTypes {
				if event.EventType == eventType {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		// Фильтр по регионам
		if len(regionIDs) > 0 {
			found := false
			for _, regionID := range regionIDs {
				if event.RegionId == regionID {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		result = append(result, event)
		count++
	}

	log.Printf("✅ Found %d events", len(result))
	return result, nil
}

// GetEventStats возвращает статистику событий
func (s *MockReplayService) GetEventStats(ctx context.Context) (map[string]int64, error) {
	stats := make(map[string]int64)

	for _, event := range s.events {
		stats[event.EventType]++
	}

	log.Printf("📊 Event stats: %v", stats)
	return stats, nil
}

// GetEventTypes возвращает доступные типы событий
func (s *MockReplayService) GetEventTypes(ctx context.Context) ([]string, error) {
	typeSet := make(map[string]bool)

	for _, event := range s.events {
		typeSet[event.EventType] = true
	}

	types := make([]string, 0, len(typeSet))
	for eventType := range typeSet {
		types = append(types, eventType)
	}

	log.Printf("📋 Available types: %v", types)
	return types, nil
}

// addSampleEvents добавляет тестовые события
func (s *MockReplayService) addSampleEvents() {
	now := time.Now()

	// Системное событие - запуск сервиса
	s.events = append(s.events, &events.EventEnvelope{
		EventId:       "evt-001",
		EventType:     "SystemEvent",
		Timestamp:     timestamppb.New(now.Add(-10 * time.Minute)),
		RegionId:      "eu-west-1",
		SourceNode:    "node-001",
		SchemaVersion: 1,
		Event: &events.EventEnvelope_SystemEvent{
			SystemEvent: &events.SystemEvent{
				Component: "game-server",
				Level:     "info",
				SystemAction: &events.SystemEvent_ServiceStarted{
					ServiceStarted: &events.SystemEvent_ServiceStarted{
						ServiceName: "mmo-game-server",
						Version:     "v1.0.0",
						Config: map[string]string{
							"region": "eu-west-1",
							"port":   "7777",
						},
					},
				},
			},
		},
	})

	// Событие мира - загрузка чанка
	s.events = append(s.events, &events.EventEnvelope{
		EventId:       "evt-002",
		EventType:     "WorldEvent",
		Timestamp:     timestamppb.New(now.Add(-8 * time.Minute)),
		RegionId:      "eu-west-1",
		SourceNode:    "node-001",
		SchemaVersion: 1,
		Event: &events.EventEnvelope_WorldEvent{
			WorldEvent: &events.WorldEvent{
				WorldId: "world-001",
				WorldAction: &events.WorldEvent_ChunkLoaded{
					ChunkLoaded: &events.WorldEvent_ChunkLoaded{
						Coords: &events.ChunkCoords{
							X: 0,
							Y: 0,
						},
						BlockCount: 256,
					},
				},
			},
		},
	})

	// Событие блока - размещение блока
	s.events = append(s.events, &events.EventEnvelope{
		EventId:       "evt-003",
		EventType:     "BlockEvent",
		Timestamp:     timestamppb.New(now.Add(-5 * time.Minute)),
		RegionId:      "eu-west-1",
		SourceNode:    "node-001",
		SchemaVersion: 1,
		Event: &events.EventEnvelope_BlockEvent{
			BlockEvent: &events.BlockEvent{
				Coords: &events.BlockCoords{
					X: 10,
					Y: 20,
					Z: 1,
				},
				PlayerId: "player-123",
				BlockAction: &events.BlockEvent_BlockPlaced{
					BlockPlaced: &events.BlockEvent_BlockPlaced{
						BlockType: "stone",
						Layer:     "active",
						Metadata: map[string]string{
							"durability": "100",
							"placed_by":  "player-123",
						},
					},
				},
			},
		},
	})

	// Событие чата
	s.events = append(s.events, &events.EventEnvelope{
		EventId:       "evt-004",
		EventType:     "ChatEvent",
		Timestamp:     timestamppb.New(now.Add(-2 * time.Minute)),
		RegionId:      "eu-west-1",
		SourceNode:    "node-001",
		SchemaVersion: 1,
		Event: &events.EventEnvelope_ChatEvent{
			ChatEvent: &events.ChatEvent{
				PlayerId: "player-123",
				Channel:  "global",
				ChatAction: &events.ChatEvent_MessageSent{
					MessageSent: &events.ChatEvent_MessageSent{
						Message:     "Hello, world!",
						MessageType: "public",
					},
				},
			},
		},
	})

	log.Printf("✅ Added %d sample events", len(s.events))
}

// PrintAllEvents выводит все события для отладки
func (s *MockReplayService) PrintAllEvents() {
	fmt.Println("📋 All events in store:")
	for i, event := range s.events {
		timestamp := event.Timestamp.AsTime().Format("15:04:05")
		fmt.Printf("%d. [%s] %s/%s [%s] %s\n",
			i+1, timestamp, event.RegionId, event.SourceNode,
			event.EventType, event.EventId)
	}
}
