package entity

import (
	"strconv"

	"github.com/annel0/mmo-game/internal/vec"
)

// PlayerBehavior определяет поведение игрока
type PlayerBehavior struct {
	// Параметры игрока
	baseSpeed      float64
	maxHealth      int
	inventorySize  int
	attackRange    float64
	attackDamage   int
	attackCooldown float64
}

// NewPlayerBehavior создает новое поведение игрока
func NewPlayerBehavior() *PlayerBehavior {
	return &PlayerBehavior{
		baseSpeed:      5.0, // Блоков в секунду
		maxHealth:      100,
		inventorySize:  36,
		attackRange:    1.5,
		attackDamage:   10,
		attackCooldown: 0.5, // Секунды между атаками
	}
}

// Update обновляет состояние игрока
func (pb *PlayerBehavior) Update(api EntityAPI, entity *Entity, dt float64) {
	// Обновление таймеров
	if cooldown, exists := entity.Payload["attackCooldown"].(float64); exists && cooldown > 0 {
		entity.Payload["attackCooldown"] = cooldown - dt
	}

	// Обновление других состояний игрока
	// Применение эффектов, регенерация, восстановление энергии и т.д.
}

// OnSpawn вызывается при создании игрока
func (pb *PlayerBehavior) OnSpawn(api EntityAPI, entity *Entity) {
	// Инициализация данных игрока
	entity.Payload["health"] = pb.maxHealth
	entity.Payload["inventory"] = make(map[string]interface{})
	entity.Payload["experience"] = 0
	entity.Payload["level"] = 1
	entity.Payload["username"] = "Player" + strconv.FormatUint(entity.ID, 10)
	entity.Payload["attackCooldown"] = 0.0
	entity.Payload["lastAttackTime"] = 0.0
}

// OnDespawn вызывается при удалении игрока
func (pb *PlayerBehavior) OnDespawn(api EntityAPI, entity *Entity) {
	// Сохранение данных игрока перед удалением
	// Здесь может быть логика сохранения прогресса игрока
}

// OnDamage вызывается при получении урона
func (pb *PlayerBehavior) OnDamage(api EntityAPI, entity *Entity, damage int, source interface{}) bool {
	if health, ok := entity.Payload["health"].(int); ok {
		newHealth := health - damage
		if newHealth <= 0 {
			// Игрок погиб
			entity.Payload["health"] = 0
			return true // Урон привел к смерти
		}
		entity.Payload["health"] = newHealth
	}
	return false // Игрок жив
}

// OnCollision вызывается при столкновении с другим объектом
func (pb *PlayerBehavior) OnCollision(api EntityAPI, entity *Entity, other interface{}, collisionPoint vec.Vec2Float) {
	// Обработка столкновений игрока
	// В зависимости от типа объекта, с которым произошло столкновение
	// Например, сбор предметов, взаимодействие с NPC и т.д.
}

// GetMoveSpeed возвращает скорость движения игрока
func (pb *PlayerBehavior) GetMoveSpeed() float64 {
	return pb.baseSpeed
}

// AddItemToInventory добавляет предмет в инвентарь игрока
func (pb *PlayerBehavior) AddItemToInventory(entity *Entity, itemID string, count int) bool {
	if inventory, ok := entity.Payload["inventory"].(map[string]interface{}); ok {
		if currentCount, exists := inventory[itemID].(int); exists {
			inventory[itemID] = currentCount + count
		} else {
			inventory[itemID] = count
		}
		return true
	}
	return false
}

// GetInventoryItem получает количество предмета в инвентаре
func (pb *PlayerBehavior) GetInventoryItem(entity *Entity, itemID string) int {
	if inventory, ok := entity.Payload["inventory"].(map[string]interface{}); ok {
		if count, exists := inventory[itemID].(int); exists {
			return count
		}
	}
	return 0
}

// Attack выполняет атаку в текущем направлении
func (pb *PlayerBehavior) Attack(api EntityAPI, entity *Entity) bool {
	// Проверка кулдауна
	if cooldown, exists := entity.Payload["attackCooldown"].(float64); exists && cooldown > 0 {
		return false // Атака на кулдауне
	}

	// Определение области атаки в зависимости от направления
	attackDirection := getDirectionVector(entity.Direction)
	attackCenter := entity.PrecisePos.Add(attackDirection.Mul(pb.attackRange / 2))

	// Получение сущностей в радиусе атаки
	entitiesInRange := api.GetEntitiesInRange(attackCenter.ToVec2(), pb.attackRange)

	// Флаг успешной атаки
	hitSomething := false

	// Применение урона к сущностям
	for _, target := range entitiesInRange {
		if target.ID == entity.ID {
			continue // Пропускаем самого игрока
		}

		// Проверка, находится ли цель в конусе атаки
		if isInAttackCone(entity.PrecisePos, target.PrecisePos, attackDirection, pb.attackRange, 90) {
			if behavior, exists := api.GetBehavior(target.Type); exists {
				behavior.OnDamage(api, target, pb.attackDamage, entity)
				hitSomething = true
			}
		}
	}

	// Установка кулдауна атаки
	entity.Payload["attackCooldown"] = pb.attackCooldown

	return hitSomething
}

// Вспомогательные функции

// getDirectionVector возвращает вектор направления по номеру
func getDirectionVector(direction int) vec.Vec2Float {
	switch direction {
	case 0: // Вниз (юг)
		return vec.Vec2Float{X: 0, Y: 1}
	case 1: // Вправо (восток)
		return vec.Vec2Float{X: 1, Y: 0}
	case 2: // Вверх (север)
		return vec.Vec2Float{X: 0, Y: -1}
	case 3: // Влево (запад)
		return vec.Vec2Float{X: -1, Y: 0}
	case 4: // Юго-восток
		return vec.Vec2Float{X: 0.7071, Y: 0.7071} // √2/2 для диагонали
	case 5: // Северо-восток
		return vec.Vec2Float{X: 0.7071, Y: -0.7071}
	case 6: // Северо-запад
		return vec.Vec2Float{X: -0.7071, Y: -0.7071}
	case 7: // Юго-запад
		return vec.Vec2Float{X: -0.7071, Y: 0.7071}
	default:
		return vec.Vec2Float{X: 0, Y: 0}
	}
}

// isInAttackCone проверяет, находится ли точка в конусе атаки
func isInAttackCone(origin, target, direction vec.Vec2Float, range_ float64, angleDegrees float64) bool {
	// Вектор от источника к цели
	toTarget := target.Sub(origin)

	// Проверка дистанции
	if toTarget.Length() > range_ {
		return false
	}

	// Нормализация векторов
	normDirection := direction.Normalized()
	normToTarget := toTarget.Normalized()

	// Скалярное произведение (косинус угла между векторами)
	dotProduct := normDirection.X*normToTarget.X + normDirection.Y*normToTarget.Y

	// Преобразование градусов в косинус
	cosAngle := cos(angleDegrees / 2)

	// Точка находится в конусе, если косинус угла между векторами больше cosAngle
	return dotProduct >= cosAngle
}

// cos вычисляет косинус угла в градусах
func cos(angleDegrees float64) float64 {
	// Приближение косинуса для упрощения (в реальном коде лучше использовать math.Cos)
	// Для 45 градусов - примерно 0.7071
	// Для 90 градусов - примерно 0
	if angleDegrees <= 45 {
		return 0.7071
	} else if angleDegrees <= 90 {
		return 0
	}
	return -1 // Для больших углов
}
