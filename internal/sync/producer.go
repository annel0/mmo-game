package sync

import (
	"context"

	"github.com/annel0/mmo-game/internal/eventbus"
)

// SyncProducer подписывается на события мира и передаёт изменения BatchManager'у.

type SyncProducer struct {
	bus eventbus.EventBus
	bm  *BatchManager
	sub eventbus.Subscription
}

func NewSyncProducer(bus eventbus.EventBus, bm *BatchManager) (*SyncProducer, error) {
	sp := &SyncProducer{bus: bus, bm: bm}
	sub, err := bus.Subscribe(context.Background(), eventbus.Filter{Types: []string{"BlockEvent", "EntityEvent"}}, sp.handle)
	if err != nil {
		return nil, err
	}
	sp.sub = sub
	return sp, nil
}

func (sp *SyncProducer) handle(ctx context.Context, ev *eventbus.Envelope) {
	// пока просто прокидываем payload, priority 3
	sp.bm.AddChange(Change{Data: ev.Payload, Priority: 3})
}

func (sp *SyncProducer) Stop() { sp.sub.Unsubscribe() }
