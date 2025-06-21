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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultServerAddr = "localhost:9090"
	timeFormat        = "2006-01-02T15:04:05Z"
)

func main() {
	var (
		serverAddr = flag.String("server", defaultServerAddr, "gRPC server address")
		command    = flag.String("cmd", "tail", "Command: tail, stats, types")
		eventTypes = flag.String("types", "", "Event types filter (comma-separated)")
		regions    = flag.String("regions", "", "Region IDs filter (comma-separated)")
		players    = flag.String("players", "", "Player IDs filter (comma-separated)")
		worldID    = flag.String("world", "", "World ID filter")
		since      = flag.String("since", "1h", "Time duration since now (e.g., 1h, 30m, 1d)")
		until      = flag.String("until", "", "End time (RFC3339 format)")
		limit      = flag.Int("limit", 100, "Maximum number of events")
		follow     = flag.Bool("follow", false, "Follow new events (like tail -f)")
	)
	flag.Parse()

	// Подключаемся к gRPC серверу
	conn, err := grpc.Dial(*serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("❌ Failed to connect to server: %v", err)
	}
	defer conn.Close()

	client := replay.NewReplayServiceClient(conn)

	// Выполняем команду
	switch *command {
	case "tail":
		if err := tailEvents(client, &TailOptions{
			EventTypes: parseStringList(*eventTypes),
			Regions:    parseStringList(*regions),
			Players:    parseStringList(*players),
			WorldID:    *worldID,
			Since:      *since,
			Until:      *until,
			Limit:      int32(*limit),
			Follow:     *follow,
		}); err != nil {
			log.Fatalf("❌ Tail failed: %v", err)
		}

	case "stats":
		if err := showStats(client, &StatsOptions{
			EventTypes: parseStringList(*eventTypes),
			Regions:    parseStringList(*regions),
			Since:      *since,
			Until:      *until,
		}); err != nil {
			log.Fatalf("❌ Stats failed: %v", err)
		}

	case "types":
		if err := showTypes(client, &TypesOptions{
			Since: *since,
			Until: *until,
		}); err != nil {
			log.Fatalf("❌ Types failed: %v", err)
		}

	default:
		fmt.Printf("❌ Unknown command: %s\n", *command)
		fmt.Println("Available commands: tail, stats, types")
		os.Exit(1)
	}
}

type TailOptions struct {
	EventTypes []string
	Regions    []string
	Players    []string
	WorldID    string
	Since      string
	Until      string
	Limit      int32
	Follow     bool
}

type StatsOptions struct {
	EventTypes []string
	Regions    []string
	Since      string
	Until      string
}

type TypesOptions struct {
	Since string
	Until string
}

// tailEvents выводит события в реальном времени
func tailEvents(client replay.ReplayServiceClient, opts *TailOptions) error {
	fmt.Printf("🎬 Tailing events (limit: %d, follow: %v)\n", opts.Limit, opts.Follow)

	// Парсим временные границы
	endTime := time.Now()
	if opts.Until != "" {
		var err error
		endTime, err = time.Parse(timeFormat, opts.Until)
		if err != nil {
			return fmt.Errorf("invalid until time: %v", err)
		}
	}

	startTime, err := parseSinceTime(opts.Since, endTime)
	if err != nil {
		return fmt.Errorf("invalid since time: %v", err)
	}

	// Создаем запрос
	req := &replay.ReplayRequest{
		StartTime:  timestamppb.New(startTime),
		EndTime:    timestamppb.New(endTime),
		EventTypes: opts.EventTypes,
		RegionIds:  opts.Regions,
		PlayerIds:  opts.Players,
		WorldId:    opts.WorldID,
		Limit:      opts.Limit,
		SortOrder:  replay.ReplayRequest_SORT_ORDER_ASC,
	}

	// Получаем стрим событий
	stream, err := client.Replay(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to start replay: %v", err)
	}

	// Читаем события
	eventCount := 0
	for {
		event, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("stream error: %v", err)
		}

		// Выводим событие
		printEvent(event)
		eventCount++

		// Если не follow режим и достигли лимита, выходим
		if !opts.Follow && eventCount >= int(opts.Limit) {
			break
		}
	}

	fmt.Printf("\n📊 Total events: %d\n", eventCount)
	return nil
}

// showStats выводит статистику событий
func showStats(client replay.ReplayServiceClient, opts *StatsOptions) error {
	fmt.Println("📊 Event statistics")

	// Парсим временные границы
	endTime := time.Now()
	if opts.Until != "" {
		var err error
		endTime, err = time.Parse(timeFormat, opts.Until)
		if err != nil {
			return fmt.Errorf("invalid until time: %v", err)
		}
	}

	startTime, err := parseSinceTime(opts.Since, endTime)
	if err != nil {
		return fmt.Errorf("invalid since time: %v", err)
	}

	// Создаем запрос
	req := &replay.EventStatsRequest{
		StartTime:  timestamppb.New(startTime),
		EndTime:    timestamppb.New(endTime),
		EventTypes: opts.EventTypes,
		RegionIds:  opts.Regions,
		GroupBy:    replay.EventStatsRequest_STATS_GROUP_BY_EVENT_TYPE,
	}

	// Получаем статистику
	stats, err := client.GetEventStats(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to get stats: %v", err)
	}

	// Выводим результаты
	fmt.Printf("Period: %s - %s\n", startTime.Format(timeFormat), endTime.Format(timeFormat))
	fmt.Printf("Total events: %d\n", stats.TotalEvents)
	fmt.Println("\nBy event type:")
	for _, stat := range stats.Stats {
		fmt.Printf("  %s: %d events\n", stat.GroupKey, stat.EventCount)
	}

	return nil
}

// showTypes выводит доступные типы событий
func showTypes(client replay.ReplayServiceClient, opts *TypesOptions) error {
	fmt.Println("📋 Available event types")

	// Парсим временные границы (опционально)
	var startTime, endTime *timestamppb.Timestamp
	if opts.Since != "" || opts.Until != "" {
		end := time.Now()
		if opts.Until != "" {
			var err error
			end, err = time.Parse(timeFormat, opts.Until)
			if err != nil {
				return fmt.Errorf("invalid until time: %v", err)
			}
		}

		start := end
		if opts.Since != "" {
			var err error
			start, err = parseSinceTime(opts.Since, end)
			if err != nil {
				return fmt.Errorf("invalid since time: %v", err)
			}
		}

		startTime = timestamppb.New(start)
		endTime = timestamppb.New(end)
	}

	// Создаем запрос
	req := &replay.EventTypesRequest{
		StartTime: startTime,
		EndTime:   endTime,
	}

	// Получаем типы событий
	types, err := client.GetEventTypes(context.Background(), req)
	if err != nil {
		return fmt.Errorf("failed to get types: %v", err)
	}

	// Выводим результаты
	for _, eventType := range types.EventTypes {
		fmt.Printf("Type: %s\n", eventType.EventType)
		fmt.Printf("  Description: %s\n", eventType.Description)
		fmt.Printf("  Count: %d\n", eventType.Count)
		fmt.Printf("  Regions: %v\n", eventType.Regions)
		fmt.Printf("  First seen: %s\n", eventType.FirstSeen.AsTime().Format(timeFormat))
		fmt.Printf("  Last seen: %s\n", eventType.LastSeen.AsTime().Format(timeFormat))
		fmt.Println()
	}

	return nil
}

// printEvent выводит событие в читаемом формате
func printEvent(event *events.EventEnvelope) {
	timestamp := event.Timestamp.AsTime().Format("15:04:05")
	fmt.Printf("[%s] %s/%s [%s] %s\n",
		timestamp,
		event.RegionId,
		event.SourceNode,
		event.EventType,
		event.EventId)

	// Добавляем детали в зависимости от типа события
	switch e := event.Event.(type) {
	case *events.EventEnvelope_WorldEvent:
		fmt.Printf("  World: %s\n", e.WorldEvent.WorldId)
	case *events.EventEnvelope_BlockEvent:
		fmt.Printf("  Block: (%d,%d,%d) Player: %s\n",
			e.BlockEvent.Coords.X, e.BlockEvent.Coords.Y, e.BlockEvent.Coords.Z,
			e.BlockEvent.PlayerId)
	case *events.EventEnvelope_ChatEvent:
		fmt.Printf("  Player: %s Channel: %s\n",
			e.ChatEvent.PlayerId, e.ChatEvent.Channel)
	case *events.EventEnvelope_SystemEvent:
		fmt.Printf("  Component: %s Level: %s\n",
			e.SystemEvent.Component, e.SystemEvent.Level)
	}
}

// parseStringList парсит строку с разделителями-запятыми
func parseStringList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// parseSinceTime парсит относительное время типа "1h", "30m", "1d"
func parseSinceTime(since string, from time.Time) (time.Time, error) {
	if since == "" {
		return from, nil
	}

	duration, err := time.ParseDuration(since)
	if err != nil {
		// Пробуем парсить как абсолютное время
		return time.Parse(timeFormat, since)
	}

	return from.Add(-duration), nil
}
