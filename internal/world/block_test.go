package world

import (
	"testing"

	"github.com/annel0/mmo-game/internal/world/block"
	// Импортируем реализации блоков для регистрации в init()
	_ "github.com/annel0/mmo-game/internal/world/block/implementations"
)

func TestBlockCreation(t *testing.T) {
	// Инициализируем реализации блоков для регистрации в регистре
	_ = block.StoneBlockID

	// Тест создания блока без метаданных
	b1 := NewBlock(block.StoneBlockID)
	if b1.ID != block.StoneBlockID {
		t.Errorf("Ожидался StoneBlockID, получен %d", b1.ID)
	}

	if b1.Payload == nil {
		t.Error("Payload должен быть инициализирован, получен nil")
	}

	// Проверяем, что поведение блока корректно получается
	behavior, exists := b1.GetBehavior()
	if !exists {
		t.Error("Поведение блока не найдено")
		return // Прерываем тест, чтобы не разыменовывать nil
	}

	if behavior.ID() != block.StoneBlockID {
		t.Errorf("Ожидался ID блока %d, получен %d", block.StoneBlockID, behavior.ID())
	}
}

func TestBlockMetadata(t *testing.T) {
	// Создаем блок с метаданными
	b := NewBlock(block.GrassBlockID)

	// Проверяем, что метаданные инициализированы из поведения
	growth, exists := b.Payload["growth"]
	if !exists {
		t.Error("Ожидалось наличие ключа 'growth' в метаданных")
	}

	// Проверяем тип и значение
	intValue, ok := growth.(int)
	if !ok {
		t.Errorf("Ожидался int, получен %T", growth)
	}

	if intValue != 0 {
		t.Errorf("Ожидалось начальное значение роста 0, получено %d", intValue)
	}
}

func TestBlockClone(t *testing.T) {
	// Создаем исходный блок с метаданными
	original := NewBlock(block.WaterBlockID)
	original.Payload["level"] = 5

	// Клонируем блок
	clone := original.Clone()

	// Проверяем, что ID совпадают
	if clone.ID != original.ID {
		t.Errorf("Ожидался тот же ID %d, получен %d", original.ID, clone.ID)
	}

	// Проверяем метаданные
	level, exists := clone.Payload["level"]
	if !exists {
		t.Error("Ожидалось наличие ключа 'level' в метаданных")
	}

	intValue, ok := level.(int)
	if !ok || intValue != 5 {
		t.Errorf("Ожидалось значение 5, получено %v (%T)", level, level)
	}

	// Проверяем, что изменение метаданных клона не влияет на оригинал
	clone.Payload["level"] = 3

	originalLevel, _ := original.Payload["level"].(int)
	cloneLevel, _ := clone.Payload["level"].(int)

	if originalLevel != 5 || cloneLevel != 3 {
		t.Errorf("Ожидалось: оригинал=5, клон=3; получено: оригинал=%d, клон=%d",
			originalLevel, cloneLevel)
	}
}

func TestBlockNeedsTick(t *testing.T) {
	// Проверяем блоки, которые требуют тиков
	waterBlock := NewBlock(block.WaterBlockID)
	if !waterBlock.NeedsTick() {
		t.Error("Ожидалось, что блок воды требует тиков")
	}

	grassBlock := NewBlock(block.GrassBlockID)
	if !grassBlock.NeedsTick() {
		t.Error("Ожидалось, что блок травы требует тиков")
	}

	// Проверяем блоки, которые не требуют тиков
	stoneBlock := NewBlock(block.StoneBlockID)
	if stoneBlock.NeedsTick() {
		t.Error("Ожидалось, что блок камня не требует тиков")
	}

	airBlock := NewBlock(block.AirBlockID)
	if airBlock.NeedsTick() {
		t.Error("Ожидалось, что блок воздуха не требует тиков")
	}
}
