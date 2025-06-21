package replay

import (
	"context"
	"fmt"
	"log"

	"github.com/annel0/mmo-game/internal/protocol/replay"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ReplayServiceImpl реализация gRPC ReplayService
type ReplayServiceImpl struct {
	replay.UnimplementedReplayServiceServer
	eventStore EventStore
}

// NewReplayService создает новый ReplayService
func NewReplayService(eventStore EventStore) *ReplayServiceImpl {
	return &ReplayServiceImpl{
		eventStore: eventStore,
	}
}

// Replay воспроизводит события по фильтрам
func (s *ReplayServiceImpl) Replay(req *replay.ReplayRequest, stream replay.ReplayService_ReplayServer) error {
	log.Printf("🎬 Replay request: types=%v, regions=%v, limit=%d",
		req.EventTypes, req.RegionIds, req.Limit)

	// Валидация запроса
	if err := s.validateReplayRequest(req); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Получаем канал событий
	eventChan, errChan := s.eventStore.StreamEvents(stream.Context(), req)

	// Стримим события клиенту
	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()

		case event, ok := <-eventChan:
			if !ok {
				// Канал закрыт, завершаем стрим
				return nil
			}

			if err := stream.Send(event); err != nil {
				return status.Errorf(codes.Internal, "failed to send event: %v", err)
			}

		case err, ok := <-errChan:
			if !ok {
				// Канал ошибок закрыт
				continue
			}

			if err != nil {
				return status.Errorf(codes.Internal, "event store error: %v", err)
			}
		}
	}
}

// GetEventStats возвращает статистику событий
func (s *ReplayServiceImpl) GetEventStats(ctx context.Context, req *replay.EventStatsRequest) (*replay.EventStatsResponse, error) {
	log.Printf("📊 EventStats request: types=%v, regions=%v, group_by=%v",
		req.EventTypes, req.RegionIds, req.GroupBy)

	// Валидация запроса
	if err := s.validateEventStatsRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Получаем статистику из хранилища
	stats, err := s.eventStore.GetEventStats(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get event stats: %v", err)
	}

	return stats, nil
}

// GetEventTypes возвращает доступные типы событий
func (s *ReplayServiceImpl) GetEventTypes(ctx context.Context, req *replay.EventTypesRequest) (*replay.EventTypesResponse, error) {
	log.Printf("📋 EventTypes request: start=%v, end=%v",
		req.StartTime, req.EndTime)

	// Валидация запроса
	if err := s.validateEventTypesRequest(req); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Получаем типы событий из хранилища
	types, err := s.eventStore.GetEventTypes(ctx, req)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get event types: %v", err)
	}

	return types, nil
}

// validateReplayRequest валидирует запрос на воспроизведение
func (s *ReplayServiceImpl) validateReplayRequest(req *replay.ReplayRequest) error {
	if req.StartTime == nil {
		return fmt.Errorf("start_time is required")
	}

	if req.EndTime == nil {
		return fmt.Errorf("end_time is required")
	}

	if req.StartTime.AsTime().After(req.EndTime.AsTime()) {
		return fmt.Errorf("start_time must be before end_time")
	}

	if req.Limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}

	// Ограничиваем максимальное количество событий
	if req.Limit > 10000 {
		return fmt.Errorf("limit cannot exceed 10000")
	}

	return nil
}

// validateEventStatsRequest валидирует запрос статистики
func (s *ReplayServiceImpl) validateEventStatsRequest(req *replay.EventStatsRequest) error {
	if req.StartTime == nil {
		return fmt.Errorf("start_time is required")
	}

	if req.EndTime == nil {
		return fmt.Errorf("end_time is required")
	}

	if req.StartTime.AsTime().After(req.EndTime.AsTime()) {
		return fmt.Errorf("start_time must be before end_time")
	}

	return nil
}

// validateEventTypesRequest валидирует запрос типов событий
func (s *ReplayServiceImpl) validateEventTypesRequest(req *replay.EventTypesRequest) error {
	if req.StartTime != nil && req.EndTime != nil {
		if req.StartTime.AsTime().After(req.EndTime.AsTime()) {
			return fmt.Errorf("start_time must be before end_time")
		}
	}

	return nil
}
