package sync

import (
	"context"
	"fmt"

	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/logging"
)

// SyncConsumer слушает SyncBatch сообщения других регионов и применяет изменения.
// Пока только выводит размер батча.

type SyncConsumer struct {
	sub        eventbus.Subscription
	compressor DeltaCompressor
}

func NewSyncConsumer(bus eventbus.EventBus, compressor DeltaCompressor) (*SyncConsumer, error) {
	if compressor == nil {
		compressor = NewPassthroughCompressor()
	}
	sc := &SyncConsumer{compressor: compressor}
	sub, err := bus.Subscribe(context.Background(), eventbus.Filter{Types: []string{"SyncBatch"}}, sc.handle)
	if err != nil {
		return nil, err
	}
	sc.sub = sub
	return sc, nil
}

func (sc *SyncConsumer) handle(ctx context.Context, ev *eventbus.Envelope) {
	logging.Debug("SyncConsumer: batch size=%d bytes from %s", len(ev.Payload), ev.Source)

	changes, err := sc.compressor.Decompress(ev.Payload)
	if err != nil {
		logging.Warn("SyncConsumer decompress error: %v", err)
		return
	}

	logging.Debug("SyncConsumer: decoded %d changes", len(changes))

	// Применяем изменения к состоянию мира
	for i, ch := range changes {
		logging.Debug("  change[%d]: %d bytes from %s at %v", i, len(ch.Data), ch.SourceRegion, ch.Timestamp)

		// В реальной реализации здесь будет:
		// 1. Валидация изменения
		// 2. Проверка на конфликты
		// 3. Применение к локальному состоянию мира
		// 4. Уведомление подключенных клиентов

		if err := sc.applyChange(&ch); err != nil {
			logging.Warn("SyncConsumer: ошибка применения изменения %d: %v", i, err)
		}
	}
}

// applyChange применяет отдельное изменение
func (sc *SyncConsumer) applyChange(change *Change) error {
	// Базовая валидация
	if change == nil {
		return fmt.Errorf("change is nil")
	}

	if len(change.Data) == 0 {
		return fmt.Errorf("change data is empty")
	}

	// Проверяем временную метку
	if change.Timestamp.IsZero() {
		logging.Warn("SyncConsumer: изменение без временной метки")
	}

	// Логируем применение
	logging.Debug("SyncConsumer: применение изменения от %s, размер=%d байт",
		change.SourceRegion, len(change.Data))

	// В реальной реализации здесь будет:
	// 1. Декодирование типа изменения из change.Data
	// 2. Применение к WorldManager
	// 3. Обновление кеша
	// 4. Уведомление клиентов через WebSocket/gRPC

	return nil
}

func (sc *SyncConsumer) Stop() { sc.sub.Unsubscribe() }
