package sync

import (
	"time"

	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/logging"
)

// SyncManager координирует работу всех компонентов синхронизации:
// BatchManager, SyncProducer, SyncConsumer.

type SyncManager struct {
	bm       *BatchManager
	producer *SyncProducer
	consumer *SyncConsumer
}

type SyncConfig struct {
	RegionID     string
	Bus          eventbus.EventBus
	BatchSize    int
	FlushEvery   time.Duration
	UseGzipCompr bool
}

func NewSyncManager(cfg SyncConfig) (*SyncManager, error) {
	var compressor DeltaCompressor
	if cfg.UseGzipCompr {
		compressor = NewSmartCompressor()
		logging.Info("🔄 SyncManager: используется gzip-компрессия")
	} else {
		compressor = NewPassthroughCompressor()
		logging.Info("🔄 SyncManager: компрессия отключена")
	}

	bm := NewBatchManager(cfg.Bus, cfg.RegionID, cfg.BatchSize, cfg.FlushEvery, compressor)
	producer, err := NewSyncProducer(cfg.Bus, bm)
	if err != nil {
		return nil, err
	}

	consumer, err := NewSyncConsumer(cfg.Bus, compressor)
	if err != nil {
		producer.Stop()
		return nil, err
	}

	logging.Info("✅ SyncManager инициализирован: region=%s, batch=%d, flush=%v",
		cfg.RegionID, cfg.BatchSize, cfg.FlushEvery)

	return &SyncManager{
		bm:       bm,
		producer: producer,
		consumer: consumer,
	}, nil
}

func (sm *SyncManager) Stop() {
	sm.producer.Stop()
	sm.consumer.Stop()
	sm.bm.Stop()
	logging.Info("🔄 SyncManager остановлен")
}
