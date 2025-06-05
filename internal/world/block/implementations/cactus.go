/** NEWFILE **/
package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// CactusBehavior описывает блок кактуса, который занимает два слоя: ACTIVE и CEILING.
// Нижний ствол – LayerActive, верхушка – LayerCeiling.  При попытке поместить
// кактус под существующий потолок установка отклоняется.

type CactusBehavior struct{}

func (b *CactusBehavior) ID() block.BlockID { return block.CactusBlockID }
func (b *CactusBehavior) Name() string      { return "Cactus" }

func (b *CactusBehavior) NeedsTick() bool                             { return false }
func (b *CactusBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {}
func (b *CactusBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{"height": 2}
}

func (b *CactusBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	// Проверяем, свободен ли потолок
	const layerCeiling uint8 = 2
	above := vec.Vec2{X: pos.X, Y: pos.Y}
	if api.GetBlockIDLayer(layerCeiling, above) != block.AirBlockID {
		// Отменяем установку – потолок занят
		return
	}

	// Ставим верхушку в слой CEILING
	api.SetBlockLayer(layerCeiling, above, block.CactusBlockID)
}

func (b *CactusBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// Удаляем верхушку
	const layerCeiling uint8 = 2
	above := vec.Vec2{X: pos.X, Y: pos.Y}
	if api.GetBlockIDLayer(layerCeiling, above) == block.CactusBlockID {
		api.SetBlockLayer(layerCeiling, above, block.AirBlockID)
	}
}

// HandleInteraction – простой сбор
func (b *CactusBehavior) HandleInteraction(action string, cur, act map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	return block.CactusBlockID, cur, block.InteractionResult{Success: false}
}

func init() {
	block.Register(block.CactusBlockID, &CactusBehavior{})
}
