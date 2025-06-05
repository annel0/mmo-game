package block

import "github.com/annel0/mmo-game/internal/vec"

// BlockAPI определяет интерфейс для взаимодействия блоков с миром
type BlockAPI interface {
    GetBlockID(pos vec.Vec2) BlockID
    SetBlock(pos vec.Vec2, id BlockID)
    GetBlockMetadata(pos vec.Vec2, key string) interface{}
    SetBlockMetadata(pos vec.Vec2, key string, value interface{})
}