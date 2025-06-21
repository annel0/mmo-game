package eventbus

import (
	"context"

	"github.com/annel0/mmo-game/internal/logging"
)

// StartLoggingListener подписывается на все события и пишет их в стандартный лог.
// Функция неблокирующая.
func StartLoggingListener(bus EventBus) error {
	_, err := bus.Subscribe(context.Background(), Filter{}, func(ctx context.Context, ev *Envelope) {
		logging.Debug("[EventBus] %s %s src=%s prio=%d size=%dB", ev.ID, ev.EventType, ev.Source, ev.Priority, len(ev.Payload))
	})
	if err != nil {
		return err
	}
	logging.Info("🪵 LoggingListener: подписка на все события активирована")
	return nil
}
