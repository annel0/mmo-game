package world

import (
	"testing"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

func TestChunkCreateAndGetBlock(t *testing.T) {
	coords := vec.Vec2{X: 5, Y: 10}
	chunk := NewChunk(coords)

	// Проверяем координаты
	if chunk.Coords.X != 5 || chunk.Coords.Y != 10 {
		t.Errorf("Ожидались координаты {5,10}, получено {%d,%d}", chunk.Coords.X, chunk.Coords.Y)
	}

	// Проверяем, что блоки инициализированы как пустые
	pos := vec.Vec2{X: 3, Y: 4}
	blockID := chunk.GetBlock(pos)
	if blockID != block.AirBlockID {
		t.Errorf("Ожидался пустой блок (AirBlockID), получен %d", blockID)
	}

	// Устанавливаем и проверяем блок
	chunk.SetBlock(pos, block.StoneBlockID)
	blockID = chunk.GetBlock(pos)
	if blockID != block.StoneBlockID {
		t.Errorf("Ожидался StoneBlockID, получен %d", blockID)
	}
}

func TestChunkMetadata(t *testing.T) {
	coords := vec.Vec2{X: 1, Y: 2}
	chunk := NewChunk(coords)
	pos := vec.Vec2{X: 5, Y: 5}

	// Проверяем, что изначально метаданных нет
	metadata := chunk.GetBlockMetadata(pos)
	if len(metadata) != 0 {
		t.Errorf("Ожидалось отсутствие метаданных, получено %v", metadata)
	}

	// Устанавливаем метаданные
	chunk.SetBlockMetadata(pos, "test_key", 42)

	// Проверяем установленные метаданные
	metadata = chunk.GetBlockMetadata(pos)
	if len(metadata) != 1 {
		t.Errorf("Ожидалось наличие 1 метаданного, получено %d", len(metadata))
	}

	value, exists := metadata["test_key"]
	if !exists {
		t.Error("Ключ test_key не найден в метаданных")
	}

	intValue, ok := value.(int)
	if !ok {
		t.Errorf("Значение не int: %v", value)
	}

	if intValue != 42 {
		t.Errorf("Ожидалось 42, получено %d", intValue)
	}

	// Проверяем получение одного значения
	singleValue, exists := chunk.GetBlockMetadataValue(pos, "test_key")
	if !exists {
		t.Error("Ключ test_key не найден при прямом запросе")
	}

	intValue, ok = singleValue.(int)
	if !ok || intValue != 42 {
		t.Errorf("Неверное значение: %v", singleValue)
	}

	// Проверяем отсутствующий ключ
	_, exists = chunk.GetBlockMetadataValue(pos, "non_existent_key")
	if exists {
		t.Error("Несуществующий ключ найден")
	}
}

func TestChunkChanges(t *testing.T) {
	coords := vec.Vec2{X: 3, Y: 4}
	chunk := NewChunk(coords)

	// Изначально изменений нет
	if chunk.HasChanges() {
		t.Error("Новый чанк не должен иметь изменений")
	}

	// Добавляем изменение
	pos := vec.Vec2{X: 1, Y: 2}
	chunk.SetBlock(pos, block.StoneBlockID)

	// Проверяем наличие изменений
	if !chunk.HasChanges() {
		t.Error("Чанк должен иметь изменения после SetBlock")
	}

	// Проверяем счетчик изменений
	if chunk.ChangeCounter != 1 {
		t.Errorf("Ожидался 1 счетчик изменений, получено %d", chunk.ChangeCounter)
	}

	// Очищаем изменения
	chunk.ClearChanges()

	// Проверяем отсутствие изменений
	if chunk.HasChanges() {
		t.Error("Чанк не должен иметь изменений после ClearChanges")
	}

	// Проверяем счетчик изменений
	if chunk.ChangeCounter != 0 {
		t.Errorf("Ожидался 0 счетчик изменений, получено %d", chunk.ChangeCounter)
	}
}
