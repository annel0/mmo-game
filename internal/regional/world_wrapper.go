package regional

import (
	"github.com/annel0/mmo-game/internal/world"
)

// World интерфейс для абстракции мира в Regional Node
// Пока просто алиас для world.WorldManager, но в будущем может расширяться
type World = world.WorldManager

// NewWorld создаёт новый экземпляр мира для региональной ноды
func NewWorld(seed int64) *World {
	return world.NewWorldManager(seed)
}
