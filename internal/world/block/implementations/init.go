package implementations

import "github.com/annel0/mmo-game/internal/world/block"

// Регистрируем все типы блоков при импорте пакета
func init() {
	// Базовые блоки
	block.Register(block.AirBlockID, &AirBehavior{})
	block.Register(block.StoneBlockID, &StoneBehavior{})
	block.Register(block.GrassBlockID, &GrassBehavior{})
	block.Register(block.WaterBlockID, &WaterBehavior{})
	block.Register(block.DirtBlockID, &DirtBehavior{})
}
