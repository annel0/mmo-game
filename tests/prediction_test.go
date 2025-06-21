package tests

import (
	"context"
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/network"
	"github.com/annel0/mmo-game/internal/protocol"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPredictionService_Basic(t *testing.T) {
	// Создаём тестовый мир
	worldManager := world.NewWorldManager(12345)

	// Создаём сервис prediction
	predictionService := network.NewPredictionService(worldManager)
	require.NotNil(t, predictionService)

	// Запускаем сервис
	ctx := context.Background()
	err := predictionService.Start(ctx)
	require.NoError(t, err)

	defer predictionService.Stop()

	// Тестируем обработку ввода
	playerID := uint64(1)
	input := &protocol.ClientInputMessage{
		InputId:           1,
		ClientTimestamp:   time.Now().UnixMilli(),
		InputType:         protocol.ClientInputType_INPUT_MOVE,
		MoveDirection:     &protocol.Vec2{X: 1, Y: 0},
		PredictedPosition: &protocol.Vec2{X: 10, Y: 10},
		PredictedVelocity: &protocol.Vec2Float{X: 5.0, Y: 0.0},
	}

	ack, err := predictionService.ProcessClientInput(playerID, input)
	require.NoError(t, err)
	require.NotNil(t, ack)
	assert.Equal(t, uint32(1), ack.InputId)
	assert.True(t, ack.Processed)
}

func TestPredictionService_Snapshots(t *testing.T) {
	// Создаём тестовый мир
	worldManager := world.NewWorldManager(12345)

	// Создаём сервис prediction
	predictionService := network.NewPredictionService(worldManager)
	require.NotNil(t, predictionService)

	// Запускаем сервис
	ctx := context.Background()
	err := predictionService.Start(ctx)
	require.NoError(t, err)

	defer predictionService.Stop()

	playerID := uint64(1)

	// Обрабатываем несколько вводов
	for i := 1; i <= 3; i++ {
		input := &protocol.ClientInputMessage{
			InputId:           uint32(i),
			ClientTimestamp:   time.Now().UnixMilli(),
			InputType:         protocol.ClientInputType_INPUT_MOVE,
			MoveDirection:     &protocol.Vec2{X: int32(i), Y: 0},
			PredictedPosition: &protocol.Vec2{X: int32(i * 10), Y: 10},
		}

		_, err := predictionService.ProcessClientInput(playerID, input)
		require.NoError(t, err)
	}

	// Ждём генерации снимка
	time.Sleep(100 * time.Millisecond)

	// Получаем снимок
	snapshot := predictionService.GetLatestSnapshot(playerID)
	require.NotNil(t, snapshot)
	assert.Greater(t, snapshot.SnapshotId, uint32(0))
	assert.Equal(t, uint32(3), snapshot.LastProcessedInput)
}

func TestPredictionService_Stats(t *testing.T) {
	// Создаём тестовый мир
	worldManager := world.NewWorldManager(12345)

	// Создаём сервис prediction
	predictionService := network.NewPredictionService(worldManager)
	require.NotNil(t, predictionService)

	// Запускаем сервис
	ctx := context.Background()
	err := predictionService.Start(ctx)
	require.NoError(t, err)

	defer predictionService.Stop()

	playerID := uint64(1)

	// Обрабатываем ввод
	input := &protocol.ClientInputMessage{
		InputId:         1,
		ClientTimestamp: time.Now().UnixMilli(),
		InputType:       protocol.ClientInputType_INPUT_MOVE,
		MoveDirection:   &protocol.Vec2{X: 1, Y: 0},
	}

	_, err = predictionService.ProcessClientInput(playerID, input)
	require.NoError(t, err)

	// Получаем статистику
	stats := predictionService.GetPredictionStats(playerID)
	require.NotNil(t, stats)
	assert.Greater(t, stats.InputsPending, uint32(0))
}

func TestPredictionService_MultipleInputTypes(t *testing.T) {
	// Создаём тестовый мир
	worldManager := world.NewWorldManager(12345)

	// Создаём сервис prediction
	predictionService := network.NewPredictionService(worldManager)
	require.NotNil(t, predictionService)

	// Запускаем сервис
	ctx := context.Background()
	err := predictionService.Start(ctx)
	require.NoError(t, err)

	defer predictionService.Stop()

	playerID := uint64(1)

	// Тестируем разные типы вводов
	testCases := []struct {
		name      string
		inputType protocol.ClientInputType
	}{
		{"Move", protocol.ClientInputType_INPUT_MOVE},
		{"Interact", protocol.ClientInputType_INPUT_INTERACT},
		{"Attack", protocol.ClientInputType_INPUT_ATTACK},
		{"Build", protocol.ClientInputType_INPUT_BUILD},
	}

	for i, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := &protocol.ClientInputMessage{
				InputId:         uint32(i + 1),
				ClientTimestamp: time.Now().UnixMilli(),
				InputType:       tc.inputType,
			}

			if tc.inputType == protocol.ClientInputType_INPUT_MOVE {
				input.MoveDirection = &protocol.Vec2{X: 1, Y: 0}
			}

			ack, err := predictionService.ProcessClientInput(playerID, input)
			require.NoError(t, err)
			require.NotNil(t, ack)
			assert.Equal(t, uint32(i+1), ack.InputId)
			assert.True(t, ack.Processed)
		})
	}
}

func TestPredictionService_PlayerRemoval(t *testing.T) {
	// Создаём тестовый мир
	worldManager := world.NewWorldManager(12345)

	// Создаём сервис prediction
	predictionService := network.NewPredictionService(worldManager)
	require.NotNil(t, predictionService)

	// Запускаем сервис
	ctx := context.Background()
	err := predictionService.Start(ctx)
	require.NoError(t, err)

	defer predictionService.Stop()

	playerID := uint64(1)

	// Обрабатываем ввод
	input := &protocol.ClientInputMessage{
		InputId:         1,
		ClientTimestamp: time.Now().UnixMilli(),
		InputType:       protocol.ClientInputType_INPUT_MOVE,
		MoveDirection:   &protocol.Vec2{X: 1, Y: 0},
	}

	_, err = predictionService.ProcessClientInput(playerID, input)
	require.NoError(t, err)

	// Проверяем, что статистика доступна
	stats := predictionService.GetPredictionStats(playerID)
	require.NotNil(t, stats)

	// Удаляем игрока
	predictionService.RemovePlayer(playerID)

	// Проверяем, что статистика больше недоступна
	stats = predictionService.GetPredictionStats(playerID)
	assert.Nil(t, stats)
}

func TestPredictionService_SnapshotRate(t *testing.T) {
	// Создаём тестовый мир
	worldManager := world.NewWorldManager(12345)

	// Создаём сервис prediction
	predictionService := network.NewPredictionService(worldManager)
	require.NotNil(t, predictionService)

	// Устанавливаем высокую частоту снимков для тестирования
	predictionService.SetSnapshotRate(10 * time.Millisecond)

	// Запускаем сервис
	ctx := context.Background()
	err := predictionService.Start(ctx)
	require.NoError(t, err)

	defer predictionService.Stop()

	playerID := uint64(1)

	// Обрабатываем ввод
	input := &protocol.ClientInputMessage{
		InputId:         1,
		ClientTimestamp: time.Now().UnixMilli(),
		InputType:       protocol.ClientInputType_INPUT_MOVE,
		MoveDirection:   &protocol.Vec2{X: 1, Y: 0},
	}

	_, err = predictionService.ProcessClientInput(playerID, input)
	require.NoError(t, err)

	// Ждём несколько снимков
	time.Sleep(50 * time.Millisecond)

	// Получаем снимок
	snapshot := predictionService.GetLatestSnapshot(playerID)
	require.NotNil(t, snapshot)
	assert.Greater(t, snapshot.SnapshotId, uint32(0))
}

// Benchmark тесты для производительности

func BenchmarkPredictionService_ProcessInput(b *testing.B) {
	worldManager := world.NewWorldManager(12345)
	predictionService := network.NewPredictionService(worldManager)

	ctx := context.Background()
	predictionService.Start(ctx)
	defer predictionService.Stop()

	playerID := uint64(1)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		input := &protocol.ClientInputMessage{
			InputId:         uint32(i + 1),
			ClientTimestamp: time.Now().UnixMilli(),
			InputType:       protocol.ClientInputType_INPUT_MOVE,
			MoveDirection:   &protocol.Vec2{X: 1, Y: 0},
		}

		predictionService.ProcessClientInput(playerID, input)
	}
}

func BenchmarkPredictionService_GetSnapshot(b *testing.B) {
	worldManager := world.NewWorldManager(12345)
	predictionService := network.NewPredictionService(worldManager)

	ctx := context.Background()
	predictionService.Start(ctx)
	defer predictionService.Stop()

	playerID := uint64(1)

	// Предварительно создаём состояние игрока
	input := &protocol.ClientInputMessage{
		InputId:         1,
		ClientTimestamp: time.Now().UnixMilli(),
		InputType:       protocol.ClientInputType_INPUT_MOVE,
		MoveDirection:   &protocol.Vec2{X: 1, Y: 0},
	}
	predictionService.ProcessClientInput(playerID, input)

	time.Sleep(10 * time.Millisecond) // Ждём генерации снимка

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		predictionService.GetLatestSnapshot(playerID)
	}
}

func BenchmarkPredictionService_Stats(b *testing.B) {
	worldManager := world.NewWorldManager(12345)
	predictionService := network.NewPredictionService(worldManager)

	ctx := context.Background()
	predictionService.Start(ctx)
	defer predictionService.Stop()

	playerID := uint64(1)

	// Предварительно создаём состояние игрока
	input := &protocol.ClientInputMessage{
		InputId:         1,
		ClientTimestamp: time.Now().UnixMilli(),
		InputType:       protocol.ClientInputType_INPUT_MOVE,
		MoveDirection:   &protocol.Vec2{X: 1, Y: 0},
	}
	predictionService.ProcessClientInput(playerID, input)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		predictionService.GetPredictionStats(playerID)
	}
}
