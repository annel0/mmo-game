/** NEWFILE **/
package implementations

import (
	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// TreeBehavior – упрощённый блок дерева, занимающий два слоя.
// trunk – LayerActive (ID TreeBlockID), leaves – LayerCeiling (тот же ID).

const layerCeiling uint8 = 2

type TreeBehavior struct{}

func (b *TreeBehavior) ID() block.BlockID                           { return block.TreeBlockID }
func (b *TreeBehavior) Name() string                                { return "Tree" }
func (b *TreeBehavior) NeedsTick() bool                             { return false }
func (b *TreeBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {}
func (b *TreeBehavior) CreateMetadata() block.Metadata              { return block.Metadata{"height": 2} }

func (b *TreeBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	above := vec.Vec2{X: pos.X, Y: pos.Y}
	if api.GetBlockIDLayer(layerCeiling, above) != block.AirBlockID {
		return
	}
	api.SetBlockLayer(layerCeiling, above, block.TreeBlockID)
}

func (b *TreeBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	above := vec.Vec2{X: pos.X, Y: pos.Y}
	if api.GetBlockIDLayer(layerCeiling, above) == block.TreeBlockID {
		api.SetBlockLayer(layerCeiling, above, block.AirBlockID)
	}
}

func (b *TreeBehavior) HandleInteraction(action string, cur, act map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	return block.TreeBlockID, cur, block.InteractionResult{Success: false}
}

func init() { block.Register(block.TreeBlockID, &TreeBehavior{}) }
