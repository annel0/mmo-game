package eventbus

import "context"

var globalBus EventBus

// Init устанавливает глобальную шину.
func Init(bus EventBus) { globalBus = bus }

// Publish отправляет событие в глобальную шину, если она инициализирована.
func Publish(ctx context.Context, ev *Envelope) error {
	if globalBus == nil {
		return nil
	}
	return globalBus.Publish(ctx, ev)
}
