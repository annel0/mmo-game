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

// testBlock - структура для хранения блока в тестовом мире
type testBlock struct {
	id       block.BlockID
	metadata map[string]interface{}
}

func TestDeepWaterWithLayers(t *testing.T) {
	// Регистрируем базовые блоки если они ещё не зарегистрированы
	if _, exists := block.Get(block.AirBlockID); !exists {
		block.Register(block.AirBlockID, &AirBehavior{})
	}
	if _, exists := block.Get(block.WaterBlockID); !exists {
		block.Register(block.WaterBlockID, &WaterBehavior{})
	}
	if _, exists := block.Get(block.DeepWaterBlockID); !exists {
		block.Register(block.DeepWaterBlockID, &DeepWaterBehavior{})
	}
	if _, exists := block.Get(block.DirtBlockID); !exists {
		block.Register(block.DirtBlockID, &DirtBehavior{})
	}
	if _, exists := block.Get(block.StoneBlockID); !exists {
		block.Register(block.StoneBlockID, &StoneBehavior{})
	}

	t.Run("deep_water_should_be_in_floor_layer", func(t *testing.T) {
		// Создаем тестовый мир со слоями
		world := createLayeredTestWorld()
		api := &testLayeredBlockAPI{world: world}

		// Размещаем глубинную воду в слое FLOOR
		api.SetBlockLayer(0, vec.Vec2{X: 5, Y: 5}, block.DeepWaterBlockID)

		// Проверяем, что вода в слое FLOOR
		if api.GetBlockIDLayer(0, vec.Vec2{X: 5, Y: 5}) != block.DeepWaterBlockID {
			t.Error("Deep water should be in floor layer")
		}

		// Проверяем, что в активном слое воздух
		if api.GetBlockIDLayer(1, vec.Vec2{X: 5, Y: 5}) != block.AirBlockID {
			t.Error("Active layer should have air above deep water")
		}
	})

	t.Run("deep_water_converts_to_regular_water_on_active_layer", func(t *testing.T) {
		world := createLayeredTestWorld()
		api := &testLayeredBlockAPI{world: world}

		// Размещаем глубинную воду в активном слое (старая логика)
		deepWater := &DeepWaterBehavior{}
		api.SetBlockLayer(1, vec.Vec2{X: 5, Y: 5}, block.DeepWaterBlockID)
		deepWater.OnPlace(api, vec.Vec2{X: 5, Y: 5})

		// Размещаем твердый блок рядом
		api.SetBlockLayer(1, vec.Vec2{X: 6, Y: 5}, block.DirtBlockID)

		// Запускаем обновление
		api.TriggerNeighborUpdates(vec.Vec2{X: 6, Y: 5})
		deepWater.TickUpdate(api, vec.Vec2{X: 5, Y: 5})

		// Проверяем, что глубинная вода превратилась в обычную
		if api.GetBlockIDLayer(1, vec.Vec2{X: 5, Y: 5}) != block.WaterBlockID {
			t.Error("Deep water should convert to regular water when adjacent to solid blocks")
		}
	})
}

// createLayeredTestWorld создаёт тестовый мир с поддержкой слоёв
func createLayeredTestWorld() map[vec.Vec2]map[uint8]testBlock {
	world := make(map[vec.Vec2]map[uint8]testBlock)
	// Инициализируем базовые позиции
	for x := 0; x < 10; x++ {
		for y := 0; y < 10; y++ {
			pos := vec.Vec2{X: x, Y: y}
			world[pos] = map[uint8]testBlock{
				0: {id: block.AirBlockID}, // FLOOR
				1: {id: block.AirBlockID}, // ACTIVE
				2: {id: block.AirBlockID}, // CEILING
			}
		}
	}
	return world
}

// testLayeredBlockAPI - тестовая реализация BlockAPI с поддержкой слоёв
type testLayeredBlockAPI struct {
	world         map[vec.Vec2]map[uint8]testBlock
	scheduledOnce map[vec.Vec2]bool
}

func (api *testLayeredBlockAPI) GetBlockID(pos vec.Vec2) block.BlockID {
	return api.GetBlockIDLayer(1, pos) // По умолчанию активный слой
}

func (api *testLayeredBlockAPI) SetBlock(pos vec.Vec2, id block.BlockID) {
	api.SetBlockLayer(1, pos, id) // По умолчанию активный слой
}

func (api *testLayeredBlockAPI) GetBlockMetadata(pos vec.Vec2, key string) interface{} {
	if layers, ok := api.world[pos]; ok {
		if b, ok := layers[1]; ok {
			return b.metadata[key]
		}
	}
	return nil
}

func (api *testLayeredBlockAPI) SetBlockMetadata(pos vec.Vec2, key string, value interface{}) {
	if layers, ok := api.world[pos]; ok {
		if b, ok := layers[1]; ok {
			if b.metadata == nil {
				b.metadata = make(map[string]interface{})
			}
			b.metadata[key] = value
			layers[1] = b
		}
	}
}

func (api *testLayeredBlockAPI) GetBlockIDLayer(layer uint8, pos vec.Vec2) block.BlockID {
	if layers, ok := api.world[pos]; ok {
		if b, ok := layers[layer]; ok {
			return b.id
		}
	}
	return block.AirBlockID
}

func (api *testLayeredBlockAPI) SetBlockLayer(layer uint8, pos vec.Vec2, id block.BlockID) {
	if _, ok := api.world[pos]; !ok {
		api.world[pos] = make(map[uint8]testBlock)
	}

	api.world[pos][layer] = testBlock{
		id:       id,
		metadata: make(map[string]interface{}),
	}
}

func (api *testLayeredBlockAPI) ScheduleUpdateOnce(pos vec.Vec2) {
	if api.scheduledOnce == nil {
		api.scheduledOnce = make(map[vec.Vec2]bool)
	}
	api.scheduledOnce[pos] = true
}

func (api *testLayeredBlockAPI) TriggerNeighborUpdates(pos vec.Vec2) {
	neighbors := []vec.Vec2{
		{X: pos.X + 1, Y: pos.Y},
		{X: pos.X - 1, Y: pos.Y},
		{X: pos.X, Y: pos.Y + 1},
		{X: pos.X, Y: pos.Y - 1},
	}
	for _, neighbor := range neighbors {
		api.ScheduleUpdateOnce(neighbor)
	}
}
