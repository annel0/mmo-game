package entity

import (
	"math"
	"math/rand"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
)

// NPCBehavior определяет поведение NPC
type NPCBehavior struct {
	// Параметры NPC
	baseSpeed       float64
	maxHealth       int
	detectionRadius float64
	wanderRadius    float64
	idleTimeRange   [2]float64 // Мин/макс время простоя
	moveTimeRange   [2]float64 // Мин/макс время движения
	npcType         string     // Тип NPC (например, "villager", "trader", "guard")
}

// NewNPCBehavior создает новое поведение NPC
func NewNPCBehavior(npcType string) *NPCBehavior {
	behavior := &NPCBehavior{
		baseSpeed:       3.0, // Медленнее игрока
		maxHealth:       50,
		detectionRadius: 8.0, // Блоков
		wanderRadius:    10.0,
		idleTimeRange:   [2]float64{1.0, 5.0}, // 1-5 секунд простоя
		moveTimeRange:   [2]float64{1.0, 3.0}, // 1-3 секунды движения
		npcType:         npcType,
	}

	// Настройка поведения в зависимости от типа NPC
	switch npcType {
	case "villager":
		behavior.baseSpeed = 2.0
		behavior.wanderRadius = 8.0
	case "trader":
		behavior.baseSpeed = 1.5
		behavior.wanderRadius = 3.0
		behavior.idleTimeRange = [2]float64{3.0, 10.0} // Дольше стоит на месте
	case "guard":
		behavior.baseSpeed = 4.0
		behavior.detectionRadius = 12.0
		behavior.wanderRadius = 15.0
	}

	return behavior
}

// Update обновляет состояние NPC
func (nb *NPCBehavior) Update(api EntityAPI, entity *Entity, dt float64) {
	// Обновляем таймеры и состояния
	if entity.Payload["actionTimer"] == nil {
		entity.Payload["actionTimer"] = 0.0
		entity.Payload["state"] = "idle"
		entity.Payload["homePosition"] = entity.Position
		entity.Payload["targetPosition"] = entity.Position
		entity.Payload["randomSeed"] = time.Now().UnixNano()
	}

	// Получаем текущее состояние
	state := entity.Payload["state"].(string)
	actionTimer := entity.Payload["actionTimer"].(float64) - dt

	// Обновляем таймер действия
	if actionTimer <= 0 {
		// Время действия истекло, меняем состояние
		switch state {
		case "idle":
			// Переходим в состояние движения
			entity.Payload["state"] = "moving"
			entity.Payload["actionTimer"] = nb.getRandomInRange(nb.moveTimeRange)

			// Выбираем случайную точку для движения в пределах радиуса блуждания
			homePos, ok := entity.Payload["homePosition"].(vec.Vec2)
			if !ok {
				homePos = entity.Position
				entity.Payload["homePosition"] = homePos
			}

			targetPos := nb.getRandomPositionInRadius(homePos, nb.wanderRadius)
			entity.Payload["targetPosition"] = targetPos
		case "moving":
			// Переходим в состояние покоя
			entity.Payload["state"] = "idle"
			entity.Payload["actionTimer"] = nb.getRandomInRange(nb.idleTimeRange)
		case "following":
			// Возвращаемся к блужданию, если цель пропала
			// Проверяем, есть ли еще игроки в радиусе обнаружения
			playerFound := false
			players := api.GetEntitiesInRange(entity.Position, nb.detectionRadius)
			for _, potentialTarget := range players {
				if potentialTarget.Type == EntityTypePlayer {
					entity.Payload["targetEntityID"] = potentialTarget.ID
					entity.Payload["actionTimer"] = 0.5 // Короткий таймер для обновления пути
					playerFound = true
					break
				}
			}

			if !playerFound {
				// Возвращаемся к обычному поведению
				entity.Payload["state"] = "idle"
				entity.Payload["actionTimer"] = nb.getRandomInRange(nb.idleTimeRange)
			}
		}
	} else {
		// Продолжаем текущее действие
		entity.Payload["actionTimer"] = actionTimer

		// Обрабатываем действия в зависимости от состояния
		switch state {
		case "idle":
			// В состоянии покоя просто стоим
			entity.Velocity = vec.Vec2Float{X: 0, Y: 0}
		case "moving":
			// Двигаемся к целевой точке
			targetPos, ok := entity.Payload["targetPosition"].(vec.Vec2)
			if !ok {
				return
			}

			// Вычисляем направление к цели
			targetPosFloat := vec.FromVec2(targetPos)
			direction := targetPosFloat.Sub(entity.PrecisePos).Normalized()

			// Проверяем, достигли ли мы цели
			if entity.PrecisePos.DistanceTo(targetPosFloat) < 0.5 {
				// Цель достигнута, переходим в состояние покоя
				entity.Payload["state"] = "idle"
				entity.Payload["actionTimer"] = nb.getRandomInRange(nb.idleTimeRange)
				entity.Velocity = vec.Vec2Float{X: 0, Y: 0}
			} else {
				// Продолжаем движение
				entity.Velocity = direction.Mul(nb.baseSpeed)

				// Обновляем направление взгляда
				entity.Direction = calculateDirectionFromVector(direction)

				// Обновляем позицию (упрощенно, без проверки коллизий)
				entity.PrecisePos = entity.PrecisePos.Add(entity.Velocity.Mul(dt))
				entity.Position = entity.PrecisePos.ToVec2()
			}
		case "following":
			// Следуем за целевой сущностью (например, игроком)
			targetID, ok := entity.Payload["targetEntityID"].(uint64)
			if !ok {
				return
			}

			// Получаем целевую сущность
			targetEntity, exists := getEntityByID(api, targetID)
			if !exists {
				// Цель потеряна, возвращаемся к обычному поведению
				entity.Payload["state"] = "idle"
				entity.Payload["actionTimer"] = nb.getRandomInRange(nb.idleTimeRange)
				return
			}

			// Вычисляем направление к цели
			direction := targetEntity.PrecisePos.Sub(entity.PrecisePos).Normalized()

			// Проверяем, достаточно ли мы близко к цели
			distance := entity.PrecisePos.DistanceTo(targetEntity.PrecisePos)
			if distance < 2.0 {
				// Мы достаточно близко, останавливаемся
				entity.Velocity = vec.Vec2Float{X: 0, Y: 0}

				// Смотрим в сторону цели
				entity.Direction = calculateDirectionFromVector(direction)

				// Выполняем действие в зависимости от типа NPC
				switch nb.npcType {
				case "trader":
					// Может предложить торговлю
					if rand.Float64() < 0.01 { // 1% шанс в кадр
						api.SendMessage(targetID, "trade_offer", entity.ID)
					}
				case "guard":
					// Может предупредить игрока или напасть
					// Логика решения пока не реализована
				}
			} else {
				// Продолжаем движение к цели
				entity.Velocity = direction.Mul(nb.baseSpeed)

				// Обновляем направление взгляда
				entity.Direction = calculateDirectionFromVector(direction)

				// Обновляем позицию (упрощенно, без проверки коллизий)
				entity.PrecisePos = entity.PrecisePos.Add(entity.Velocity.Mul(dt))
				entity.Position = entity.PrecisePos.ToVec2()
			}
		}
	}

	// Проверяем, нет ли игроков в радиусе обнаружения (для NPC, которые реагируют на игроков)
	if nb.npcType == "guard" || nb.npcType == "trader" {
		if state != "following" {
			players := api.GetEntitiesInRange(entity.Position, nb.detectionRadius)
			for _, potentialTarget := range players {
				if potentialTarget.Type == EntityTypePlayer {
					// Обнаружен игрок, начинаем следовать за ним
					entity.Payload["state"] = "following"
					entity.Payload["targetEntityID"] = potentialTarget.ID
					entity.Payload["actionTimer"] = 0.5 // Короткий таймер для обновления пути
					break
				}
			}
		}
	}
}

// OnSpawn вызывается при создании NPC
func (nb *NPCBehavior) OnSpawn(api EntityAPI, entity *Entity) {
	// Инициализация данных NPC
	entity.Payload["health"] = nb.maxHealth
	entity.Payload["npcType"] = nb.npcType
	entity.Payload["state"] = "idle"
	entity.Payload["actionTimer"] = nb.getRandomInRange(nb.idleTimeRange)
	entity.Payload["homePosition"] = entity.Position
	entity.Payload["randomSeed"] = time.Now().UnixNano()

	// Дополнительные данные в зависимости от типа NPC
	switch nb.npcType {
	case "trader":
		// Торговцы имеют инвентарь товаров
		entity.Payload["inventory"] = makeTraderInventory()
		entity.Payload["prices"] = makeTraderPrices()
	case "guard":
		// Охранники имеют оружие и защиту
		entity.Payload["weapon"] = "sword"
		entity.Payload["armor"] = 5
	}
}

// OnDespawn вызывается при удалении NPC
func (nb *NPCBehavior) OnDespawn(api EntityAPI, entity *Entity) {
	// Освобождение ресурсов, если необходимо
}

// OnDamage вызывается при получении урона
func (nb *NPCBehavior) OnDamage(api EntityAPI, entity *Entity, damage int, source interface{}) bool {
	if health, ok := entity.Payload["health"].(int); ok {
		newHealth := health - damage
		if newHealth <= 0 {
			// NPC погиб
			entity.Payload["health"] = 0
			return true // Урон привел к смерти
		}
		entity.Payload["health"] = newHealth

		// Реакция на урон в зависимости от типа NPC
		switch nb.npcType {
		case "villager":
			// Сельчане убегают при получении урона
			entity.Payload["state"] = "fleeing"
			entity.Payload["actionTimer"] = 5.0 // Убегаем в течение 5 секунд

			// Убегаем от источника урона, если это сущность
			if sourceEntity, ok := source.(*Entity); ok {
				fleeDirection := entity.PrecisePos.Sub(sourceEntity.PrecisePos).Normalized()
				targetPos := entity.PrecisePos.Add(fleeDirection.Mul(nb.wanderRadius))
				entity.Payload["targetPosition"] = targetPos.ToVec2()
			}
		case "guard":
			// Охранники атакуют в ответ
			if sourceEntity, ok := source.(*Entity); ok {
				entity.Payload["state"] = "attacking"
				entity.Payload["targetEntityID"] = sourceEntity.ID
				entity.Payload["actionTimer"] = 10.0 // Атакуем в течение 10 секунд
			}
		}
	}
	return false // NPC жив
}

// OnCollision вызывается при столкновении с другим объектом
func (nb *NPCBehavior) OnCollision(api EntityAPI, entity *Entity, other interface{}, collisionPoint vec.Vec2Float) {
	// Обработка столкновений
	if entity.Payload["state"] == "moving" {
		// При столкновении во время движения меняем направление
		entity.Payload["state"] = "idle"
		entity.Payload["actionTimer"] = nb.getRandomInRange(nb.idleTimeRange)
	}
}

// GetMoveSpeed возвращает скорость движения NPC
func (nb *NPCBehavior) GetMoveSpeed() float64 {
	return nb.baseSpeed
}

// Вспомогательные функции

// getRandomInRange возвращает случайное число в указанном диапазоне
func (nb *NPCBehavior) getRandomInRange(r [2]float64) float64 {
	return r[0] + rand.Float64()*(r[1]-r[0])
}

// getRandomPositionInRadius возвращает случайную позицию в указанном радиусе от центра
func (nb *NPCBehavior) getRandomPositionInRadius(center vec.Vec2, radius float64) vec.Vec2 {
	// Случайный угол
	angle := rand.Float64() * 2 * math.Pi
	// Случайное расстояние (корень для равномерного распределения по площади)
	distance := radius * math.Sqrt(rand.Float64())

	// Преобразуем в координаты
	x := float64(center.X) + distance*math.Cos(angle)
	y := float64(center.Y) + distance*math.Sin(angle)

	return vec.Vec2{X: int(x), Y: int(y)}
}

// getEntityByID получает сущность по ID через API
func getEntityByID(api EntityAPI, entityID uint64) (*Entity, bool) {
	// Это упрощенная реализация - в реальном коде нужно добавить
	// метод GetEntity в EntityAPI для прямого доступа
	entities := api.GetEntitiesInRange(vec.Vec2{X: 0, Y: 0}, 1000.0) // Большой радиус
	for _, entity := range entities {
		if entity.ID == entityID {
			return entity, true
		}
	}
	return nil, false
}

// calculateDirectionFromVector вычисляет направление взгляда из вектора направления
func calculateDirectionFromVector(direction vec.Vec2Float) int {
	// Для 4 направлений
	if math.Abs(direction.X) > math.Abs(direction.Y) {
		if direction.X > 0 {
			return 1 // Восток
		} else {
			return 3 // Запад
		}
	} else {
		if direction.Y > 0 {
			return 0 // Юг
		} else {
			return 2 // Север
		}
	}
}

// makeTraderInventory создает инвентарь торговца
func makeTraderInventory() map[string]int {
	return map[string]int{
		"potion":   10,
		"food":     20,
		"material": 15,
		"tool":     5,
	}
}

// makeTraderPrices создает цены торговца
func makeTraderPrices() map[string]int {
	return map[string]int{
		"potion":   10,
		"food":     5,
		"material": 8,
		"tool":     25,
	}
}
