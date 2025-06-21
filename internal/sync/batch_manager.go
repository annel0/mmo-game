package sync

import (
	"context"
	"sync"
	"time"

	"github.com/annel0/mmo-game/internal/eventbus"
	"github.com/annel0/mmo-game/internal/logging"
)

// Change содержит сериализованное изменение состояния (protobuf/json/avro).
// Тип определяется полем EventType в Envelope, поэтому здесь просто []byte.

type Change struct {
	Data         []byte    // Сериализованные данные изменения
	Priority     int       // приоритизация для сброса при перегрузке
	Timestamp    time.Time // Время создания изменения
	SourceRegion string    // Регион-источник изменения
	ChangeType   string    // Тип изменения: "BlockEvent", "EntityEvent"
}

// BatchManager накапливает изменения и отправляет их пакетами через EventBus.
// Каждый региональный узел имеет собственный экземпляр.

type BatchManager struct {
	mu       sync.Mutex
	buf      []Change
	capacity int

	flushEvery time.Duration
	bus        eventbus.EventBus
	source     string // имя текущего узла/region-id
	compressor DeltaCompressor

	quit chan struct{}
}

// NewBatchManager создаёт менеджер с указанным лимитом буфера и интервалом отправки.
func NewBatchManager(bus eventbus.EventBus, source string, capacity int, flushEvery time.Duration, compressor DeltaCompressor) *BatchManager {
	if compressor == nil {
		compressor = NewPassthroughCompressor()
	}
	bm := &BatchManager{
		capacity:   capacity,
		flushEvery: flushEvery,
		bus:        bus,
		source:     source,
		compressor: compressor,
		quit:       make(chan struct{}),
	}
	go bm.loop()
	return bm
}

// AddChange добавляет изменение в буфер; при переполнении низкоприоритетные изменения отбрасываются.
func (bm *BatchManager) AddChange(ch Change) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if len(bm.buf) >= bm.capacity {
		// ищем самое низкое Priority и заменяем, если новый выше.
		lowIdx := -1
		lowPri := ch.Priority
		for i, c := range bm.buf {
			if c.Priority < lowPri {
				lowPri = c.Priority
				lowIdx = i
			}
		}
		if lowIdx >= 0 {
			bm.buf[lowIdx] = ch
		} else {
			// все изменения >= чем новый — дропаём новый
			return
		}
	} else {
		bm.buf = append(bm.buf, ch)
	}
}

func (bm *BatchManager) loop() {
	ticker := time.NewTicker(bm.flushEvery)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bm.flush()
		case <-bm.quit:
			return
		}
	}
}

// flush отсылает накопленные изменения единым сообщением.
func (bm *BatchManager) flush() {
	bm.mu.Lock()
	if len(bm.buf) == 0 {
		bm.mu.Unlock()
		return
	}
	// компрессия через DeltaCompressor
	changes := make([]Change, len(bm.buf))
	copy(changes, bm.buf)
	bm.buf = bm.buf[:0]
	bm.mu.Unlock()

	batchPayload, err := bm.compressor.Compress(changes)
	if err != nil {
		logging.Warn("BatchManager compress error: %v", err)
		return
	}

	env := &eventbus.Envelope{
		ID:        time.Now().Format("20060102150405.000000000"),
		Timestamp: time.Now().UTC(),
		Source:    bm.source,
		EventType: "SyncBatch",
		Version:   1,
		Priority:  5,
		Payload:   batchPayload,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := bm.bus.Publish(ctx, env); err != nil {
		logging.Warn("BatchManager publish error: %v", err)
	}
}

// Stop завершает работу менеджера и отправляет оставшиеся изменения.
func (bm *BatchManager) Stop() {
	close(bm.quit)
	bm.flush()
}
