package world

import (
	"math/rand"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
)

// AnimalBehavior представляет поведение животного
type AnimalBehavior struct {
	entityID     uint64
	entityData   *EntityData
	worldManager *WorldManager
}

// NewAnimalBehavior создает новое поведение животного
func NewAnimalBehavior(entityID uint64, data *EntityData, wm *WorldManager) *AnimalBehavior {
	return &AnimalBehavior{
		entityID:     entityID,
		entityData:   data,
		worldManager: wm,
	}
}

// Update обновляет состояние животного
func (a *AnimalBehavior) Update(dt float64) {
	// Обновляем счетчики и внутреннее состояние
	hunger, ok := a.entityData.Metadata["hunger"].(float64)
	if !ok {
		hunger = 0
		a.entityData.Metadata["hunger"] = hunger
	}

	// Увеличиваем голод со временем
	a.entityData.Metadata["hunger"] = hunger + dt*0.1 // 10% голода в секунду

	// Если голод превысил определенный порог, ищем траву для поедания
	if hunger > 50 {
		a.searchForFood()
	} else {
		// Случайное перемещение
		a.randomMovement(dt)
	}
}

// searchForFood ищет блоки травы и перемещается к ним
func (a *AnimalBehavior) searchForFood() {
	position := a.entityData.Position

	// Ищем траву в радиусе 5 блоков
	for dx := -5; dx <= 5; dx++ {
		for dy := -5; dy <= 5; dy++ {
			blockPos := vec.Vec2{X: position.X + dx, Y: position.Y + dy}

			// Получаем блок по координатам
			block := a.worldManager.GetBlock(blockPos)

			// Проверяем, является ли блок травой (ID 2 - трава)
			if uint16(block.ID) == 2 {
				// Нашли траву, перемещаемся к ней
				a.moveTowards(blockPos)

				// Если достигли блока, съедаем траву
				if position.X == blockPos.X && position.Y == blockPos.Y {
					a.eatGrass(blockPos)
				}

				return
			}
		}
	}

	// Если не нашли траву, случайно перемещаемся
	a.randomMovement(1.0)
}

// moveTowards перемещает животное в направлении целевой позиции
func (a *AnimalBehavior) moveTowards(target vec.Vec2) {
	position := a.entityData.Position

	// Определяем направление движения
	dx := 0
	dy := 0

	if target.X > position.X {
		dx = 1
	} else if target.X < position.X {
		dx = -1
	}

	if target.Y > position.Y {
		dy = 1
	} else if target.Y < position.Y {
		dy = -1
	}

	// Перемещаемся только в одном направлении за раз
	if dx != 0 && dy != 0 {
		if rand.Float64() < 0.5 {
			dy = 0
		} else {
			dx = 0
		}
	}

	// Перемещаем животное
	newPos := vec.Vec2{X: position.X + dx, Y: position.Y + dy}
	a.entityData.Position = newPos

	// Отправляем событие перемещения
	event := EntityEvent{
		EventType:   EventTypeEntityMove,
		EntityID:    a.entityID,
		Position:    newPos,
		SourceChunk: position.ToBigChunkCoords(),
		TargetChunk: newPos.ToBigChunkCoords(),
	}

	a.worldManager.HandleEntityEvent(event)
}

// eatGrass съедает траву и меняет блок на землю
func (a *AnimalBehavior) eatGrass(pos vec.Vec2) {
	// Уменьшаем голод
	a.entityData.Metadata["hunger"] = 0.0

	// Меняем траву на землю (ID 1 - земля)
	dirt := Block{ID: 1, Payload: make(map[string]interface{})}
	a.worldManager.SetBlock(pos, dirt)

	// Логика воспроизводства, если сыты и прошло достаточно времени
	lastBreed, ok := a.entityData.Metadata["lastBreed"].(int64)
	if !ok {
		lastBreed = 0
		a.entityData.Metadata["lastBreed"] = lastBreed
	}

	now := time.Now().Unix()
	if now-lastBreed > 300 { // 5 минут между воспроизводством
		// 10% шанс на воспроизводство
		if rand.Float64() < 0.1 {
			a.reproduce()
			a.entityData.Metadata["lastBreed"] = now
		}
	}
}

// reproduce создает новое животное рядом с текущим
func (a *AnimalBehavior) reproduce() {
	position := a.entityData.Position

	// Случайное смещение для нового животного
	offset := []vec.Vec2{
		{X: 1, Y: 0},
		{X: -1, Y: 0},
		{X: 0, Y: 1},
		{X: 0, Y: -1},
	}[rand.Intn(4)]

	newPos := vec.Vec2{X: position.X + offset.X, Y: position.Y + offset.Y}

	// Копируем метаданные для нового животного
	newMetadata := make(map[string]interface{})
	for k, v := range a.entityData.Metadata {
		newMetadata[k] = v
	}
	newMetadata["age"] = 0
	newMetadata["hunger"] = 0.0

	// Создаем новое животное
	a.worldManager.SpawnEntity(a.entityData.Type, newPos, newMetadata)
}

// randomMovement осуществляет случайное перемещение животного
func (a *AnimalBehavior) randomMovement(dt float64) {
	// Случайное перемещение только с определенной вероятностью
	if rand.Float64() > dt*0.2 { // 20% шанс в секунду
		return
	}

	// Возможные направления
	directions := []vec.Vec2{
		{X: 0, Y: 0},  // Оставаться на месте
		{X: 1, Y: 0},  // Вправо
		{X: -1, Y: 0}, // Влево
		{X: 0, Y: 1},  // Вниз
		{X: 0, Y: -1}, // Вверх
	}

	// Выбираем случайное направление
	dir := directions[rand.Intn(len(directions))]
	if dir.X == 0 && dir.Y == 0 {
		return
	}

	// Новая позиция
	position := a.entityData.Position
	newPos := vec.Vec2{X: position.X + dir.X, Y: position.Y + dir.Y}
	a.entityData.Position = newPos

	// Отправляем событие перемещения
	event := EntityEvent{
		EventType:   EventTypeEntityMove,
		EntityID:    a.entityID,
		Position:    newPos,
		SourceChunk: position.ToBigChunkCoords(),
		TargetChunk: newPos.ToBigChunkCoords(),
	}

	a.worldManager.HandleEntityEvent(event)
}
