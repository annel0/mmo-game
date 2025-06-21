package network

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world"
)

// PredictionService обрабатывает client-side prediction
type PredictionService struct {
	mu                sync.RWMutex
	worldManager      *world.WorldManager
	playerStates      map[uint64]*PlayerPredictionState // playerID -> state
	snapshotRate      time.Duration                     // Частота снимков (по умолчанию 50ms = 20Hz)
	maxInputBuffer    int                               // Максимальный размер буфера вводов
	maxSnapshotBuffer int                               // Максимальный размер буфера снимков
	running           bool
	ticker            *time.Ticker
	stopCh            chan struct{}
}

// PlayerPredictionState содержит состояние prediction для игрока
type PlayerPredictionState struct {
	PlayerID           uint64
	LastProcessedInput uint32
	InputBuffer        []*protocol.ClientInputMessage   // Буфер необработанных вводов
	SnapshotBuffer     []*protocol.WorldSnapshotMessage // Буфер снимков
	LastSnapshotID     uint32
	LastPosition       *protocol.Vec2
	LastVelocity       *protocol.Vec2Float
	PredictionErrors   []float32 // История ошибок prediction для статистики

	// Метрики
	TotalInputs        uint64
	TotalCorrections   uint64
	AvgPredictionError float32
	MaxPredictionError float32
}

// NewPredictionService создаёт новый сервис prediction
func NewPredictionService(worldManager *world.WorldManager) *PredictionService {
	return &PredictionService{
		worldManager:      worldManager,
		playerStates:      make(map[uint64]*PlayerPredictionState),
		snapshotRate:      50 * time.Millisecond, // 20Hz
		maxInputBuffer:    64,                    // Максимум 64 необработанных ввода
		maxSnapshotBuffer: 32,                    // Максимум 32 снимка в истории
		stopCh:            make(chan struct{}),
	}
}

// Start запускает сервис prediction
func (ps *PredictionService) Start(ctx context.Context) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if ps.running {
		return fmt.Errorf("prediction service already running")
	}

	ps.running = true
	ps.ticker = time.NewTicker(ps.snapshotRate)

	go ps.snapshotLoop(ctx)

	return nil
}

// Stop останавливает сервис prediction
func (ps *PredictionService) Stop() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if !ps.running {
		return nil
	}

	ps.running = false
	if ps.ticker != nil {
		ps.ticker.Stop()
	}
	close(ps.stopCh)

	return nil
}

// ProcessClientInput обрабатывает ввод от клиента
func (ps *PredictionService) ProcessClientInput(playerID uint64, input *protocol.ClientInputMessage) (*protocol.InputAckMessage, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Получаем или создаём состояние игрока
	state, exists := ps.playerStates[playerID]
	if !exists {
		state = &PlayerPredictionState{
			PlayerID:         playerID,
			InputBuffer:      make([]*protocol.ClientInputMessage, 0, ps.maxInputBuffer),
			SnapshotBuffer:   make([]*protocol.WorldSnapshotMessage, 0, ps.maxSnapshotBuffer),
			PredictionErrors: make([]float32, 0, 100),
		}
		ps.playerStates[playerID] = state
	}

	// Добавляем ввод в буфер
	if len(state.InputBuffer) >= ps.maxInputBuffer {
		// Удаляем старый ввод
		state.InputBuffer = state.InputBuffer[1:]
	}
	state.InputBuffer = append(state.InputBuffer, input)
	state.TotalInputs++

	// Обрабатываем ввод в мире
	ack := &protocol.InputAckMessage{
		InputId:   input.InputId,
		Processed: true,
	}

	// Применяем ввод в игровой мир
	err := ps.applyInputToWorld(playerID, input)
	if err != nil {
		ack.Processed = false
		errStr := err.Error()
		ack.Error = &errStr
		return ack, nil
	}

	// Обновляем последний обработанный ввод
	state.LastProcessedInput = input.InputId

	return ack, nil
}

// applyInputToWorld применяет клиентский ввод к игровому миру
func (ps *PredictionService) applyInputToWorld(playerID uint64, input *protocol.ClientInputMessage) error {
	// Заглушка для работы с игроками - пока используем простую логику
	// В будущем здесь будет интеграция с системой игроков в world

	switch input.InputType {
	case protocol.ClientInputType_INPUT_MOVE:
		if input.MoveDirection != nil {
			// Конвертируем координаты из protocol в vec
			movePos := vec.Vec2{
				X: int(input.MoveDirection.X),
				Y: int(input.MoveDirection.Y),
			}

			var velocity vec.Vec2Float
			if input.PredictedVelocity != nil {
				velocity = vec.Vec2Float{
					X: float64(input.PredictedVelocity.X),
					Y: float64(input.PredictedVelocity.Y),
				}
			}

			// Применяем движение через world manager
			return ps.worldManager.MoveEntity(playerID, movePos, velocity)
		}

	case protocol.ClientInputType_INPUT_INTERACT:
		// Обработка взаимодействия - пока заглушка
		return nil

	case protocol.ClientInputType_INPUT_ATTACK:
		// Обработка атаки - пока заглушка
		return nil

	case protocol.ClientInputType_INPUT_BUILD:
		// Обработка строительства - пока заглушка
		return nil
	}

	return nil
}

// snapshotLoop генерирует снимки мира для всех игроков
func (ps *PredictionService) snapshotLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ps.stopCh:
			return
		case <-ps.ticker.C:
			ps.generateSnapshots()
		}
	}
}

// generateSnapshots создаёт снимки для всех активных игроков
func (ps *PredictionService) generateSnapshots() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now().UnixMilli()

	for playerID, state := range ps.playerStates {
		snapshot := ps.createSnapshotForPlayer(playerID, state, now)
		if snapshot != nil {
			ps.addSnapshotToBuffer(state, snapshot)
		}
	}
}

// createSnapshotForPlayer создаёт снимок мира для конкретного игрока
func (ps *PredictionService) createSnapshotForPlayer(playerID uint64, state *PlayerPredictionState, timestamp int64) *protocol.WorldSnapshotMessage {
	// Пока используем заглушку для создания снимков
	// В будущем здесь будет полная интеграция с world manager

	state.LastSnapshotID++

	// Создаём базовый снимок
	snapshot := &protocol.WorldSnapshotMessage{
		SnapshotId:         state.LastSnapshotID,
		ServerTimestamp:    timestamp,
		LastProcessedInput: state.LastProcessedInput,
		PlayerState: &protocol.EntityData{
			Id:       playerID,
			Type:     protocol.EntityType_ENTITY_PLAYER,
			Position: &protocol.Vec2{X: 0, Y: 0},      // Заглушка
			Velocity: &protocol.Vec2Float{X: 0, Y: 0}, // Заглушка
			Active:   true,
		},
	}

	// Пустой список видимых сущностей (заглушка)
	snapshot.VisibleEntities = make([]*protocol.EntityData, 0)

	// Добавляем метаданные
	totalEntities := int32(1) // Заглушка
	snapshot.EntitiesTotal = &totalEntities

	tickrate := float32(1000.0 / ps.snapshotRate.Milliseconds())
	snapshot.ServerTickrate = &tickrate

	return snapshot
}

// addSnapshotToBuffer добавляет снимок в буфер игрока
func (ps *PredictionService) addSnapshotToBuffer(state *PlayerPredictionState, snapshot *protocol.WorldSnapshotMessage) {
	if len(state.SnapshotBuffer) >= ps.maxSnapshotBuffer {
		// Удаляем старый снимок
		state.SnapshotBuffer = state.SnapshotBuffer[1:]
	}
	state.SnapshotBuffer = append(state.SnapshotBuffer, snapshot)
}

// GetLatestSnapshot возвращает последний снимок для игрока
func (ps *PredictionService) GetLatestSnapshot(playerID uint64) *protocol.WorldSnapshotMessage {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	state, exists := ps.playerStates[playerID]
	if !exists || len(state.SnapshotBuffer) == 0 {
		return nil
	}

	return state.SnapshotBuffer[len(state.SnapshotBuffer)-1]
}

// GetPredictionStats возвращает статистику prediction для игрока
func (ps *PredictionService) GetPredictionStats(playerID uint64) *protocol.PredictionStatsMessage {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	state, exists := ps.playerStates[playerID]
	if !exists {
		return nil
	}

	return &protocol.PredictionStatsMessage{
		InputsPending:       uint32(len(state.InputBuffer)),
		SnapshotsBuffered:   uint32(len(state.SnapshotBuffer)),
		AvgPredictionError:  state.AvgPredictionError,
		MaxPredictionError:  state.MaxPredictionError,
		ReconciliationCount: int32(state.TotalCorrections),
	}
}

// RemovePlayer удаляет состояние игрока при отключении
func (ps *PredictionService) RemovePlayer(playerID uint64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	delete(ps.playerStates, playerID)
}

// SetSnapshotRate устанавливает частоту снимков
func (ps *PredictionService) SetSnapshotRate(rate time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.snapshotRate = rate
	if ps.ticker != nil {
		ps.ticker.Stop()
		ps.ticker = time.NewTicker(rate)
	}
}
