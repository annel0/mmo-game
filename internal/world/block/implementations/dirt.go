package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// DirtBehavior реализует поведение блока земли/грязи
type DirtBehavior struct{}

// ID возвращает идентификатор блока
func (b *DirtBehavior) ID() block.BlockID {
	return block.DirtBlockID
}

// Name возвращает имя блока
func (b *DirtBehavior) Name() string {
	return "Dirt"
}

// NeedsTick возвращает false, земля статична
func (b *DirtBehavior) NeedsTick() bool {
	return false
}

// TickUpdate ничего не делает для земли
func (b *DirtBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {
	// Земля не обновляется каждый тик
}

// OnPlace инициализирует блок при установке
func (b *DirtBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	// Инициализируем влажность земли
	api.SetBlockMetadata(pos, "moisture", 0)
}

// OnBreak вызывается при разрушении блока
func (b *DirtBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// Ничего не делаем при разрушении
}

// CreateMetadata создает начальные метаданные для блока
func (b *DirtBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{
		"moisture": 0,
	}
}

// HandleInteraction обрабатывает взаимодействие с блоком земли
func (b *DirtBehavior) HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	// Копируем текущие метаданные
	newPayload := make(map[string]interface{})
	for k, v := range currentPayload {
		newPayload[k] = v
	}

	if action == "use" {
		// Если используется предмет (например, лопата или семена)
		if tool, ok := actionPayload["tool"].(string); ok {
			if tool == "seed" {
				// Превращаем землю в траву при использовании семян
				return block.GrassBlockID, map[string]interface{}{"growth": 0}, block.InteractionResult{
					Success: true,
					Message: "Земля засеяна травой",
				}
			} else if tool == "water" {
				// Увеличиваем влажность земли
				moisture := 0
				if m, ok := currentPayload["moisture"].(float64); ok {
					moisture = int(m)
				}

				if moisture < 10 {
					moisture += 2
					if moisture > 10 {
						moisture = 10
					}
				}

				newPayload["moisture"] = moisture

				return block.DirtBlockID, newPayload, block.InteractionResult{
					Success: true,
					Message: "Земля увлажнена",
				}
			}
		}
	}

	// Стандартное взаимодействие не меняет блок
	return block.DirtBlockID, currentPayload, block.InteractionResult{
		Success: false,
		Message: "Действие не поддерживается для земли",
	}
}

func init() {
	block.Register(block.DirtBlockID, &DirtBehavior{})
}
