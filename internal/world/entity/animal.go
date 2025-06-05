package entity

import (
	"math"
	"math/rand"
	"time"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// AnimalBehavior определяет поведение животного
type AnimalBehavior struct {
	// Параметры животного
	baseSpeed       float64
	maxHealth       int
	detectionRadius float64
	wanderRadius    float64
	idleTimeRange   [2]float64 // Мин/макс время простоя
	moveTimeRange   [2]float64 // Мин/макс время движения
	animalType      AnimalType // Тип животного
	maxHunger       int        // Максимальный уровень сытости
	hungerRate      float64    // Скорость увеличения голода (единиц в секунду)
	eatDuration     float64    // Время поедания пищи в секундах
}

// NewAnimalBehavior создает новое поведение животного
func NewAnimalBehavior(animalType AnimalType) *AnimalBehavior {
	behavior := &AnimalBehavior{
		baseSpeed:       2.0, // Блоков в секунду
		maxHealth:       30,
		detectionRadius: 6.0, // Блоков
		wanderRadius:    10.0,
		idleTimeRange:   [2]float64{2.0, 7.0}, // 2-7 секунд простоя
		moveTimeRange:   [2]float64{1.0, 4.0}, // 1-4 секунды движения
		animalType:      animalType,
		maxHunger:       100,
		hungerRate:      0.05, // 0.05 единиц голода в секунду
		eatDuration:     3.0,  // 3 секунды на поедание
	}

	// Настройка поведения в зависимости от типа животного
	switch animalType {
	case AnimalTypeCow:
		behavior.baseSpeed = 1.5
		behavior.maxHealth = 40
		behavior.maxHunger = 150
		behavior.eatDuration = 4.0 // Коровы едят дольше
	case AnimalTypeSheep:
		behavior.baseSpeed = 1.8
		behavior.maxHealth = 25
	case AnimalTypeChicken:
		behavior.baseSpeed = 2.2
		behavior.maxHealth = 10
		behavior.maxHunger = 50
	case AnimalTypePig:
		behavior.baseSpeed = 1.7
		behavior.maxHealth = 30
	case AnimalTypeHorse:
		behavior.baseSpeed = 4.0
		behavior.maxHealth = 60
	}

	return behavior
}

// Update обновляет состояние животного
func (ab *AnimalBehavior) Update(api EntityAPI, entity *Entity, dt float64) {
	// Инициализация данных животного, если нужно
	ab.initAnimalData(entity)

	// Обновляем голод
	hunger := entity.Payload["hunger"].(int)
	hunger += int(ab.hungerRate * dt * 100) // Умножаем на 100 для более плавного изменения
	if hunger > ab.maxHunger*100 {
		hunger = ab.maxHunger * 100
	}
	entity.Payload["hunger"] = hunger

	// Получаем текущее состояние
	state := entity.Payload["state"].(string)
	actionTimer := entity.Payload["actionTimer"].(float64) - dt

	// Обработка особых состояний для коровы
	if ab.animalType == AnimalTypeCow {
		ab.updateCowState(api, entity, dt, state, &actionTimer)
	} else {
		// Обновляем таймер действия для остальных животных
		if actionTimer <= 0 {
			ab.updateBaseState(entity, state, &actionTimer)
		} else {
			entity.Payload["actionTimer"] = actionTimer
			ab.processState(api, entity, state, dt)
		}
	}
}

// updateCowState обновляет состояние коровы
func (ab *AnimalBehavior) updateCowState(api EntityAPI, entity *Entity, dt float64, state string, actionTimer *float64) {
	// Проверяем, голодна ли корова (голод > 40%)
	hunger := entity.Payload["hunger"].(int)

	if hunger > ab.maxHunger*40 && state != "eating" {
		// Ищем ближайшую траву, если корова голодна и не в состоянии поедания
		if grass := ab.findNearbyGrass(api, entity); grass.X != 0 || grass.Y != 0 {
			// Переходим в состояние "движение к траве"
			entity.Payload["state"] = "moving_to_grass"
			entity.Payload["targetPosition"] = grass
			entity.Payload["actionTimer"] = 10.0 // Максимум 10 секунд для достижения травы
			return
		}
	}

	// Обновляем таймер действия
	if *actionTimer <= 0 {
		// Время действия истекло, меняем состояние
		switch state {
		case "eating":
			// Корова закончила есть, переходим в состояние покоя
			entity.Payload["state"] = "idle"
			*actionTimer = ab.getRandomInRange(ab.idleTimeRange)

			// Уменьшаем голод
			hunger = hunger - 30*100 // Уменьшаем голод на 30% от максимума
			if hunger < 0 {
				hunger = 0
			}
			entity.Payload["hunger"] = hunger
		case "moving_to_grass":
			// Не достигли травы за отведенное время, переходим к блужданию
			entity.Payload["state"] = "idle"
			*actionTimer = ab.getRandomInRange(ab.idleTimeRange)
		default:
			// Обычное обновление состояния
			ab.updateBaseState(entity, state, actionTimer)
		}
	} else {
		// Продолжаем текущее действие
		entity.Payload["actionTimer"] = *actionTimer

		// Обрабатываем состояния
		switch state {
		case "eating":
			// В состоянии поедания просто ждем
			entity.Velocity = vec.Vec2Float{X: 0, Y: 0}
		case "moving_to_grass":
			// Двигаемся к траве
			targetPos, ok := entity.Payload["targetPosition"].(vec.Vec2)
			if !ok {
				return
			}

			// Если достигли травы, начинаем есть
			targetPosFloat := vec.FromVec2(targetPos)
			if entity.PrecisePos.DistanceTo(targetPosFloat) < 0.8 {
				// Проверяем, все еще ли там трава
				blockID := api.GetBlock(targetPos)
				if blockID == block.GrassBlockID {
					// Начинаем есть траву
					entity.Payload["state"] = "eating"
					entity.Payload["actionTimer"] = ab.eatDuration
					entity.Velocity = vec.Vec2Float{X: 0, Y: 0}

					// Уменьшаем уровень травы
					growth, ok := api.GetBlockMetadata(targetPos, "growth").(int)
					if ok && growth > 0 {
						api.SetBlockMetadata(targetPos, "growth", growth-1)

						// Если трава полностью съедена, превращаем ее в грязь или землю
						if growth <= 1 {
							api.SetBlock(targetPos, block.SandBlockID) // Земля или грязь
						}
					}
				} else {
					// Трава уже исчезла, переходим в состояние покоя
					entity.Payload["state"] = "idle"
					entity.Payload["actionTimer"] = ab.getRandomInRange(ab.idleTimeRange)
				}
			} else {
				// Продолжаем движение к траве
				direction := targetPosFloat.Sub(entity.PrecisePos).Normalized()
				entity.Velocity = direction.Mul(ab.baseSpeed)

				// Обновляем направление взгляда
				entity.Direction = ab.calculateDirection(direction)
			}
		default:
			// Обычная обработка состояний
			ab.processState(api, entity, state, dt)
		}
	}
}

// findNearbyGrass ищет ближайшую траву вокруг животного
func (ab *AnimalBehavior) findNearbyGrass(api EntityAPI, entity *Entity) vec.Vec2 {
	// Радиус поиска травы
	searchRadius := 8.0 // Блоков

	// Центр поиска - текущая позиция животного
	centerPos := entity.Position

	// Проверяем блоки в радиусе поиска
	for y := centerPos.Y - int(searchRadius); y <= centerPos.Y+int(searchRadius); y++ {
		for x := centerPos.X - int(searchRadius); x <= centerPos.X+int(searchRadius); x++ {
			pos := vec.Vec2{X: x, Y: y}

			// Проверяем, находится ли блок в радиусе поиска
			distanceSquared := float64((x-centerPos.X)*(x-centerPos.X) + (y-centerPos.Y)*(y-centerPos.Y))
			if distanceSquared > searchRadius*searchRadius {
				continue
			}

			// Проверяем, является ли блок травой
			blockID := api.GetBlock(pos)
			if blockID == block.GrassBlockID {
				// Проверяем, достаточно ли выросла трава
				growth, ok := api.GetBlockMetadata(pos, "growth").(int)
				if ok && growth >= 3 { // Минимальный уровень роста для поедания
					return pos
				}
			}
		}
	}

	// Трава не найдена
	return vec.Vec2{X: 0, Y: 0}
}

// updateBaseState обновляет базовое состояние животного
func (ab *AnimalBehavior) updateBaseState(entity *Entity, state string, actionTimer *float64) {
	// Время действия истекло, меняем состояние
	switch state {
	case "idle":
		// Переходим в состояние движения
		entity.Payload["state"] = "moving"
		*actionTimer = ab.getRandomInRange(ab.moveTimeRange)

		// Выбираем случайную точку для движения
		homePos, ok := entity.Payload["homePosition"].(vec.Vec2)
		if !ok {
			homePos = entity.Position
			entity.Payload["homePosition"] = homePos
		}

		targetPos := ab.getRandomPositionInRadius(homePos, ab.wanderRadius)
		entity.Payload["targetPosition"] = targetPos
	case "moving":
		// Переходим в состояние покоя
		entity.Payload["state"] = "idle"
		*actionTimer = ab.getRandomInRange(ab.idleTimeRange)
	}
}

// processState обрабатывает текущее состояние животного
func (ab *AnimalBehavior) processState(api EntityAPI, entity *Entity, state string, dt float64) {
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
			entity.Payload["actionTimer"] = ab.getRandomInRange(ab.idleTimeRange)
			entity.Velocity = vec.Vec2Float{X: 0, Y: 0}
		} else {
			// Продолжаем движение
			entity.Velocity = direction.Mul(ab.baseSpeed)

			// Обновляем направление взгляда
			entity.Direction = ab.calculateDirection(direction)
		}
	}
}

// OnSpawn вызывается при создании животного
func (ab *AnimalBehavior) OnSpawn(api EntityAPI, entity *Entity) {
	// Инициализация данных животного
	ab.initAnimalData(entity)
}

// initAnimalData инициализирует данные животного, если нужно
func (ab *AnimalBehavior) initAnimalData(entity *Entity) {
	if entity.Payload["animalType"] == nil {
		entity.Payload["animalType"] = int(ab.animalType)
		entity.Payload["health"] = ab.maxHealth
		entity.Payload["state"] = "idle"
		entity.Payload["actionTimer"] = ab.getRandomInRange(ab.idleTimeRange)
		entity.Payload["homePosition"] = entity.Position
		entity.Payload["hunger"] = 0 // Начальный голод
		entity.Payload["lastEatTime"] = 0.0
		entity.Payload["randomSeed"] = time.Now().UnixNano()
	}
}

// OnDespawn вызывается при удалении животного
func (ab *AnimalBehavior) OnDespawn(api EntityAPI, entity *Entity) {
	// Освобождение ресурсов, если необходимо
}

// OnDamage вызывается при получении урона
func (ab *AnimalBehavior) OnDamage(api EntityAPI, entity *Entity, damage int, source interface{}) bool {
	if health, ok := entity.Payload["health"].(int); ok {
		newHealth := health - damage
		if newHealth <= 0 {
			// Животное погибло
			entity.Payload["health"] = 0
			return true // Урон привел к смерти
		}
		entity.Payload["health"] = newHealth

		// Животное убегает при получении урона
		entity.Payload["state"] = "fleeing"
		entity.Payload["actionTimer"] = 5.0 // Убегаем в течение 5 секунд

		// Убегаем от источника урона, если это сущность
		if sourceEntity, ok := source.(*Entity); ok {
			fleeDirection := entity.PrecisePos.Sub(sourceEntity.PrecisePos).Normalized()
			targetPos := entity.PrecisePos.Add(fleeDirection.Mul(ab.wanderRadius))
			entity.Payload["targetPosition"] = targetPos.ToVec2()
		}
	}
	return false // Животное живо
}

// OnCollision вызывается при столкновении с другим объектом
func (ab *AnimalBehavior) OnCollision(api EntityAPI, entity *Entity, other interface{}, collisionPoint vec.Vec2Float) {
	// Обработка столкновений
	if entity.Payload["state"] == "moving" {
		// При столкновении во время движения меняем направление
		entity.Payload["state"] = "idle"
		entity.Payload["actionTimer"] = ab.getRandomInRange(ab.idleTimeRange)
	}
}

// GetMoveSpeed возвращает скорость движения животного
func (ab *AnimalBehavior) GetMoveSpeed() float64 {
	return ab.baseSpeed
}

// Вспомогательные функции

// getRandomInRange возвращает случайное число в указанном диапазоне
func (ab *AnimalBehavior) getRandomInRange(r [2]float64) float64 {
	return r[0] + rand.Float64()*(r[1]-r[0])
}

// getRandomPositionInRadius возвращает случайную позицию в указанном радиусе от центра
func (ab *AnimalBehavior) getRandomPositionInRadius(center vec.Vec2, radius float64) vec.Vec2 {
	// Случайный угол
	angle := rand.Float64() * 2 * math.Pi
	// Случайное расстояние (корень для равномерного распределения по площади)
	distance := radius * math.Sqrt(rand.Float64())

	// Преобразуем в координаты
	x := float64(center.X) + distance*math.Cos(angle)
	y := float64(center.Y) + distance*math.Sin(angle)

	return vec.Vec2{X: int(x), Y: int(y)}
}

// calculateDirection вычисляет направление взгляда из вектора направления
func (ab *AnimalBehavior) calculateDirection(direction vec.Vec2Float) int {
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
