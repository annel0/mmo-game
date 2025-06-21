package eventbus

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	nats "github.com/nats-io/nats.go"
)

// JetStreamBus реализует EventBus поверх NATS JetStream.
type JetStreamBus struct {
	nc        *nats.Conn
	js        nats.JetStreamContext
	stream    string
	published uint64
	consumed  uint64
	dropped   uint64
}

// NewJetStreamBus подключается к кластеру NATS и гарантирует наличие стрима.
// url: nats://127.0.0.1:4222, stream: "EVENTS".
func NewJetStreamBus(url, stream string, retention time.Duration) (*JetStreamBus, error) {
	if stream == "" {
		stream = "EVENTS"
	}

	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Drain()
		return nil, fmt.Errorf("jetstream: %w", err)
	}

	// Ensure stream exists (subjects: events.*)
	_, err = js.StreamInfo(stream)
	if err != nil {
		_, err = js.AddStream(&nats.StreamConfig{
			Name:      stream,
			Subjects:  []string{"events.*"},
			Retention: nats.LimitsPolicy,
			MaxAge:    retention,
			Storage:   nats.FileStorage,
		})
		if err != nil {
			nc.Drain()
			return nil, fmt.Errorf("add stream: %w", err)
		}
	}

	return &JetStreamBus{nc: nc, js: js, stream: stream}, nil
}

// Publish сериализует Envelope в JSON и публикует в subject events.<type>.
func (jb *JetStreamBus) Publish(ctx context.Context, ev *Envelope) error {
	subj := fmt.Sprintf("events.%s", ev.EventType)
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = jb.js.Publish(subj, data)
	if err == nil {
		atomic.AddUint64(&jb.published, 1)
	}
	return err
}

// Subscribe создаёт durable consumer и вызывает handler асинхронно.
func (jb *JetStreamBus) Subscribe(ctx context.Context, f Filter, h Handler) (Subscription, error) {
	subj := "events.*"
	if len(f.Types) == 1 {
		subj = fmt.Sprintf("events.%s", f.Types[0])
	}

	durable := nats.Durable(fmt.Sprintf("sub_%d", time.Now().UnixNano()))

	natSub, err := jb.js.Subscribe(subj, func(msg *nats.Msg) {
		var ev Envelope
		if err := json.Unmarshal(msg.Data, &ev); err == nil {
			h(ctx, &ev)
			atomic.AddUint64(&jb.consumed, 1)
		}
		_ = msg.Ack()
	}, nats.ManualAck(), durable, nats.AckWait(30*time.Second))
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}

	return &jetSub{natSub}, nil
}

// jetSub обёртка вокруг *nats.Subscription чтобы удовлетворить наш интерфейс.
type jetSub struct{
	s *nats.Subscription
}

func (j *jetSub) Unsubscribe() {
	_ = j.s.Unsubscribe()
}

// Metrics возвращает текущие метрики.
func (jb *JetStreamBus) Metrics() Stats {
	return Stats{
		Published: atomic.LoadUint64(&jb.published),
		Consumed:  atomic.LoadUint64(&jb.consumed),
		Dropped:   atomic.LoadUint64(&jb.dropped),
		InFlight:  0, // jetstream keeps its own queue
	}
}
