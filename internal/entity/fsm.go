package entity

import (
	"math"
	"math/rand"

	"github.com/annel0/mmo-game/internal/physics"
	"github.com/annel0/mmo-game/internal/vec"
)

// State представляет состояние конечного автомата
type State interface {
	Enter(entity *Entity)
	Update(entity *Entity, worldAPI WorldAPI) State
	Exit(entity *Entity)
}

// Entity представляет сущность с конечным автоматом
type Entity struct {
	ID           uint64
	Type         uint16
	Position     vec.Vec2
	Velocity     vec.Vec2
	Collider     *physics.BoxCollider
	CurrentState State
	Data         map[string]interface{}
}

// WorldAPI представляет интерфейс для взаимодействия с миром
type WorldAPI interface {
	IsBlockPassable(pos vec.Vec2) bool
	CanEntityMoveTo(entity *Entity, newPos vec.Vec2) bool
	GetEntitiesInRange(center vec.Vec2, radius int) []*Entity
}

// NewEntity создаёт новую сущность с указанным ID и типом
func NewEntity(id uint64, entityType uint16, pos vec.Vec2) *Entity {
	return &Entity{
		ID:           id,
		Type:         entityType,
		Position:     pos,
		Velocity:     vec.Vec2{X: 0, Y: 0},
		Collider:     physics.NewBoxCollider(1, 1), // По умолчанию 1x1
		CurrentState: nil,
		Data:         make(map[string]interface{}),
	}
}

// Update обновляет состояние сущности
func (e *Entity) Update(worldAPI WorldAPI) {
	if e.CurrentState != nil {
		newState := e.CurrentState.Update(e, worldAPI)
		if newState != e.CurrentState {
			e.CurrentState.Exit(e)
			e.CurrentState = newState
			e.CurrentState.Enter(e)
		}
	}
}

// SetState устанавливает новое состояние сущности
func (e *Entity) SetState(state State) {
	if e.CurrentState != nil {
		e.CurrentState.Exit(e)
	}

	e.CurrentState = state

	if e.CurrentState != nil {
		e.CurrentState.Enter(e)
	}
}

// MoveTo пытается переместить сущность в указанную позицию
func (e *Entity) MoveTo(newPos vec.Vec2, worldAPI WorldAPI) bool {
	if worldAPI.CanEntityMoveTo(e, newPos) {
		e.Position = newPos
		return true
	}
	return false
}

// === Конкретные состояния ===

// IdleState - состояние бездействия
type IdleState struct {
	TimeInState float64
	MaxIdleTime float64
}

// NewIdleState создаёт новое состояние бездействия
func NewIdleState() *IdleState {
	return &IdleState{
		TimeInState: 0,
		MaxIdleTime: 2.0 + rand.Float64()*3.0, // 2-5 секунд
	}
}

func (s *IdleState) Enter(entity *Entity) {
	s.TimeInState = 0
	// Останавливаем движение
	entity.Velocity = vec.Vec2{X: 0, Y: 0}
}

func (s *IdleState) Update(entity *Entity, worldAPI WorldAPI) State {
	s.TimeInState += 1.0 / 60.0 // Предполагаем 60 тиков в секунду

	// Переход в Wander после определённого времени
	if s.TimeInState >= s.MaxIdleTime {
		return NewWanderState()
	}

	return s
}

func (s *IdleState) Exit(entity *Entity) {
	// Ничего не делаем при выходе
}

// WanderState - состояние блуждания
type WanderState struct {
	TargetPos     vec.Vec2
	MoveSpeed     float64
	TimeInState   float64
	MaxWanderTime float64
}

// NewWanderState создаёт новое состояние блуждания
func NewWanderState() *WanderState {
	return &WanderState{
		MoveSpeed:     0.05, // Блоков за тик
		TimeInState:   0,
		MaxWanderTime: 3.0 + rand.Float64()*5.0, // 3-8 секунд
	}
}

func (s *WanderState) Enter(entity *Entity) {
	s.TimeInState = 0

	// Выбираем случайную точку назначения в радиусе 5 блоков
	angle := rand.Float64() * 2 * math.Pi // Случайный угол
	distance := 2.0 + rand.Float64()*3.0  // Расстояние 2-5 блоков

	s.TargetPos = vec.Vec2{
		X: entity.Position.X + int(distance*math.Cos(angle)),
		Y: entity.Position.Y + int(distance*math.Sin(angle)),
	}
}

func (s *WanderState) Update(entity *Entity, worldAPI WorldAPI) State {
	s.TimeInState += 1.0 / 60.0

	// Проверяем, достигли ли цели или истекло время
	if s.TimeInState >= s.MaxWanderTime ||
		(entity.Position.X == s.TargetPos.X && entity.Position.Y == s.TargetPos.Y) {
		return NewIdleState()
	}

	// Движение к цели
	dirX := 0
	dirY := 0

	if entity.Position.X < s.TargetPos.X {
		dirX = 1
	} else if entity.Position.X > s.TargetPos.X {
		dirX = -1
	}

	if entity.Position.Y < s.TargetPos.Y {
		dirY = 1
	} else if entity.Position.Y > s.TargetPos.Y {
		dirY = -1
	}

	// Пробуем двигаться
	newPos := vec.Vec2{
		X: entity.Position.X + dirX,
		Y: entity.Position.Y + dirY,
	}

	// Проверяем возможность перемещения
	if !entity.MoveTo(newPos, worldAPI) {
		// Если не можем двигаться в желаемом направлении, возвращаемся в Idle
		return NewIdleState()
	}

	return s
}

func (s *WanderState) Exit(entity *Entity) {
	// Ничего не делаем при выходе
}
