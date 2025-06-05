package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// StoneBehavior реализует поведение блока камня
type StoneBehavior struct{}

// ID возвращает идентификатор блока
func (b *StoneBehavior) ID() block.BlockID {
	return block.StoneBlockID
}

// Name возвращает имя блока
func (b *StoneBehavior) Name() string {
	return "Stone"
}

// NeedsTick возвращает false, камень статичен
func (b *StoneBehavior) NeedsTick() bool {
	return false
}

// TickUpdate ничего не делает для камня
func (b *StoneBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {
	// Камень не обновляется каждый тик
}

// OnPlace инициализирует блок при установке
func (b *StoneBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	// Инициализируем прочность камня
	api.SetBlockMetadata(pos, "hardness", 10)

	// Возможно, есть вероятность появления руды
	api.SetBlockMetadata(pos, "has_ore", false)
}

// OnBreak вызывается при разрушении блока
func (b *StoneBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// Проверим, была ли в камне руда
	hasOre, ok := api.GetBlockMetadata(pos, "has_ore").(bool)
	if ok && hasOre {
		// Здесь можно было бы создать выпадающий предмет руды
		// Но в текущей архитектуре это не реализовано
	}
}

// CreateMetadata создает начальные метаданные для блока
func (b *StoneBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{
		"hardness": 10,
		"has_ore":  false,
	}
}

// HandleInteraction обрабатывает взаимодействие с блоком камня
func (b *StoneBehavior) HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	// Копируем текущие метаданные
	newPayload := make(map[string]interface{})
	for k, v := range currentPayload {
		newPayload[k] = v
	}

	if action == "mine" {
		// Получаем текущую прочность
		hardness := 10
		if h, ok := currentPayload["hardness"].(float64); ok {
			hardness = int(h)
		}

		// Сила воздействия (по умолчанию 1)
		strength := 1
		if s, ok := actionPayload["strength"].(float64); ok {
			strength = int(s)
		}

		// Уменьшаем прочность
		hardness -= strength
		newPayload["hardness"] = hardness

		// Если прочность исчерпана, блок разрушается
		if hardness <= 0 {
			return block.AirBlockID, map[string]interface{}{}, block.InteractionResult{
				Success: true,
				Message: "Камень разрушен",
				Effects: []string{"particle_break"},
			}
		}

		// Возвращаем обновленные метаданные
		return block.StoneBlockID, newPayload, block.InteractionResult{
			Success: true,
			Message: "Камень поврежден",
			Effects: []string{"particle_hit"},
		}
	}

	// Стандартное взаимодействие не меняет блок
	return block.StoneBlockID, currentPayload, block.InteractionResult{
		Success: false,
		Message: "Действие не поддерживается для камня",
	}
}

func init() {
	block.Register(block.StoneBlockID, &StoneBehavior{})
}
