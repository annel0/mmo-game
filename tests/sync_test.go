package tests

import (
	"testing"
	"time"

	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/sync"
)

func TestBatchManagerCompression(t *testing.T) {
	bus := eventbus.NewMemoryBus(10)
	compressor := sync.NewSmartCompressor()

	bm := sync.NewBatchManager(bus, "test-region", 5, 100*time.Millisecond, compressor)
	defer bm.Stop()

	// Добавляем несколько изменений
	for i := 0; i < 3; i++ {
		bm.AddChange(sync.Change{
			Data:     []byte("test-data-" + string(rune('0'+i))),
			Priority: 5,
		})
	}

	// Подождём flush
	time.Sleep(150 * time.Millisecond)

	// Проверяем, что событие опубликовано
	stats := bus.Metrics()
	if stats.Published == 0 {
		t.Errorf("Expected published > 0, got %d", stats.Published)
	}
}

func TestDeltaCompressor(t *testing.T) {
	changes := []sync.Change{
		{Data: []byte("change1"), Priority: 1},
		{Data: []byte("change2"), Priority: 2},
	}

	// Test passthrough
	passthrough := sync.NewPassthroughCompressor()
	compressed, err := passthrough.Compress(changes)
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	decompressed, err := passthrough.Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress error: %v", err)
	}

	if len(decompressed) != 2 {
		t.Errorf("Expected 2 changes, got %d", len(decompressed))
	}

	// Test smart compressor
	smart := sync.NewSmartCompressor()
	smartCompressed, err := smart.Compress(changes)
	if err != nil {
		t.Fatalf("Smart compress error: %v", err)
	}

	smartDecompressed, err := smart.Decompress(smartCompressed)
	if err != nil {
		t.Fatalf("Smart decompress error: %v", err)
	}

	if len(smartDecompressed) != 2 {
		t.Errorf("Expected 2 changes from smart, got %d", len(smartDecompressed))
	}
}
