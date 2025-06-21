package network

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/protocol"
)

// GameHandlerPrediction расширяет GameHandlerPB поддержкой client-side prediction
type GameHandlerPrediction struct {
	*GameHandlerPB                       // Встраиваем существующий GameHandlerPB
	predictionService *PredictionService // Сервис prediction
	mu                sync.RWMutex       // Мьютекс для потокобезопасности

	// Каналы для отправки снимков
	snapshotChannels map[uint64]chan *protocol.NetGameMessage // playerID -> channel

	// Настройки prediction
	predictionEnabled    bool
	snapshotSendInterval time.Duration // Интервал отправки снимков клиентам
	maxSnapshotQueue     int           // Максимальный размер очереди снимков
}

// NewGameHandlerPrediction создаёт новый GameHandlerPB с поддержкой prediction
func NewGameHandlerPrediction(originalHandler *GameHandlerPB, predictionService *PredictionService) *GameHandlerPrediction {
	return &GameHandlerPrediction{
		GameHandlerPB:        originalHandler,
		predictionService:    predictionService,
		snapshotChannels:     make(map[uint64]chan *protocol.NetGameMessage),
		predictionEnabled:    true,
		snapshotSendInterval: 50 * time.Millisecond, // 20Hz
		maxSnapshotQueue:     10,
	}
}

// Start запускает расширенный обработчик с prediction
func (ghp *GameHandlerPrediction) Start(ctx context.Context) error {
	// Базовый GameHandlerPB не имеет метода Start, поэтому просто запускаем prediction service

	// Запускаем prediction service
	if err := ghp.predictionService.Start(ctx); err != nil {
		return fmt.Errorf("failed to start prediction service: %w", err)
	}

	// Запускаем отправку снимков
	go ghp.snapshotSendLoop(ctx)

	return nil
}

// HandleNetGameMessage обрабатывает сообщения с поддержкой prediction
func (ghp *GameHandlerPrediction) HandleNetGameMessage(playerID uint64, msg *protocol.NetGameMessage) error {
	// Проверяем, это ли сообщение prediction
	switch msg.Payload.(type) {
	case *protocol.NetGameMessage_ClientInput:
		return ghp.handleClientInput(playerID, msg.GetClientInput())

	case *protocol.NetGameMessage_PredictionStats:
		return ghp.handlePredictionStatsRequest(playerID)

	default:
		// Обрабатываем через базовый GameHandlerPB - используем адаптер
		return ghp.adaptLegacyMessage(playerID, msg)
	}
}

// handleClientInput обрабатывает клиентский ввод
func (ghp *GameHandlerPrediction) handleClientInput(playerID uint64, input *protocol.ClientInputMessage) error {
	if !ghp.predictionEnabled {
		return fmt.Errorf("prediction disabled")
	}

	// Обрабатываем ввод через prediction service
	ack, err := ghp.predictionService.ProcessClientInput(playerID, input)
	if err != nil {
		log.Printf("Error processing client input for player %d: %v", playerID, err)
		return err
	}

	// Отправляем подтверждение обратно клиенту
	ackMsg := &protocol.NetGameMessage{
		Sequence: 0, // Будет установлено в NetChannel
		Flags:    protocol.NetFlags_RELIABLE_UNORDERED,
		Payload: &protocol.NetGameMessage_InputAck{
			InputAck: ack,
		},
	}

	return ghp.sendMessageToPlayer(playerID, ackMsg)
}

// handlePredictionStatsRequest обрабатывает запрос статистики prediction
func (ghp *GameHandlerPrediction) handlePredictionStatsRequest(playerID uint64) error {
	stats := ghp.predictionService.GetPredictionStats(playerID)
	if stats == nil {
		return fmt.Errorf("no prediction stats for player %d", playerID)
	}

	// Отправляем статистику клиенту
	statsMsg := &protocol.NetGameMessage{
		Sequence: 0,
		Flags:    protocol.NetFlags_UNRELIABLE_UNORDERED,
		Payload: &protocol.NetGameMessage_PredictionStats{
			PredictionStats: stats,
		},
	}

	return ghp.sendMessageToPlayer(playerID, statsMsg)
}

// snapshotSendLoop отправляет снимки мира всем игрокам
func (ghp *GameHandlerPrediction) snapshotSendLoop(ctx context.Context) {
	ticker := time.NewTicker(ghp.snapshotSendInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ghp.sendSnapshotsToAllPlayers()
		}
	}
}

// sendSnapshotsToAllPlayers отправляет снимки всем активным игрокам
func (ghp *GameHandlerPrediction) sendSnapshotsToAllPlayers() {
	ghp.mu.RLock()
	playerChannels := make(map[uint64]chan *protocol.NetGameMessage)
	for playerID, ch := range ghp.snapshotChannels {
		playerChannels[playerID] = ch
	}
	ghp.mu.RUnlock()

	// Отправляем снимки всем игрокам
	for playerID, ch := range playerChannels {
		snapshot := ghp.predictionService.GetLatestSnapshot(playerID)
		if snapshot == nil {
			continue
		}

		snapshotMsg := &protocol.NetGameMessage{
			Sequence: 0,
			Flags:    protocol.NetFlags_UNRELIABLE_UNORDERED, // Снимки могут теряться
			Payload: &protocol.NetGameMessage_WorldSnapshot{
				WorldSnapshot: snapshot,
			},
		}

		// Неблокирующая отправка
		select {
		case ch <- snapshotMsg:
			// Успешно отправлено
		default:
			// Канал переполнен, пропускаем снимок
			log.Printf("Snapshot queue full for player %d, dropping snapshot", playerID)
		}
	}
}

// RegisterPlayer регистрирует игрока для получения снимков
func (ghp *GameHandlerPrediction) RegisterPlayer(playerID uint64) {
	ghp.mu.Lock()
	defer ghp.mu.Unlock()

	// Создаём канал для снимков игрока
	ch := make(chan *protocol.NetGameMessage, ghp.maxSnapshotQueue)
	ghp.snapshotChannels[playerID] = ch

	// Запускаем горутину для отправки снимков игроку
	go ghp.playerSnapshotSender(playerID, ch)
}

// UnregisterPlayer удаляет игрока из системы prediction
func (ghp *GameHandlerPrediction) UnregisterPlayer(playerID uint64) {
	ghp.mu.Lock()
	defer ghp.mu.Unlock()

	// Закрываем канал снимков
	if ch, exists := ghp.snapshotChannels[playerID]; exists {
		close(ch)
		delete(ghp.snapshotChannels, playerID)
	}

	// Удаляем состояние из prediction service
	ghp.predictionService.RemovePlayer(playerID)
}

// playerSnapshotSender отправляет снимки конкретному игроку
func (ghp *GameHandlerPrediction) playerSnapshotSender(playerID uint64, snapshotCh <-chan *protocol.NetGameMessage) {
	for msg := range snapshotCh {
		if err := ghp.sendMessageToPlayer(playerID, msg); err != nil {
			log.Printf("Failed to send snapshot to player %d: %v", playerID, err)
		}
	}
}

// sendMessageToPlayer отправляет сообщение игроку
func (ghp *GameHandlerPrediction) sendMessageToPlayer(playerID uint64, msg *protocol.NetGameMessage) error {
	// Здесь используем существующую логику отправки из GameHandlerPB
	// Это может быть через ChannelManager или прямую отправку через NetChannel

	// Заглушка - в реальной реализации нужно использовать существующую инфраструктуру
	log.Printf("Sending message to player %d: %T", playerID, msg.Payload)
	return nil
}

// adaptLegacyMessage адаптирует NetGameMessage для обработки через базовый GameHandlerPB
func (ghp *GameHandlerPrediction) adaptLegacyMessage(playerID uint64, msg *protocol.NetGameMessage) error {
	// Пока заглушка - в реальной реализации нужно конвертировать NetGameMessage в GameMessage
	// и вызывать соответствующие методы GameHandlerPB
	log.Printf("Adapting legacy message for player %d: %T", playerID, msg.Payload)
	return nil
}

// SetPredictionEnabled включает/выключает prediction
func (ghp *GameHandlerPrediction) SetPredictionEnabled(enabled bool) {
	ghp.mu.Lock()
	defer ghp.mu.Unlock()
	ghp.predictionEnabled = enabled
}

// IsPredictionEnabled возвращает статус prediction
func (ghp *GameHandlerPrediction) IsPredictionEnabled() bool {
	ghp.mu.RLock()
	defer ghp.mu.RUnlock()
	return ghp.predictionEnabled
}

// SetSnapshotRate устанавливает частоту отправки снимков
func (ghp *GameHandlerPrediction) SetSnapshotRate(interval time.Duration) {
	ghp.mu.Lock()
	defer ghp.mu.Unlock()
	ghp.snapshotSendInterval = interval

	// Также обновляем частоту в prediction service
	ghp.predictionService.SetSnapshotRate(interval)
}

// GetActivePlayersCount возвращает количество активных игроков в prediction
func (ghp *GameHandlerPrediction) GetActivePlayersCount() int {
	ghp.mu.RLock()
	defer ghp.mu.RUnlock()
	return len(ghp.snapshotChannels)
}

// Stop останавливает GameHandler с prediction
func (ghp *GameHandlerPrediction) Stop() error {
	// Останавливаем prediction service
	if err := ghp.predictionService.Stop(); err != nil {
		log.Printf("Error stopping prediction service: %v", err)
	}

	// Закрываем все каналы снимков
	ghp.mu.Lock()
	for playerID, ch := range ghp.snapshotChannels {
		close(ch)
		delete(ghp.snapshotChannels, playerID)
	}
	ghp.mu.Unlock()

	// Базовый GameHandlerPB не имеет метода Stop
	return nil
}
