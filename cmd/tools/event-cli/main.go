package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/annel0/mmo-game/internal/protocol/events"
	"github.com/annel0/mmo-game/internal/protocol/replay"
)

// MockReplayServiceClient - заглушка для gRPC клиента
type MockReplayServiceClient struct{}

func NewReplayServiceClient() *MockReplayServiceClient {
	return &MockReplayServiceClient{}
}

func (c *MockReplayServiceClient) StreamEvents(ctx context.Context, filter *replay.ReplayFilter) ([]events.Event, error) {
	// Возвращаем тестовые события
	return []events.Event{
		{
			Type:      events.EventTypeSystem,
			Timestamp: time.Now().Unix(),
			Data:      map[string]interface{}{"component": "server", "action": "started"},
		},
		{
			Type:      events.EventTypeWorld,
			Timestamp: time.Now().Unix(),
			Data:      map[string]interface{}{"chunk_x": 0, "chunk_y": 0, "action": "loaded"},
		},
		{
			Type:      events.EventTypeBlock,
			Timestamp: time.Now().Unix(),
			Data:      map[string]interface{}{"x": 10, "y": 20, "block_id": 1, "action": "placed"},
		},
		{
			Type:      events.EventTypeChat,
			Timestamp: time.Now().Unix(),
			Data:      map[string]interface{}{"player_id": 123, "message": "Hello world!", "channel": "global"},
		},
	}, nil
}

func (c *MockReplayServiceClient) GetEventStats(ctx context.Context, filter *replay.ReplayFilter) (map[string]interface{}, error) {
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
	}, nil
}

func (c *MockReplayServiceClient) GetEventTypes(ctx context.Context) ([]string, error) {
	return []string{"system", "world", "block", "chat"}, nil
}

func main() {
	var (
		serverAddr = flag.String("server", "localhost:9090", "gRPC server address")
		command    = flag.String("command", "tail", "Command to execute: tail, stats, types")
		eventTypes = flag.String("types", "", "Comma-separated event types to filter")
		region     = flag.String("region", "", "Region to filter events")
		playerID   = flag.Uint64("player", 0, "Player ID to filter events")
		follow     = flag.Bool("follow", false, "Follow mode (like tail -f)")
		limit      = flag.Int("limit", 100, "Maximum number of events to show")
	)
	flag.Parse()

	fmt.Printf("🎮 MMO Event CLI Tool\n")
	fmt.Printf("Server: %s\n", *serverAddr)
	fmt.Printf("Command: %s\n\n", *command)

	// Создаем клиент (заглушка)
	client := NewReplayServiceClient()

	// Создаем фильтр
	filter := &replay.ReplayFilter{
		Region:   *region,
		PlayerID: *playerID,
	}

	// Парсим типы событий
	if *eventTypes != "" {
		types := strings.Split(*eventTypes, ",")
		for _, t := range types {
			filter.EventTypes = append(filter.EventTypes, events.EventType(strings.TrimSpace(t)))
		}
	}

	ctx := context.Background()

	switch *command {
	case "tail":
		err := tailEvents(ctx, client, filter, *follow, *limit)
		if err != nil {
			log.Fatalf("Failed to tail events: %v", err)
		}

	case "stats":
		err := showStats(ctx, client, filter)
		if err != nil {
			log.Fatalf("Failed to get stats: %v", err)
		}

	case "types":
		err := showEventTypes(ctx, client)
		if err != nil {
			log.Fatalf("Failed to get event types: %v", err)
		}

	default:
		fmt.Printf("Unknown command: %s\n", *command)
		fmt.Printf("Available commands: tail, stats, types\n")
		os.Exit(1)
	}
}

func tailEvents(ctx context.Context, client *MockReplayServiceClient, filter *replay.ReplayFilter, follow bool, limit int) error {
	fmt.Printf("📡 Tailing events...\n")
	if len(filter.EventTypes) > 0 {
		fmt.Printf("Types: %v\n", filter.EventTypes)
	}
	if filter.Region != "" {
		fmt.Printf("Region: %s\n", filter.Region)
	}
	if filter.PlayerID != 0 {
		fmt.Printf("Player: %d\n", filter.PlayerID)
	}
	fmt.Printf("\n")

	for {
		events, err := client.StreamEvents(ctx, filter)
		if err != nil {
			return fmt.Errorf("failed to stream events: %w", err)
		}

		count := 0
		for _, event := range events {
			if count >= limit {
				break
			}

			// Применяем фильтры
			if len(filter.EventTypes) > 0 {
				found := false
				for _, t := range filter.EventTypes {
					if event.Type == t {
						found = true
						break
					}
				}
				if !found {
					continue
				}
			}

			printEvent(event)
			count++
		}

		if !follow {
			break
		}

		time.Sleep(1 * time.Second)
	}

	return nil
}

func showStats(ctx context.Context, client *MockReplayServiceClient, filter *replay.ReplayFilter) error {
	fmt.Printf("📊 Event Statistics\n")
	if filter.Region != "" {
		fmt.Printf("Region: %s\n", filter.Region)
	}
	fmt.Printf("\n")

	stats, err := client.GetEventStats(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Printf("Total Events: %v\n", stats["total_events"])
	fmt.Printf("\nEvent Types:\n")

	if eventTypes, ok := stats["event_types"].(map[string]int); ok {
		for eventType, count := range eventTypes {
			fmt.Printf("  %s: %d\n", eventType, count)
		}
	}

	if timeRange, ok := stats["time_range"].(map[string]interface{}); ok {
		fmt.Printf("\nTime Range:\n")
		fmt.Printf("  Start: %v\n", timeRange["start"])
		fmt.Printf("  End: %v\n", timeRange["end"])
	}

	return nil
}

func showEventTypes(ctx context.Context, client *MockReplayServiceClient) error {
	fmt.Printf("📋 Available Event Types\n\n")

	types, err := client.GetEventTypes(ctx)
	if err != nil {
		return fmt.Errorf("failed to get event types: %w", err)
	}

	for i, eventType := range types {
		fmt.Printf("%d. %s\n", i+1, eventType)
	}

	fmt.Printf("\nUsage examples:\n")
	fmt.Printf("  event-cli -command=tail -types=world,block\n")
	fmt.Printf("  event-cli -command=stats -region=eu-west\n")
	fmt.Printf("  event-cli -command=tail -player=123 -follow\n")

	return nil
}

func printEvent(event events.Event) {
	timestamp := time.Unix(event.Timestamp, 0).Format("15:04:05")

	fmt.Printf("[%s] %s: ", timestamp, event.Type)

	switch event.Type {
	case events.EventTypeSystem:
		if component, ok := event.Data["component"].(string); ok {
			if action, ok := event.Data["action"].(string); ok {
				fmt.Printf("Component %s %s\n", component, action)
			}
		}
	case events.EventTypeWorld:
		if x, okX := event.Data["chunk_x"]; okX {
			if y, okY := event.Data["chunk_y"]; okY {
				if action, okA := event.Data["action"].(string); okA {
					fmt.Printf("Chunk (%v,%v) %s\n", x, y, action)
				}
			}
		}
	case events.EventTypeBlock:
		if x, okX := event.Data["x"]; okX {
			if y, okY := event.Data["y"]; okY {
				if blockID, okB := event.Data["block_id"]; okB {
					if action, okA := event.Data["action"].(string); okA {
						fmt.Printf("Block at (%v,%v) ID=%v %s\n", x, y, blockID, action)
					}
				}
			}
		}
	case events.EventTypeChat:
		if playerID, okP := event.Data["player_id"]; okP {
			if message, okM := event.Data["message"].(string); okM {
				if channel, okC := event.Data["channel"].(string); okC {
					fmt.Printf("Player %v in %s: %s\n", playerID, channel, message)
				}
			}
		}
	default:
		fmt.Printf("%v\n", event.Data)
	}
}
