package implementations

import (
	"testing"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// mockBlockAPI реализует block.BlockAPI для тестирования
type mockBlockAPI struct {
	blocks           map[vec.Vec2]block.BlockID
	metadata         map[vec.Vec2]map[string]interface{}
	scheduledUpdates map[vec.Vec2]bool
}

func newMockBlockAPI() *mockBlockAPI {
	return &mockBlockAPI{
		blocks:           make(map[vec.Vec2]block.BlockID),
		metadata:         make(map[vec.Vec2]map[string]interface{}),
		scheduledUpdates: make(map[vec.Vec2]bool),
	}
}

func (m *mockBlockAPI) GetBlockID(pos vec.Vec2) block.BlockID {
	if id, exists := m.blocks[pos]; exists {
		return id
	}
	return block.AirBlockID
}

func (m *mockBlockAPI) SetBlock(pos vec.Vec2, id block.BlockID) {
	m.blocks[pos] = id
}

func (m *mockBlockAPI) GetBlockMetadata(pos vec.Vec2, key string) interface{} {
	if metadata, exists := m.metadata[pos]; exists {
		return metadata[key]
	}
	return nil
}

func (m *mockBlockAPI) SetBlockMetadata(pos vec.Vec2, key string, value interface{}) {
	if _, exists := m.metadata[pos]; !exists {
		m.metadata[pos] = make(map[string]interface{})
	}
	m.metadata[pos][key] = value
}

func (m *mockBlockAPI) GetBlockIDLayer(layer uint8, pos vec.Vec2) block.BlockID {
	return m.GetBlockID(pos)
}

func (m *mockBlockAPI) SetBlockLayer(layer uint8, pos vec.Vec2, id block.BlockID) {
	m.SetBlock(pos, id)
}

func (m *mockBlockAPI) ScheduleUpdateOnce(pos vec.Vec2) {
	m.scheduledUpdates[pos] = true
}

func (m *mockBlockAPI) TriggerNeighborUpdates(pos vec.Vec2) {
	neighbors := []vec.Vec2{
		{X: pos.X + 1, Y: pos.Y},
		{X: pos.X - 1, Y: pos.Y},
		{X: pos.X, Y: pos.Y + 1},
		{X: pos.X, Y: pos.Y - 1},
	}
	for _, neighbor := range neighbors {
		m.ScheduleUpdateOnce(neighbor)
	}
}

func TestDeepWaterBehavior_TickUpdate(t *testing.T) {
	behavior := &DeepWaterBehavior{}
	api := newMockBlockAPI()

	// Тест 1: Глубинная вода окружена водой - остается глубинной водой
	pos := vec.Vec2{X: 5, Y: 5}
	api.SetBlock(pos, block.DeepWaterBlockID)

	// Окружаем водой
	api.SetBlock(vec.Vec2{X: 6, Y: 5}, block.WaterBlockID)
	api.SetBlock(vec.Vec2{X: 4, Y: 5}, block.WaterBlockID)
	api.SetBlock(vec.Vec2{X: 5, Y: 6}, block.DeepWaterBlockID)
	api.SetBlock(vec.Vec2{X: 5, Y: 4}, block.WaterBlockID)

	behavior.TickUpdate(api, pos)

	if api.GetBlockID(pos) != block.DeepWaterBlockID {
		t.Errorf("Глубинная вода должна остаться глубинной водой, но стала %v", api.GetBlockID(pos))
	}

	// Тест 2: Глубинная вода рядом с воздухом - превращается в обычную воду
	pos2 := vec.Vec2{X: 10, Y: 10}
	api.SetBlock(pos2, block.DeepWaterBlockID)

	// Окружаем водой, но один блок - воздух
	api.SetBlock(vec.Vec2{X: 11, Y: 10}, block.AirBlockID)
	api.SetBlock(vec.Vec2{X: 9, Y: 10}, block.WaterBlockID)
	api.SetBlock(vec.Vec2{X: 10, Y: 11}, block.WaterBlockID)
	api.SetBlock(vec.Vec2{X: 10, Y: 9}, block.WaterBlockID)

	behavior.TickUpdate(api, pos2)

	if api.GetBlockID(pos2) != block.WaterBlockID {
		t.Errorf("Глубинная вода должна превратиться в обычную воду, но осталась %v", api.GetBlockID(pos2))
	}

	// Проверяем, что уровень воды установлен
	if level := api.GetBlockMetadata(pos2, "level"); level != 7 {
		t.Errorf("Уровень воды должен быть 7, но получен %v", level)
	}
}

func TestDeepWaterBehavior_OnBreak(t *testing.T) {
	behavior := &DeepWaterBehavior{}
	api := newMockBlockAPI()

	pos := vec.Vec2{X: 5, Y: 5}

	// Вызываем OnBreak
	behavior.OnBreak(api, pos)

	// Проверяем, что соседние блоки запланированы для обновления
	expectedUpdates := []vec.Vec2{
		{X: 6, Y: 5},
		{X: 4, Y: 5},
		{X: 5, Y: 6},
		{X: 5, Y: 4},
	}

	for _, neighbor := range expectedUpdates {
		if !api.scheduledUpdates[neighbor] {
			t.Errorf("Блок %v должен быть запланирован для обновления", neighbor)
		}
	}
}

func TestDeepWaterBehavior_Properties(t *testing.T) {
	behavior := &DeepWaterBehavior{}

	// Проверяем ID
	if behavior.ID() != block.DeepWaterBlockID {
		t.Errorf("ID должен быть DeepWaterBlockID")
	}

	// Проверяем имя
	if behavior.Name() != "Deep Water" {
		t.Errorf("Имя должно быть 'Deep Water'")
	}

	// Проверяем, что блок не требует постоянных тиков
	if behavior.NeedsTick() {
		t.Errorf("Глубинная вода не должна требовать постоянных тиков")
	}

	// Проверяем проходимость
	if !behavior.IsPassable() {
		t.Errorf("Глубинная вода должна быть проходимой")
	}

	// Проверяем метаданные
	metadata := behavior.CreateMetadata()
	if metadata["deep"] != true {
		t.Errorf("Метаданные должны содержать deep=true")
	}
	if metadata["level"] != 7 {
		t.Errorf("Метаданные должны содержать level=7")
	}
}
