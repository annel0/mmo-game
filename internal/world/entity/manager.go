package entity

import (
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"github.com/annel0/mmo-game/internal/vec"
	"github.com/annel0/mmo-game/internal/world/block"
)

// EntityManager управляет всеми сущностями в мире
type EntityManager struct {
	entities     map[uint64]*Entity            // Хранилище всех сущностей
	behaviors    map[EntityType]EntityBehavior // Реестр поведений сущностей
	nextEntityID uint64                        // Счетчик для генерации ID
	mu           sync.RWMutex                  // Мьютекс для безопасного доступа
}

// NewEntityManager создаёт новый менеджер сущностей
func NewEntityManager() *EntityManager {
	return &EntityManager{
		entities:     make(map[uint64]*Entity),
		behaviors:    make(map[EntityType]EntityBehavior),
		nextEntityID: 1,
		mu:           sync.RWMutex{},
	}
}

// RegisterBehavior регистрирует поведение для типа сущности
func (em *EntityManager) RegisterBehavior(entityType EntityType, behavior EntityBehavior) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.behaviors[entityType] = behavior
}

// RegisterDefaultBehaviors регистрирует поведения по умолчанию
func (em *EntityManager) RegisterDefaultBehaviors() {
	// Регистрируем базовые типы сущностей
	em.RegisterBehavior(EntityTypePlayer, NewPlayerBehavior())
	em.RegisterBehavior(EntityTypeNPC, NewNPCBehavior("villager"))
	em.RegisterBehavior(EntityTypeNPC, NewNPCBehavior("trader"))
	em.RegisterBehavior(EntityTypeNPC, NewNPCBehavior("guard"))

	// Регистрируем животных
	em.RegisterBehavior(EntityTypeAnimal, NewAnimalBehavior(AnimalTypeCow))
	em.RegisterBehavior(EntityTypeAnimal, NewAnimalBehavior(AnimalTypeSheep))
	em.RegisterBehavior(EntityTypeAnimal, NewAnimalBehavior(AnimalTypeChicken))
	em.RegisterBehavior(EntityTypeAnimal, NewAnimalBehavior(AnimalTypePig))
}

// SpawnEntity создаёт новую сущность в мире
func (em *EntityManager) SpawnEntity(entityType EntityType, position vec.Vec2, api EntityAPI) uint64 {
	em.mu.Lock()
	defer em.mu.Unlock()

	// Генерируем уникальный ID
	entityID := atomic.AddUint64(&em.nextEntityID, 1)

	// Создаём сущность
	entity := NewEntity(entityID, entityType, position)
	em.entities[entityID] = entity

	// Вызываем OnSpawn, если есть поведение
	if behavior, exists := em.behaviors[entityType]; exists {
		behavior.OnSpawn(api, entity)
	}

	return entityID
}

// SpawnAnimal создает новое животное указанного типа
func (em *EntityManager) SpawnAnimal(animalType AnimalType, position vec.Vec2, api EntityAPI) uint64 {
	// Создаем сущность с типом животного
	entity := &Entity{
		Type:       EntityTypeAnimal,
		Position:   position,
		PrecisePos: vec.FromVec2(position),
		Direction:  0,
		Velocity:   vec.Vec2Float{X: 0, Y: 0},
		Active:     true,
		Payload:    make(map[string]interface{}),
	}

	// Добавляем подтип животного
	entity.Payload["animalType"] = int(animalType)

	// Получаем ID и добавляем в карту сущностей
	em.mu.Lock()
	entity.ID = em.nextEntityID
	em.nextEntityID++
	em.entities[entity.ID] = entity
	em.mu.Unlock()

	// Получаем поведение для животного
	behavior, _ := em.GetBehavior(EntityTypeAnimal)

	// Инициализируем сущность
	behavior.OnSpawn(api, entity)

	return entity.ID
}

// DespawnEntity удаляет сущность из мира
func (em *EntityManager) DespawnEntity(entityID uint64, api EntityAPI) bool {
	em.mu.Lock()
	defer em.mu.Unlock()

	entity, exists := em.entities[entityID]
	if !exists {
		return false
	}

	// Вызываем OnDespawn, если есть поведение
	if behavior, exists := em.behaviors[entity.Type]; exists {
		behavior.OnDespawn(api, entity)
	}

	// Удаляем сущность
	delete(em.entities, entityID)
	return true
}

// GetEntity возвращает сущность по ID
func (em *EntityManager) GetEntity(entityID uint64) (*Entity, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	entity, exists := em.entities[entityID]
	return entity, exists
}

// GetEntitiesInRange возвращает сущности в указанном радиусе
func (em *EntityManager) GetEntitiesInRange(center vec.Vec2, radius float64) []*Entity {
	em.mu.RLock()
	defer em.mu.RUnlock()

	var result []*Entity
	centerFloat := vec.FromVec2(center)

	for _, entity := range em.entities {
		if entity.Active && centerFloat.DistanceTo(entity.PrecisePos) <= radius {
			result = append(result, entity)
		}
	}

	return result
}

// GetBehavior возвращает поведение для типа сущности
func (em *EntityManager) GetBehavior(entityType EntityType) (EntityBehavior, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	behavior, exists := em.behaviors[entityType]
	return behavior, exists
}

// UpdateEntities обновляет все активные сущности
func (em *EntityManager) UpdateEntities(dt float64, api EntityAPI) {
	// Держим блокировку на всё время обновления для избежания race conditions
	em.mu.Lock()
	defer em.mu.Unlock()

	// Обновляем каждую сущность
	for _, entity := range em.entities {
		if entity.Active {
			if behavior, exists := em.behaviors[entity.Type]; exists {
				behavior.Update(api, entity, dt)
			}
		}
	}
}

// ProcessMovement обрабатывает перемещение сущности
func (em *EntityManager) ProcessMovement(entityID uint64, direction MovementDirection, dt float64, api EntityAPI) bool {
	entity, exists := em.GetEntity(entityID)
	if !exists || !entity.Active {
		return false
	}

	behavior, exists := em.GetBehavior(entity.Type)
	if !exists {
		return false
	}

	// Получаем скорость движения из поведения
	moveSpeed := behavior.GetMoveSpeed()

	// Вычисляем вектор направления
	moveDir := vec.Vec2Float{X: 0, Y: 0}

	if direction.Up {
		moveDir.Y -= 1
	}
	if direction.Down {
		moveDir.Y += 1
	}
	if direction.Left {
		moveDir.X -= 1
	}
	if direction.Right {
		moveDir.X += 1
	}

	// Обновляем направление взгляда сущности
	if moveDir.X != 0 || moveDir.Y != 0 {
		entity.Direction = calculateDirection(moveDir)
	}

	// Нормализуем вектор направления для диагонального движения
	if moveDir.X != 0 || moveDir.Y != 0 {
		moveDir = moveDir.Normalized()
	}

	// Вычисляем новую позицию
	newVelocity := moveDir.Mul(moveSpeed)
	newPos := entity.PrecisePos.Add(newVelocity.Mul(dt))

	// Проверяем коллизии по осям X и Y отдельно для правильного скольжения вдоль стен
	newPosX := vec.Vec2Float{X: newPos.X, Y: entity.PrecisePos.Y}
	newPosY := vec.Vec2Float{X: entity.PrecisePos.X, Y: newPos.Y}

	// Проверяем коллизии по каждой оси отдельно
	collisionX := em.checkCollision(entity, newPosX, api)
	collisionY := em.checkCollision(entity, newPosY, api)

	// Применяем движение по осям, которые не имеют коллизий
	finalPos := entity.PrecisePos

	if !collisionX {
		finalPos.X = newPos.X
	}
	if !collisionY {
		finalPos.Y = newPos.Y
	}

	// Применяем новую позицию
	em.mu.Lock()
	if entityInMap, exists := em.entities[entity.ID]; exists {
		entityInMap.PrecisePos = finalPos
		entityInMap.Position = finalPos.ToVec2()

		// Устанавливаем скорость только по осям без коллизий
		if !collisionX && !collisionY {
			entityInMap.Velocity = newVelocity
		} else {
			// Если есть коллизия, то корректируем скорость
			var velocityX, velocityY float64
			if collisionX {
				velocityX = 0
			} else {
				velocityX = newVelocity.X
			}
			if collisionY {
				velocityY = 0
			} else {
				velocityY = newVelocity.Y
			}

			entityInMap.Velocity = vec.Vec2Float{
				X: velocityX,
				Y: velocityY,
			}
		}
	}
	em.mu.Unlock()

	// Движение было успешным, если хотя бы по одной оси произошло смещение
	return !collisionX || !collisionY
}

// checkCollision проверяет коллизии сущности с блоками мира
func (em *EntityManager) checkCollision(entity *Entity, newPos vec.Vec2Float, api EntityAPI) bool {
	// Получаем размеры сущности для хитбокса
	halfWidth := entity.Size.X / 2
	halfHeight := entity.Size.Y / 2

	// Проверяем углы хитбокса
	corners := []vec.Vec2Float{
		{X: newPos.X - halfWidth, Y: newPos.Y - halfHeight}, // Левый верхний
		{X: newPos.X + halfWidth, Y: newPos.Y - halfHeight}, // Правый верхний
		{X: newPos.X - halfWidth, Y: newPos.Y + halfHeight}, // Левый нижний
		{X: newPos.X + halfWidth, Y: newPos.Y + halfHeight}, // Правый нижний
	}

	// Проверяем каждый угол на коллизию с блоками
	for _, corner := range corners {
		blockPos := corner.ToVec2()
		blockID := api.GetBlock(blockPos)

		// Проверяем, является ли блок препятствием
		if !isPassableBlock(blockID) {
			// Вызываем обработчик коллизии, если есть поведение
			if behavior, exists := em.GetBehavior(entity.Type); exists {
				behavior.OnCollision(api, entity, blockID, corner)
			}
			return true
		}
	}

	// Также проверяем коллизии с другими сущностями
	for _, otherEntity := range em.entities {
		if otherEntity.ID == entity.ID || !otherEntity.Active {
			continue // Пропускаем себя и неактивные сущности
		}

		// Простая проверка пересечения хитбоксов (можно улучшить)
		if entitiesCollide(newPos, entity.Size, otherEntity.PrecisePos, otherEntity.Size) {
			// Вызываем обработчик коллизии, если есть поведение
			if behavior, exists := em.GetBehavior(entity.Type); exists {
				collisionPoint := calculateCollisionPoint(newPos, otherEntity.PrecisePos)
				behavior.OnCollision(api, entity, otherEntity, collisionPoint)
			}
			return true
		}
	}

	return false
}

// calculateDirection определяет направление взгляда по вектору движения
func calculateDirection(moveDir vec.Vec2Float) int {
	// Для 4 направлений
	if math.Abs(moveDir.X) > math.Abs(moveDir.Y) {
		if moveDir.X > 0 {
			return 1 // Восток
		} else {
			return 3 // Запад
		}
	} else {
		if moveDir.Y > 0 {
			return 0 // Юг
		} else {
			return 2 // Север
		}
	}

	// Для 8 направлений (раскомментировать при необходимости)
	/*
		angle := math.Atan2(moveDir.Y, moveDir.X)

		// Преобразуем угол в диапазоне [-π, π] в направление [0-7]
		// 0 = восток, 2 = юг, 4 = запад, 6 = север
		direction := int(math.Round(4 * angle / math.Pi))

		// Преобразуем отрицательные направления
		if direction < 0 {
			direction += 8
		}

		return direction
	*/
}

// isPassableBlock проверяет, можно ли пройти сквозь блок.
// Предпочтительно использует метод IsPassable() у поведения блока,
// иначе по умолчанию проходимым считается только воздух.
func isPassableBlock(blockID block.BlockID) bool {
	behavior, exists := block.Get(blockID)
	if exists {
		if p, ok := behavior.(interface{ IsPassable() bool }); ok {
			return p.IsPassable()
		}
	}

	// Фоллбэк на воздух
	return blockID == block.AirBlockID
}

// entitiesCollide проверяет, сталкиваются ли две сущности
func entitiesCollide(pos1 vec.Vec2Float, size1 vec.Vec2Float, pos2 vec.Vec2Float, size2 vec.Vec2Float) bool {
	// Проверка пересечения прямоугольников
	return pos1.X-size1.X/2 < pos2.X+size2.X/2 &&
		pos1.X+size1.X/2 > pos2.X-size2.X/2 &&
		pos1.Y-size1.Y/2 < pos2.Y+size2.Y/2 &&
		pos1.Y+size1.Y/2 > pos2.Y-size2.Y/2
}

// calculateCollisionPoint вычисляет точку коллизии между двумя сущностями
func calculateCollisionPoint(pos1, pos2 vec.Vec2Float) vec.Vec2Float {
	// Простой способ - вернуть среднюю точку между центрами сущностей
	return vec.Vec2Float{
		X: (pos1.X + pos2.X) / 2,
		Y: (pos1.Y + pos2.Y) / 2,
	}
}

// GetStats возвращает статистику по сущностям
func (em *EntityManager) GetStats() map[string]interface{} {
	em.mu.RLock()
	defer em.mu.RUnlock()

	stats := make(map[string]interface{})

	// Общее количество сущностей
	stats["total_entities"] = len(em.entities)

	// Активные сущности
	activeCount := 0
	for _, entity := range em.entities {
		if entity.Active {
			activeCount++
		}
	}
	stats["active_entities"] = activeCount

	// Статистика по типам сущностей
	typeStats := make(map[string]int)
	for _, entity := range em.entities {
		if entity.Active {
			entityType := fmt.Sprintf("type_%d", int(entity.Type))
			typeStats[entityType]++
		}
	}
	stats["entity_types"] = typeStats

	// Количество зарегистрированных поведений
	stats["registered_behaviors"] = len(em.behaviors)

	return stats
}

// AddEntity добавляет уже созданную сущность в менеджер (используется, когда ID выбирается внешним кодом).
// Если сущность с таким ID уже существует, она будет перезаписана.
func (em *EntityManager) AddEntity(entity *Entity) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.entities[entity.ID] = entity
	if entity.ID >= em.nextEntityID {
		em.nextEntityID = entity.ID + 1
	}
}
