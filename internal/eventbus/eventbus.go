package eventbus

import (
	"context"
	"sync"
	"time"
)

// Envelope описывает универсальный контейнер события.
// Все поля фиксированы для версиирования и трассировки.
type Envelope struct {
	ID            string            // Глобально уникальный идентификатор (UUID).
	Timestamp     time.Time         // Время создания события (UTC).
	Source        string            // Имя сервиса-источника.
	EventType     string            // Тип события (BlockEvent, ChatEvent…).
	Version       int               // Схема полезной нагрузки.
	CorrelationID string            // Для связывания цепочек.
	Tenant        string            // Для мульти-тенанности (пока пусто).
	Priority      int               // 0=Low … 9=Critical (для backpressure).
	Payload       []byte            // Сериализованный protobuf/avro.
	Metadata      map[string]string // Произвольные метаданные.
}

// Filter позволяет подписаться только на нужные события.
type Filter struct {
	Types   []string // Если пусто — все типы.
	Sources []string // Если пусто — все источники.
}

// Subscription возвращается при подписке; позволяет отписаться.
type Subscription interface {
	Unsubscribe()
}

// Handler потребляет события.
type Handler func(ctx context.Context, ev *Envelope)

// Stats агрегированные метрики шины.
type Stats struct {
	Published uint64
	Consumed  uint64
	Dropped   uint64
	InFlight  int
}

// EventBus определяет абстракцию шины событий.
// В дальнейшем может иметь разные реализации (JetStream, Kafka, Redis).
type EventBus interface {
	Publish(ctx context.Context, ev *Envelope) error
	Subscribe(ctx context.Context, f Filter, h Handler) (Subscription, error)
	Metrics() Stats
}

//================ In-Memory implementation =================//

type memoryBus struct {
	mu          sync.RWMutex
	subscribers map[int]subscriber
	nextID      int
	stats       Stats
	buffer      chan *Envelope
	capacity    int
}

type subscriber struct {
	filter  Filter
	handler Handler
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewMemoryBus создаёт in-memory Bus с указанным буфером.
func NewMemoryBus(capacity int) EventBus {
	mb := &memoryBus{
		subscribers: make(map[int]subscriber),
		buffer:      make(chan *Envelope, capacity),
		capacity:    capacity,
	}
	go mb.dispatchLoop()
	return mb
}

func (mb *memoryBus) Publish(ctx context.Context, ev *Envelope) error {
	select {
	case mb.buffer <- ev:
		mb.mu.Lock()
		mb.stats.Published++
		mb.mu.Unlock()
		return nil
	default:
		// Буфер заполнен — дропаём низкий приоритет (<5)
		if ev.Priority < 5 {
			mb.mu.Lock()
			mb.stats.Dropped++
			mb.mu.Unlock()
			return nil
		}
		// Для High-priority блокируем до освобождения места или отмены контекста
		select {
		case mb.buffer <- ev:
			mb.mu.Lock()
			mb.stats.Published++
			mb.mu.Unlock()
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (mb *memoryBus) Subscribe(ctx context.Context, f Filter, h Handler) (Subscription, error) {
	mb.mu.Lock()
	id := mb.nextID
	mb.nextID++
	cctx, cancel := context.WithCancel(ctx)
	mb.subscribers[id] = subscriber{filter: f, handler: h, ctx: cctx, cancel: cancel}
	mb.mu.Unlock()

	return &memSub{bus: mb, id: id}, nil
}

func (mb *memoryBus) Metrics() Stats {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	s := mb.stats
	s.InFlight = len(mb.buffer)
	return s
}

// dispatchLoop рассылает события подписчикам.
func (mb *memoryBus) dispatchLoop() {
	for ev := range mb.buffer {
		mb.mu.RLock()
		subs := make([]subscriber, 0, len(mb.subscribers))
		for _, sub := range mb.subscribers {
			subs = append(subs, sub)
		}
		mb.mu.RUnlock()

		for _, sub := range subs {
			if !matchFilter(ev, sub.filter) {
				continue
			}
			// Передаём копию в handler
			go func(s subscriber) {
				select {
				case <-s.ctx.Done():
					return
				default:
					s.handler(s.ctx, ev)
					mb.mu.Lock()
					mb.stats.Consumed++
					mb.mu.Unlock()
				}
			}(sub)
		}
	}
}

func matchFilter(ev *Envelope, f Filter) bool {
	match := func(val string, arr []string) bool {
		if len(arr) == 0 {
			return true
		}
		for _, v := range arr {
			if v == val {
				return true
			}
		}
		return false
	}
	return match(ev.EventType, f.Types) && match(ev.Source, f.Sources)
}

type memSub struct {
	bus *memoryBus
	id  int
}

func (s *memSub) Unsubscribe() {
	s.bus.mu.Lock()
	if sub, ok := s.bus.subscribers[s.id]; ok {
		sub.cancel()
		delete(s.bus.subscribers, s.id)
	}
	s.bus.mu.Unlock()
}
