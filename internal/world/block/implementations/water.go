package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// WaterBehavior реализует поведение блока воды
type WaterBehavior struct{}

// ID возвращает идентификатор блока
func (b *WaterBehavior) ID() block.BlockID {
	return block.WaterBlockID
}

// Name возвращает имя блока
func (b *WaterBehavior) Name() string {
	return "Water"
}

// NeedsTick возвращает true, так как вода течет
func (b *WaterBehavior) NeedsTick() bool {
	return true
}

// TickUpdate обновляет состояние воды - растекание
func (b *WaterBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {
	// Получаем текущий уровень воды
	level, ok := api.GetBlockMetadata(pos, "level").(int)
	if !ok {
		// Если метаданных нет или они некорректные, инициализируем
		level = 7 // Максимальный уровень воды
		api.SetBlockMetadata(pos, "level", level)
		return
	}

	// Если уровень воды минимальный, ничего не делаем
	if level <= 0 {
		return
	}

	// Проверяем блоки вокруг для растекания
	directions := []vec.Vec2{
		{X: pos.X + 1, Y: pos.Y}, // право
		{X: pos.X - 1, Y: pos.Y}, // лево
		{X: pos.X, Y: pos.Y + 1}, // вниз
		{X: pos.X, Y: pos.Y - 1}, // вверх
	}

	for _, dir := range directions {
		targetID := api.GetBlockID(dir)

		// Если блок воздух, заменяем на воду с уменьшенным уровнем
		if targetID == block.AirBlockID {
			api.SetBlock(dir, block.WaterBlockID)
			api.SetBlockMetadata(dir, "level", level-1)
			continue
		}

		// Если блок уже вода, пытаемся выровнять уровни
		if targetID == block.WaterBlockID {
			targetLevel, ok := api.GetBlockMetadata(dir, "level").(int)
			if !ok {
				continue
			}

			// Если уровень воды в текущем блоке выше, чем в соседнем на 2+,
			// выравниваем уровни
			if level > targetLevel+1 {
				newLevel := (level + targetLevel) / 2
				api.SetBlockMetadata(pos, "level", newLevel)
				api.SetBlockMetadata(dir, "level", newLevel)
			}
		}
	}
}

// OnPlace инициализирует блок при установке
func (b *WaterBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	api.SetBlockMetadata(pos, "level", 7) // Максимальный уровень воды
}

// OnBreak вызывается при разрушении блока
func (b *WaterBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// Просто удаляем блок, без дополнительных действий
}

// CreateMetadata создает начальные метаданные для блока
func (b *WaterBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{"level": 7}
}

// HandleInteraction обрабатывает взаимодействие с блоком воды
func (b *WaterBehavior) HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	// Копируем текущие метаданные
	newPayload := make(map[string]interface{})
	for k, v := range currentPayload {
		newPayload[k] = v
	}

	if action == "use" {
		// Если используется ведро
		if tool, ok := actionPayload["tool"].(string); ok && tool == "bucket" {
			// Уменьшаем уровень воды
			level := 7
			if l, ok := currentPayload["level"].(float64); ok {
				level = int(l)
			}

			// Уменьшаем уровень
			level -= 3

			// Если воды не осталось, заменяем на воздух
			if level <= 0 {
				return block.AirBlockID, map[string]interface{}{}, block.InteractionResult{
					Success: true,
					Message: "Вода собрана в ведро",
					Effects: []string{"particle_splash"},
				}
			}

			// Иначе обновляем уровень воды
			newPayload["level"] = level
			return block.WaterBlockID, newPayload, block.InteractionResult{
				Success: true,
				Message: "Часть воды собрана в ведро",
				Effects: []string{"particle_splash"},
			}
		}
	} else if action == "place" {
		// Если в воду что-то помещают, это может быть невозможно
		return block.WaterBlockID, currentPayload, block.InteractionResult{
			Success: false,
			Message: "Нельзя поместить этот блок в воду",
		}
	}

	// Стандартное взаимодействие не меняет блок
	return block.WaterBlockID, currentPayload, block.InteractionResult{
		Success: false,
		Message: "Действие не поддерживается для воды",
	}
}

func init() {
	block.Register(block.WaterBlockID, &WaterBehavior{})
}
