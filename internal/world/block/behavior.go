package block

import (
	"github.com/annel0/mmo-game/internal/vec"
)

type Metadata map[string]interface{}

// InteractionResult представляет результат взаимодействия с блоком
type InteractionResult struct {
	Success bool     // Успешно ли выполнено взаимодействие
	Message string   // Сообщение о результате взаимодействия
	Effects []string // Эффекты взаимодействия (опционально)
}

// BlockBehavior определяет поведение блока
type BlockBehavior interface {
	ID() BlockID
	Name() string
	NeedsTick() bool
	TickUpdate(api BlockAPI, pos vec.Vec2)
	OnPlace(api BlockAPI, pos vec.Vec2)
	OnBreak(api BlockAPI, pos vec.Vec2)
	CreateMetadata() Metadata
	// Новый метод для обработки взаимодействия
	HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (BlockID, map[string]interface{}, InteractionResult)
}
