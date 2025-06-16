package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// DeepWaterBehavior реализует поведение блока глубинной воды
// Глубинная вода статична и не распространяется, но может превратиться
// в обычную воду при контакте с не-водными блоками
type DeepWaterBehavior struct{}

// ID возвращает идентификатор блока
func (b *DeepWaterBehavior) ID() block.BlockID {
	return block.DeepWaterBlockID
}

// Name возвращает имя блока
func (b *DeepWaterBehavior) Name() string {
	return "Deep Water"
}

// NeedsTick возвращает false, так как глубинная вода не требует постоянных обновлений
func (b *DeepWaterBehavior) NeedsTick() bool {
	return false
}

// TickUpdate вызывается только через updateOnce при изменении соседних блоков
func (b *DeepWaterBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {
	// Проверяем соседние блоки
	neighbors := []vec.Vec2{
		{X: pos.X + 1, Y: pos.Y}, // право
		{X: pos.X - 1, Y: pos.Y}, // лево
		{X: pos.X, Y: pos.Y + 1}, // вниз
		{X: pos.X, Y: pos.Y - 1}, // вверх
	}

	// Если хотя бы один сосед не является водой или глубинной водой,
	// превращаемся в обычную воду
	for _, neighbor := range neighbors {
		neighborID := api.GetBlockID(neighbor)
		if neighborID != block.WaterBlockID && neighborID != block.DeepWaterBlockID {
			// Превращаемся в обычную воду
			api.SetBlock(pos, block.WaterBlockID)
			// Инициализируем уровень воды
			api.SetBlockMetadata(pos, "level", 7)
			return
		}
	}
}

// OnPlace вызывается при установке блока
func (b *DeepWaterBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	// Инициализируем метаданные глубинной воды
	api.SetBlockMetadata(pos, "deep", true)
	api.SetBlockMetadata(pos, "level", 7) // Максимальный уровень
}

// OnBreak вызывается при разрушении блока
func (b *DeepWaterBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// При удалении глубинной воды запускаем обновление соседних блоков
	api.TriggerNeighborUpdates(pos)
}

// CreateMetadata создает начальные метаданные для блока
func (b *DeepWaterBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{
		"deep":  true,
		"level": 7,
	}
}

// IsPassable возвращает true — игрок может перемещаться в глубинной воде (плавать)
func (b *DeepWaterBehavior) IsPassable() bool {
	return true
}

// HandleInteraction обрабатывает взаимодействие с блоком глубинной воды
func (b *DeepWaterBehavior) HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	// Глубинная вода не имеет специальных взаимодействий
	return block.DeepWaterBlockID, currentPayload, block.InteractionResult{
		Success: false,
		Message: "Глубинная вода не поддерживает взаимодействие",
	}
}

func init() {
	block.Register(block.DeepWaterBlockID, &DeepWaterBehavior{})
}
