package world

import (
	"github.com/annel0/mmo-game/internal/world/block"
)

// Block представляет собой блок в игровом мире
type Block struct {
	ID      block.BlockID          // Идентификатор типа блока
	Payload map[string]interface{} // Метаданные блока (состояние)
}

// NewBlock создаёт новый блок с указанным ID и инициализированными метаданными
func NewBlock(id block.BlockID) Block {
	behavior, exists := block.Get(id)
	if !exists {
		return Block{
			ID:      id,
			Payload: make(map[string]interface{}),
		}
	}

	// Инициализируем метаданные через поведение блока
	return Block{
		ID:      id,
		Payload: behavior.CreateMetadata(),
	}
}

// GetBehavior возвращает поведение для блока
func (b Block) GetBehavior() (block.BlockBehavior, bool) {
	return block.Get(b.ID)
}

// NeedsTick возвращает true, если блок требует обновления в тиках
func (b Block) NeedsTick() bool {
	behavior, exists := b.GetBehavior()
	if !exists {
		return false
	}
	return behavior.NeedsTick()
}

// Clone создаёт копию блока
func (b Block) Clone() Block {
	newPayload := make(map[string]interface{}, len(b.Payload))
	for k, v := range b.Payload {
		newPayload[k] = v
	}

	return Block{
		ID:      b.ID,
		Payload: newPayload,
	}
}
