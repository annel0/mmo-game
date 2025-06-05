package implementations

import (
	"math/rand"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// GrassBehavior реализует поведение блока травы
type GrassBehavior struct{}

// ID возвращает идентификатор блока
func (b *GrassBehavior) ID() block.BlockID {
	return block.GrassBlockID
}

// Name возвращает имя блока
func (b *GrassBehavior) Name() string {
	return "Grass"
}

// NeedsTick возвращает true, так как трава растет
func (b *GrassBehavior) NeedsTick() bool {
	return true
}

// TickUpdate обновляет состояние травы - постепенный рост и распространение
func (b *GrassBehavior) TickUpdate(api block.BlockAPI, pos vec.Vec2) {
	// Получаем текущий уровень роста
	growth, ok := api.GetBlockMetadata(pos, "growth").(int)
	if !ok {
		// Если метаданных нет или они некорректные, инициализируем
		growth = 0
		api.SetBlockMetadata(pos, "growth", growth)
		return
	}

	// Шанс роста 10% каждый тик, максимальный рост 5
	if growth < 5 && rand.Float32() < 0.1 {
		growth++
		api.SetBlockMetadata(pos, "growth", growth)
	}

	// Если трава достаточно выросла, пытаемся распространиться на соседние блоки земли
	if growth >= 3 && rand.Float32() < 0.05 {
		// Проверяем соседние блоки
		directions := []vec.Vec2{
			{X: pos.X + 1, Y: pos.Y}, // право
			{X: pos.X - 1, Y: pos.Y}, // лево
			{X: pos.X, Y: pos.Y + 1}, // вниз
			{X: pos.X, Y: pos.Y - 1}, // вверх
		}

		// Случайно выбираем направление для распространения
		targetPos := directions[rand.Intn(len(directions))]

		// Проверяем, что блок по указанному направлению - земля
		if api.GetBlockID(targetPos) == block.DirtBlockID {
			// Проверяем влажность земли
			moisture, ok := api.GetBlockMetadata(targetPos, "moisture").(int)
			if ok && moisture >= 2 {
				// Превращаем землю в траву
				api.SetBlock(targetPos, block.GrassBlockID)

				// Инициализируем метаданные новой травы
				api.SetBlockMetadata(targetPos, "growth", 0)
			}
		}
	}
}

// OnPlace инициализирует блок при установке
func (b *GrassBehavior) OnPlace(api block.BlockAPI, pos vec.Vec2) {
	api.SetBlockMetadata(pos, "growth", 0)
}

// OnBreak вызывается при разрушении блока
func (b *GrassBehavior) OnBreak(api block.BlockAPI, pos vec.Vec2) {
	// Ничего не делаем при разрушении
}

// CreateMetadata создает начальные метаданные для блока
func (b *GrassBehavior) CreateMetadata() block.Metadata {
	return block.Metadata{"growth": 0}
}

// HandleInteraction обрабатывает взаимодействие с блоком травы
func (b *GrassBehavior) HandleInteraction(action string, currentPayload, actionPayload map[string]interface{}) (block.BlockID, map[string]interface{}, block.InteractionResult) {
	// Копируем текущие метаданные
	newPayload := make(map[string]interface{})
	for k, v := range currentPayload {
		newPayload[k] = v
	}

	if action == "mine" || action == "dig" {
		// При раскопке трава превращается в землю
		return block.DirtBlockID, map[string]interface{}{"moisture": 2}, block.InteractionResult{
			Success: true,
			Message: "Трава выкопана, обнажилась земля",
			Effects: []string{"particle_grass"},
		}
	} else if action == "use" {
		// Если используется бонмил или другое удобрение
		if tool, ok := actionPayload["tool"].(string); ok && tool == "fertilizer" {
			// Получаем текущий рост
			growth := 0
			if g, ok := currentPayload["growth"].(float64); ok {
				growth = int(g)
			}

			// Увеличиваем рост
			if growth < 5 {
				growth += 1
				newPayload["growth"] = growth

				return block.GrassBlockID, newPayload, block.InteractionResult{
					Success: true,
					Message: "Трава подкормлена и выросла",
					Effects: []string{"particle_fertilizer"},
				}
			}
		}
	}

	// Стандартное взаимодействие не меняет блок
	return block.GrassBlockID, currentPayload, block.InteractionResult{
		Success: false,
		Message: "Действие не поддерживается для травы",
	}
}

func init() {
	block.Register(block.GrassBlockID, &GrassBehavior{})
}
