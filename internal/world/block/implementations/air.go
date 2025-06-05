package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// AirBehavior реализует поведение пустого блока (воздуха)
type AirBehavior struct{}

// ID возвращает идентификатор блока
func (b *AirBehavior) ID() block.BlockID {
	return block.AirBlockID
}

// Name возвращает имя блока
func (b *AirBehavior) Name() string {
	return "Air"
}

// NeedsTick возвращает false, воздух статичен
func (b *AirBehavior) NeedsTick() bool {
	return false
}

// TickUpdate ничего не делает для воздуха
func (b *AirBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {
	// Воздух не обновляется
}

// OnPlace вызывается при установке блока
func (b *AirBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	// Ничего не делаем
}

// OnBreak вызывается при разрушении блока
func (b *AirBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// Ничего не делаем
}

// CreateMetadata создает пустые метаданные
func (b *AirBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{}
}

// HandleInteraction обрабатывает взаимодействие с блоком воздуха
func (b *AirBehavior) HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	// Воздух нельзя изменить взаимодействием, но можно поставить блок
	if action == "place" {
		if blockID, ok := actionPayload["block_id"].(float64); ok {
			newBlockID := block.BlockID(uint16(blockID))

			// Получаем поведение для создаваемого блока
			behavior, exists := block.Get(newBlockID)
			if exists {
				// Создаем метаданные для нового блока
				return newBlockID, behavior.CreateMetadata(), block.InteractionResult{
					Success: true,
					Message: "Блок установлен",
				}
			}
		}
	}

	// Стандартное взаимодействие не меняет блок
	return block.AirBlockID, currentPayload, block.InteractionResult{
		Success: false,
		Message: "Нельзя взаимодействовать с воздухом",
	}
}

func init() {
	block.Register(block.AirBlockID, &AirBehavior{})
}
