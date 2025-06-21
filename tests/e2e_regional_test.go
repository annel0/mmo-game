package tests

import (
	"context"
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/regional"
	"github.com/annel0/mmo-game/internal/sync"
	"github.com/annel0/mmo-game/internal/world"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegionalNodeE2E тестирует взаимодействие двух региональных узлов
func TestRegionalNodeE2E(t *testing.T) {
	// Создаём in-memory EventBus для тестирования
	bus := eventbus.NewMemoryBus(1000)

	// Создаём BatchManager для тестирования
	batchManager := sync.NewBatchManager(bus, "test-region", 10, 1*time.Second, nil)

	// Создаём первый региональный узел (EU-West)
	euWorld := world.NewWorldManager(12345)
	euCfg := regional.NodeConfig{
		RegionID:     "eu-west-1",
		WorldManager: euWorld,
		EventBus:     bus,
		BatchManager: batchManager,
		Resolver:     nil, // Использует LWW по умолчанию
	}
	euNode, err := regional.NewRegionalNode(euCfg)
	require.NoError(t, err)

	// Создаём второй региональный узел (US-East)
	usWorld := world.NewWorldManager(12345) // Тот же seed для одинакового мира
	usCfg := regional.NodeConfig{
		RegionID:     "us-east-1",
		WorldManager: usWorld,
		EventBus:     bus,
		BatchManager: batchManager,
		Resolver:     nil, // Использует LWW по умолчанию
	}
	usNode, err := regional.NewRegionalNode(usCfg)
	require.NoError(t, err)

	// Запускаем оба узла
	ctx := context.Background()
	require.NoError(t, euNode.Start(ctx))
	require.NoError(t, usNode.Start(ctx))

	defer func() {
		euNode.Stop()
		usNode.Stop()
	}()

	// Проверяем, что узлы запустились корректно
	assert.Equal(t, "eu-west-1", euNode.GetRegionID())
	assert.Equal(t, "us-east-1", usNode.GetRegionID())

	// Проверяем, что миры доступны
	assert.NotNil(t, euNode.GetLocalWorld())
	assert.NotNil(t, usNode.GetLocalWorld())

	// Создаём тестовое изменение
	testChange := &sync.Change{
		Data:         []byte(`{"type":"block","x":100,"y":50,"block_id":1}`),
		Priority:     5,
		Timestamp:    time.Now(),
		SourceRegion: "eu-west-1",
		ChangeType:   "BlockEvent",
	}

	// Применяем изменение к US узлу (симулируем получение удалённого изменения)
	err = usNode.ApplyRemoteChange(testChange)
	assert.NoError(t, err)

	// Проверяем, что изменение можно отправить через EU узел
	err = euNode.BroadcastLocalChange(testChange)
	assert.NoError(t, err)

	t.Logf("✅ E2E тест пройден: EU узел создал изменение, US узел его применил")
}

// TestRegionalNodeConflictResolution тестирует разрешение конфликтов
func TestRegionalNodeConflictResolution(t *testing.T) {
	resolver := regional.NewLWWResolver()

	// Создаём конфликт: два изменения для одного блока
	oldChange := &sync.Change{
		Data:         []byte(`{"type":"block","x":100,"y":50,"block_id":1}`),
		Priority:     5,
		Timestamp:    time.Now().Add(-1 * time.Minute), // Старше
		SourceRegion: "eu-west-1",
		ChangeType:   "BlockEvent",
	}

	newChange := &sync.Change{
		Data:         []byte(`{"type":"block","x":100,"y":50,"block_id":2}`),
		Priority:     5,
		Timestamp:    time.Now(), // Новее
		SourceRegion: "us-east-1",
		ChangeType:   "BlockEvent",
	}

	conflict := &regional.Conflict{
		LocalChange:  oldChange,
		RemoteChange: newChange,
		DetectedAt:   time.Now(),
	}

	// Разрешаем конфликт (должен выбрать новее изменение)
	resolved, err := resolver.Resolve(conflict)
	require.NoError(t, err)

	assert.Equal(t, newChange, resolved)
	assert.Equal(t, "us-east-1", resolved.SourceRegion)

	t.Logf("✅ Конфликт разрешён: выбрано новее изменение от %s", resolved.SourceRegion)
}

// BenchmarkRegionalNodeThroughput бенчмарк пропускной способности
func BenchmarkRegionalNodeThroughput(b *testing.B) {
	bus := eventbus.NewMemoryBus(1000)
	world := world.NewWorldManager(12345)
	batchManager := sync.NewBatchManager(bus, "benchmark-region", 10, 1*time.Second, nil)

	cfg := regional.NodeConfig{
		RegionID:     "benchmark-region",
		WorldManager: world,
		EventBus:     bus,
		BatchManager: batchManager,
		Resolver:     nil,
	}

	node, err := regional.NewRegionalNode(cfg)
	require.NoError(b, err)

	ctx := context.Background()
	require.NoError(b, node.Start(ctx))
	defer node.Stop()

	// Создаём изменения для бенчмарка
	testChange := &sync.Change{
		Data:         []byte(`{"type":"block","x":100,"y":50,"block_id":1}`),
		Priority:     5,
		Timestamp:    time.Now(),
		SourceRegion: "remote-region",
		ChangeType:   "BlockEvent",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Обновляем timestamp для каждого изменения
		testChange.Timestamp = time.Now()

		err := node.ApplyRemoteChange(testChange)
		if err != nil {
			b.Fatalf("Ошибка применения изменения: %v", err)
		}
	}

	b.Logf("Обработано %d изменений", b.N)
}
