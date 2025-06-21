package block

import (
	"github.com/annel0/mmo-game/internal/vec"
)

// BlockAPI определяет интерфейс для взаимодействия блоков с игровым миром.
// Этот интерфейс предоставляет блокам возможность читать и изменять состояние
// мира, включая получение и установку блоков, работу с метаданными и
// управление системой обновлений.
type BlockAPI interface {
	// GetBlockID возвращает идентификатор блока в указанной позиции (активный слой).
	GetBlockID(pos vec.Vec2) BlockID

	// SetBlock устанавливает блок в указанной позиции (активный слой).
	SetBlock(pos vec.Vec2, id BlockID)

	// GetBlockMetadata возвращает значение метаданных блока по ключу.
	GetBlockMetadata(pos vec.Vec2, key string) interface{}

	// SetBlockMetadata устанавливает значение метаданных блока по ключу.
	SetBlockMetadata(pos vec.Vec2, key string, value interface{})

	// GetBlockIDLayer возвращает идентификатор блока в указанной позиции и слое.
	GetBlockIDLayer(layer uint8, pos vec.Vec2) BlockID

	// SetBlockLayer устанавливает блок в указанной позиции и слое.
	SetBlockLayer(layer uint8, pos vec.Vec2, id BlockID)

	// ScheduleUpdateOnce помечает блок для разового обновления в следующем тике.
	// Используется для избежания лишних вычислений при массовых изменениях.
	ScheduleUpdateOnce(pos vec.Vec2)

	// TriggerNeighborUpdates запускает разовое обновление для всех соседних блоков.
	// Обновляет блоки сверху, снизу, слева и справа от указанной позиции.
	TriggerNeighborUpdates(pos vec.Vec2)
}
